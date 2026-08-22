package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/rbac-service/internal/model"
)

// Override permission per user adalah lapis kedua di atas role_menu_permissions
// (lihat komentar di 001_init.sql): dua user dengan role yang sama bisa dibuat
// berbeda tanpa harus membuat role baru untuk satu orang.
//
// Cakupan versi ini: scope COMPANY saja. Kolom branch_id/department_id memang
// ada di tabel dan ikut unique index-nya, tapi API ini menolak keduanya secara
// eksplisit (400) daripada menyimpan baris yang diam-diam tidak berpengaruh --
// menu-tree & permission efektif di bawah hanya membaca override ber-scope
// company.
//
// Catatan penting soal role: hak dari role SENGAJA tetap dihitung lintas
// company (persis seperti menuTree sejak awal), hanya override-nya yang
// di-scope per company. Menyempitkan hak role ke company yang sedang dipilih
// adalah perubahan perilaku tersendiri -- sidebar user yang punya role di
// company lain akan menyusut -- jadi itu bukan bagian dari perubahan ini.

type userOverrideView struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	CompanyID  string `json:"company_id"`
	MenuID     string `json:"menu_id"`
	MenuName   string `json:"menu_name"`
	MenuPath   string `json:"menu_path"`
	ModuleID   string `json:"module_id"`
	ModuleName string `json:"module_name"`
	model.MenuActions
	CreatedBy *string `json:"created_by"`
}

const userOverrideSelect = `
	SELECT o.id, o.user_id, o.company_id, o.menu_id, m.name, COALESCE(m.path, ''),
	       mod.id, mod.name,
	       o.can_view, o.can_create, o.can_update, o.can_delete, o.can_approve, o.can_export,
	       o.created_by
	FROM user_menu_permission_overrides o
	JOIN menus m ON m.id = o.menu_id
	JOIN modules mod ON mod.id = m.module_id`

func scanUserOverride(row pgx.Row) (userOverrideView, error) {
	var v userOverrideView
	err := row.Scan(&v.ID, &v.UserID, &v.CompanyID, &v.MenuID, &v.MenuName, &v.MenuPath,
		&v.ModuleID, &v.ModuleName,
		&v.CanView, &v.CanCreate, &v.CanUpdate, &v.CanDelete, &v.CanApprove, &v.CanExport,
		&v.CreatedBy)
	return v, err
}

func (h *Handler) listUserOverrides(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user_id wajib diisi")
		return
	}
	companyID := r.URL.Query().Get("company_id")

	rows, err := h.pool.Query(r.Context(), userOverrideSelect+`
		WHERE o.user_id = $1 AND ($2 = '' OR o.company_id = $2::uuid)
		ORDER BY mod.sort_order ASC, m.sort_order ASC`, userID, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat override permission")
		return
	}
	defer rows.Close()

	views := []userOverrideView{}
	for rows.Next() {
		v, err := scanUserOverride(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca override permission")
			return
		}
		views = append(views, v)
	}
	writeJSON(w, http.StatusOK, views)
}

type putUserOverrideRequest struct {
	UserID       string  `json:"user_id"`
	CompanyID    string  `json:"company_id"`
	BranchID     *string `json:"branch_id"`
	DepartmentID *string `json:"department_id"`
	MenuID       string  `json:"menu_id"`
	model.MenuActions
}

