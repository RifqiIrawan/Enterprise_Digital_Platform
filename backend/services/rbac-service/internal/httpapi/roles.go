package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/rbac-service/internal/model"
)

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, company_id, code, name, description, is_system, created_at, updated_at
		 FROM roles ORDER BY is_system DESC, name ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat daftar role")
		return
	}
	defer rows.Close()

	roles := []model.Role{}
	for rows.Next() {
		var role model.Role
		if err := rows.Scan(&role.ID, &role.CompanyID, &role.Code, &role.Name, &role.Description, &role.IsSystem, &role.CreatedAt, &role.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data role")
			return
		}
		roles = append(roles, role)
	}
	writeJSON(w, http.StatusOK, roles)
}

type createRoleRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *Handler) createRole(w http.ResponseWriter, r *http.Request) {
	var req createRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Code = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(req.Code, " ", "_")))
	req.Name = strings.TrimSpace(req.Name)
	if req.Code == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "Code dan nama wajib diisi")
		return
	}

	var role model.Role
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO roles (code, name, description, is_system) VALUES ($1, $2, $3, false)
		 RETURNING id, company_id, code, name, description, is_system, created_at, updated_at`,
		req.Code, req.Name, req.Description,
	).Scan(&role.ID, &role.CompanyID, &role.Code, &role.Name, &role.Description, &role.IsSystem, &role.CreatedAt, &role.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Code role sudah dipakai")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat role")
		return
	}

	h.events.Publish("rbac.role.created", newAuditEvent("rbac.role.created", nil, "create", "role", role.ID, role))
	writeJSON(w, http.StatusCreated, role)
}

type updateRoleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *Handler) updateRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Nama wajib diisi")
		return
	}

	var role model.Role
	err := h.pool.QueryRow(r.Context(),
		`UPDATE roles SET name = $1, description = $2, updated_at = now() WHERE id = $3
		 RETURNING id, company_id, code, name, description, is_system, created_at, updated_at`,
		req.Name, req.Description, id,
	).Scan(&role.ID, &role.CompanyID, &role.Code, &role.Name, &role.Description, &role.IsSystem, &role.CreatedAt, &role.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Role tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui role")
		return
	}

	h.events.Publish("rbac.role.updated", newAuditEvent("rbac.role.updated", nil, "update", "role", role.ID, role))
	writeJSON(w, http.StatusOK, role)
}

func (h *Handler) deleteRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var isSystem bool
	err := h.pool.QueryRow(r.Context(), `SELECT is_system FROM roles WHERE id = $1`, id).Scan(&isSystem)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Role tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa role")
		return
	}
	if isSystem {
		writeError(w, http.StatusForbidden, "Role sistem tidak boleh dihapus")
		return
	}

	if _, err := h.pool.Exec(r.Context(), `DELETE FROM roles WHERE id = $1`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghapus role")
		return
	}

	h.events.Publish("rbac.role.deleted", newAuditEvent("rbac.role.deleted", nil, "delete", "role", id, nil))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// menuPermissionRow adalah bentuk gabungan menu + hak akses role tertentu,
// dipakai oleh RolePermissionMatrixPage & RoleCreatePage di frontend.
type menuPermissionRow struct {
	ID         string `json:"id"`
	ModuleID   string `json:"module_id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Icon       string `json:"icon"`
	SortOrder  int    `json:"sort_order"`
	CanView    bool   `json:"can_view"`
	CanCreate  bool   `json:"can_create"`
	CanUpdate  bool   `json:"can_update"`
	CanDelete  bool   `json:"can_delete"`
	CanApprove bool   `json:"can_approve"`
	CanExport  bool   `json:"can_export"`
}

func (h *Handler) fetchRolePermissions(w http.ResponseWriter, r *http.Request, roleID string) ([]menuPermissionRow, bool) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT m.id, m.module_id, m.name, m.path, m.icon, m.sort_order,
		       COALESCE(rmp.can_view, false), COALESCE(rmp.can_create, false),
		       COALESCE(rmp.can_update, false), COALESCE(rmp.can_delete, false),
		       COALESCE(rmp.can_approve, false), COALESCE(rmp.can_export, false)
		FROM menus m
		LEFT JOIN role_menu_permissions rmp ON rmp.menu_id = m.id AND rmp.role_id = $1
		WHERE m.is_active = true
		ORDER BY m.sort_order ASC`, roleID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat permission role")
		return nil, false
	}
	defer rows.Close()

	result := []menuPermissionRow{}
	for rows.Next() {
		var m menuPermissionRow
		if err := rows.Scan(&m.ID, &m.ModuleID, &m.Name, &m.Path, &m.Icon, &m.SortOrder,
			&m.CanView, &m.CanCreate, &m.CanUpdate, &m.CanDelete, &m.CanApprove, &m.CanExport); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca permission role")
			return nil, false
		}
		result = append(result, m)
	}
	return result, true
}

func (h *Handler) getRolePermissions(w http.ResponseWriter, r *http.Request) {
	roleID := r.PathValue("id")

	var exists bool
	if err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM roles WHERE id = $1)`, roleID).Scan(&exists); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa role")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "Role tidak ditemukan")
		return
	}

	result, ok := h.fetchRolePermissions(w, r, roleID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type putPermissionItem struct {
	MenuID     string `json:"menu_id"`
	CanView    bool   `json:"can_view"`
	CanCreate  bool   `json:"can_create"`
	CanUpdate  bool   `json:"can_update"`
	CanDelete  bool   `json:"can_delete"`
	CanApprove bool   `json:"can_approve"`
	CanExport  bool   `json:"can_export"`
}

// putRolePermissions mengganti seluruh set permission role dengan payload
// yang dikirim: menu yang tidak disertakan otomatis kembali ke default
// (tanpa akses), sesuai perilaku RoleCreatePage/RolePermissionMatrixPage
// yang hanya mengirim menu dengan can_view = true.
func (h *Handler) putRolePermissions(w http.ResponseWriter, r *http.Request) {
	roleID := r.PathValue("id")

	var items []putPermissionItem
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM role_menu_permissions WHERE role_id = $1`, roleID); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mereset permission role")
		return
	}

	for _, item := range items {
		if !item.CanView {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			roleID, item.MenuID, item.CanView, item.CanCreate, item.CanUpdate, item.CanDelete, item.CanApprove, item.CanExport,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal menyimpan permission role")
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan permission role")
		return
	}

	h.events.Publish("rbac.role.permissions_updated", newAuditEvent("rbac.role.permissions_updated", nil, "update", "role_menu_permissions", roleID, items))

	result, ok := h.fetchRolePermissions(w, r, roleID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, result)
}
