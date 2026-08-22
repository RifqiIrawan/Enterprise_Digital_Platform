package httpapi

import (
	"context"
	"net/http"

	"github.com/enterprise-digital-platform/rbac-service/internal/model"
)

// Endpoint ini dipakai api-gateway untuk MENEGAKKAN hak akses, bukan untuk
// menggambar UI. Bedanya dengan GET /user-permissions ada tiga:
//
//  1. Kuncinya PATH menu, bukan menu_id. Gateway memetakan endpoint API ke
//     halaman (mis. POST /api/finance/invoices -> /finance/invoices), dan path
//     itulah satu-satunya identitas menu yang stabil lintas environment --
//     menu_id di-generate ulang tiap kali database di-seed dari nol.
//  2. Hanya menu yang PUNYA hak yang ikut. Gateway tidak perlu tahu daftar
//     menu yang tidak boleh diakses; menu yang tidak ada di map = tidak boleh,
//     jadi jawaban untuk user biasa jauh lebih kecil dan bisa di-cache murah.
//  3. Ada `member`. Company yang dikirim client TIDAK bisa dipercaya begitu
//     saja (lihat authz.CompanyID di api-gateway): tanpa pemeriksaan ini, user
//     bisa menyebut company mana pun yang override "cabut akses"-nya kebetulan
//     tidak ada, dan mendapatkan kembali hak yang sengaja dicabut.
//
// Aturan penggabungannya sendiri SAMA PERSIS dengan /user-permissions -- kedua
// endpoint memanggil resolveEffective di bawah -- supaya tombol yang terlihat
// di UI dan yang benar-benar diizinkan gateway tidak pernah berbeda.

type accessResponse struct {
	UserID    string `json:"user_id"`
	CompanyID string `json:"company_id"`
	// Member = user punya penugasan role aktif di company ini. Gateway menolak
	// seluruh request ber-company X dari user yang bukan member-nya.
	Member      bool                         `json:"member"`
	Permissions map[string]model.MenuActions `json:"permissions"`
}

func (h *Handler) access(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	companyID := r.URL.Query().Get("company_id")
	if userID == "" || companyID == "" {
		writeError(w, http.StatusBadRequest, "user_id dan company_id wajib diisi")
		return
	}

	ctx := r.Context()

	var member bool
	if err := h.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_roles
			WHERE user_id = $1 AND company_id = $2
			  AND valid_from <= now() AND (valid_to IS NULL OR valid_to > now())
		)`, userID, companyID).Scan(&member); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa keanggotaan company")
		return
	}

	resp := accessResponse{UserID: userID, CompanyID: companyID, Member: member, Permissions: map[string]model.MenuActions{}}
	if !member {
		// Tetap 200 dengan permissions kosong, bukan 403: yang bertanya adalah
		// gateway, dan "user ini tidak punya apa-apa di company itu" adalah
		// jawaban yang sah untuk pertanyaannya -- bukan kegagalan.
		writeJSON(w, http.StatusOK, resp)
		return
	}

	fromRole, fromOverride, err := h.resolveEffective(ctx, userID, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat hak akses user")
		return
	}

	rows, err := h.pool.Query(ctx, `
		SELECT m.id, m.path
		FROM menus m
		WHERE m.is_active = true AND m.path IS NOT NULL AND m.path <> ''`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat daftar menu")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var menuID, path string
		if err := rows.Scan(&menuID, &path); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca daftar menu")
			return
		}
		actions, ok := fromOverride[menuID]
		if !ok {
			actions = fromRole[menuID]
		}
		if actions == (model.MenuActions{}) {
			continue
		}
		// Dua menu tidak pernah berbagi path (path adalah rute frontend), jadi
		// tabrakan di map ini tidak mungkin terjadi tanpa salah seed.
		resp.Permissions[path] = actions
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca daftar menu")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// resolveEffective mengembalikan hak per menu_id dari role (digabung OR lintas
// seluruh role user) dan override company ini secara TERPISAH, karena kedua
// pemanggilnya butuh keduanya untuk hal berbeda: /user-permissions menampilkan
// "asalnya dari mana", /access hanya butuh hasil akhirnya.
//
// Penugasan role yang sudah kedaluwarsa (valid_to lewat) atau belum berlaku
// (valid_from di masa depan) tidak ikut dihitung. Kolomnya memang belum bisa
// diisi lewat API mana pun hari ini, tapi menghormatinya sejak awal berarti
// begitu kolomnya dipakai, hak yang habis masa berlakunya berhenti dengan
// sendirinya -- bukan tetap hidup sampai ada yang ingat mencabutnya.
func (h *Handler) resolveEffective(ctx context.Context, userID, companyID string) (fromRole, fromOverride map[string]model.MenuActions, err error) {
	roleRows, err := h.pool.Query(ctx, `
		SELECT rmp.menu_id, rmp.can_view, rmp.can_create, rmp.can_update, rmp.can_delete, rmp.can_approve, rmp.can_export
		FROM user_roles ur
		JOIN role_menu_permissions rmp ON rmp.role_id = ur.role_id
		WHERE ur.user_id = $1
		  AND ur.valid_from <= now() AND (ur.valid_to IS NULL OR ur.valid_to > now())`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer roleRows.Close()

	fromRole = map[string]model.MenuActions{}
	for roleRows.Next() {
		var menuID string
		var a model.MenuActions
		if err := roleRows.Scan(&menuID, &a.CanView, &a.CanCreate, &a.CanUpdate, &a.CanDelete, &a.CanApprove, &a.CanExport); err != nil {
			return nil, nil, err
		}
		fromRole[menuID] = fromRole[menuID].Or(a)
	}
	if err := roleRows.Err(); err != nil {
		return nil, nil, err
	}

	overrideRows, err := h.pool.Query(ctx, `
		SELECT menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export
		FROM user_menu_permission_overrides
		WHERE user_id = $1 AND company_id = $2 AND branch_id IS NULL AND department_id IS NULL`, userID, companyID)
	if err != nil {
		return nil, nil, err
	}
	defer overrideRows.Close()

	fromOverride = map[string]model.MenuActions{}
	for overrideRows.Next() {
		var menuID string
		var a model.MenuActions
		if err := overrideRows.Scan(&menuID, &a.CanView, &a.CanCreate, &a.CanUpdate, &a.CanDelete, &a.CanApprove, &a.CanExport); err != nil {
			return nil, nil, err
		}
		fromOverride[menuID] = a
	}
	if err := overrideRows.Err(); err != nil {
		return nil, nil, err
	}

	return fromRole, fromOverride, nil
}