// putUserOverride membuat atau memperbarui satu override (idempotent per scope
// user+company+menu). Dipakai PUT, bukan POST, karena pemanggil tidak perlu
// tahu apakah override-nya sudah ada -- UI hanya mengirim keadaan yang
// diinginkan untuk satu menu.
func (h *Handler) putUserOverride(w http.ResponseWriter, r *http.Request) {
	var req putUserOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.UserID == "" || req.CompanyID == "" || req.MenuID == "" {
		writeError(w, http.StatusBadRequest, "user_id, company_id, dan menu_id wajib diisi")
		return
	}
	if req.BranchID != nil || req.DepartmentID != nil {
		writeError(w, http.StatusBadRequest, "Override per branch/department belum didukung, isi hanya company")
		return
	}
	// Hak turunan tanpa hak lihat tidak punya arti di UI; sekaligus menjaga
	// override "cabut akses" tetap terbaca jelas: semua kolom false.
	if !req.CanView && (req.CanCreate || req.CanUpdate || req.CanDelete || req.CanApprove || req.CanExport) {
		writeError(w, http.StatusBadRequest, "Hak create/update/delete/approve/export butuh can_view")
		return
	}

	ctx := r.Context()
	var menuExists bool
	if err := h.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM menus WHERE id = $1)`, req.MenuID).Scan(&menuExists); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa menu")
		return
	}
	if !menuExists {
		writeError(w, http.StatusNotFound, "Menu tidak ditemukan")
		return
	}

	createdBy := r.Header.Get("X-User-Id")
	var createdByPtr *string
	if createdBy != "" {
		createdByPtr = &createdBy
	}

	// DELETE + INSERT dalam satu transaksi, bukan ON CONFLICT: unique index-nya
	// dibangun di atas ekspresi COALESCE(branch_id, '000...') sehingga klausa
	// ON CONFLICT harus menyalin ekspresi itu persis -- mudah lepas sinkron
	// kalau scope-nya nanti diperluas.
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM user_menu_permission_overrides
		WHERE user_id = $1 AND company_id = $2 AND menu_id = $3
		  AND branch_id IS NULL AND department_id IS NULL`,
		req.UserID, req.CompanyID, req.MenuID); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan override permission")
		return
	}

	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO user_menu_permission_overrides
			(user_id, company_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id`,
		req.UserID, req.CompanyID, req.MenuID,
		req.CanView, req.CanCreate, req.CanUpdate, req.CanDelete, req.CanApprove, req.CanExport,
		createdByPtr).Scan(&id); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan override permission")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan override permission")
		return
	}

	view, err := scanUserOverride(h.pool.QueryRow(ctx, userOverrideSelect+` WHERE o.id = $1`, id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca override permission")
		return
	}

	h.events.Publish("rbac.user.override_set", newAuditEvent("rbac.user.override_set", &req.CompanyID, "update", "user_menu_permission_override", id, view))
	writeJSON(w, http.StatusOK, view)
}

// deleteUserOverride mengembalikan menu itu ke hak bawaan role user tersebut.
func (h *Handler) deleteUserOverride(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var companyID string
	err := h.pool.QueryRow(r.Context(),
		`DELETE FROM user_menu_permission_overrides WHERE id = $1 RETURNING company_id`, id).Scan(&companyID)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Override tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghapus override permission")
		return
	}

	h.events.Publish("rbac.user.override_cleared", newAuditEvent("rbac.user.override_cleared", &companyID, "delete", "user_menu_permission_override", id, nil))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type effectivePermissionRow struct {
	MenuID     string `json:"menu_id"`
	MenuName   string `json:"menu_name"`
	MenuPath   string `json:"menu_path"`
	ModuleID   string `json:"module_id"`
	ModuleName string `json:"module_name"`
	SortOrder  int    `json:"sort_order"`
	model.MenuActions
	// Source menjelaskan dari mana hak itu berasal: "role" (bawaan role),
	// "override" (ditimpa khusus untuk user ini), atau "none" (tidak punya
	// akses). Dipakai UI untuk menandai baris yang menyimpang dari role.
	Source string `json:"source"`
	// RoleActions adalah hak bawaan role SEBELUM override, supaya UI bisa
	// menampilkan "aslinya apa" di samping hasil akhirnya.
	RoleActions model.MenuActions `json:"role_actions"`
}

// userPermissions mengembalikan hak efektif user per menu: gabungan (OR) hak
// dari seluruh role yang ditugaskan padanya, lalu ditimpa override company ini
// kalau ada. Endpoint ini yang membuat UI bisa menampilkan hak nyata seorang
// user tanpa harus meniru aturan penggabungannya sendiri.
func (h *Handler) userPermissions(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user_id wajib diisi")
		return
	}
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	ctx := r.Context()

	// Aturan penggabungannya dipakai bersama GET /access (yang dipanggil
	// api-gateway untuk menegakkan hak) -- lihat access.go. Menyalinnya di sini
	// akan membuat tombol yang terlihat di UI dan yang benar-benar diizinkan
	// gateway pelan-pelan berbeda.
	fromRole, fromOverride, err := h.resolveEffective(ctx, userID, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat hak akses user")
		return
	}

	menuRows, err := h.pool.Query(ctx, `
		SELECT m.id, m.name, COALESCE(m.path, ''), m.sort_order, mod.id, mod.name
		FROM menus m
		JOIN modules mod ON mod.id = m.module_id
		WHERE m.is_active = true
		ORDER BY mod.sort_order ASC, m.sort_order ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat daftar menu")
		return
	}
	defer menuRows.Close()

	result := []effectivePermissionRow{}
	for menuRows.Next() {
		var row effectivePermissionRow
		if err := menuRows.Scan(&row.MenuID, &row.MenuName, &row.MenuPath, &row.SortOrder, &row.ModuleID, &row.ModuleName); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca daftar menu")
			return
		}
		row.RoleActions = fromRole[row.MenuID]
		if override, ok := fromOverride[row.MenuID]; ok {
			// Override MENANG UTUH atas role, bukan digabung: itulah gunanya
			// override "cabut akses" (semua kolom false) untuk menu yang
			// sebenarnya diberikan role.
			row.MenuActions = override
			row.Source = "override"
		} else {
			row.MenuActions = row.RoleActions
			if row.CanView {
				row.Source = "role"
			} else {
				row.Source = "none"
			}
		}
		result = append(result, row)
	}
	if err := menuRows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca daftar menu")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
