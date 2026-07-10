package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// userRoleView adalah UserRole yang digabung dengan role_code/role_name,
// dipakai oleh UserRoleAssignmentPage untuk menampilkan chip role per user.
type userRoleView struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	RoleID       string     `json:"role_id"`
	RoleCode     string     `json:"role_code"`
	RoleName     string     `json:"role_name"`
	CompanyID    string     `json:"company_id"`
	BranchID     *string    `json:"branch_id"`
	DepartmentID *string    `json:"department_id"`
	ValidFrom    time.Time  `json:"valid_from"`
	ValidTo      *time.Time `json:"valid_to"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (h *Handler) listUserRoles(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user_id wajib diisi")
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT ur.id, ur.user_id, ur.role_id, ro.code, ro.name, ur.company_id, ur.branch_id, ur.department_id,
		       ur.valid_from, ur.valid_to, ur.created_at
		FROM user_roles ur
		JOIN roles ro ON ro.id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY ur.created_at ASC`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat role user")
		return
	}
	defer rows.Close()

	views := []userRoleView{}
	for rows.Next() {
		var v userRoleView
		if err := rows.Scan(&v.ID, &v.UserID, &v.RoleID, &v.RoleCode, &v.RoleName, &v.CompanyID, &v.BranchID, &v.DepartmentID, &v.ValidFrom, &v.ValidTo, &v.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca role user")
			return
		}
		views = append(views, v)
	}
	writeJSON(w, http.StatusOK, views)
}

type assignUserRoleRequest struct {
	UserID    string `json:"user_id"`
	RoleID    string `json:"role_id"`
	CompanyID string `json:"company_id"`
}

func (h *Handler) assignUserRole(w http.ResponseWriter, r *http.Request) {
	var req assignUserRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.UserID == "" || req.RoleID == "" || req.CompanyID == "" {
		writeError(w, http.StatusBadRequest, "user_id, role_id, dan company_id wajib diisi")
		return
	}

	assignedBy := r.Header.Get("X-User-Id")
	var assignedByPtr *string
	if assignedBy != "" {
		assignedByPtr = &assignedBy
	}

	var v userRoleView
	err := h.pool.QueryRow(r.Context(), `
		WITH inserted AS (
			INSERT INTO user_roles (user_id, role_id, company_id, assigned_by)
			VALUES ($1, $2, $3, $4)
			RETURNING id, user_id, role_id, company_id, branch_id, department_id, valid_from, valid_to, created_at
		)
		SELECT inserted.id, inserted.user_id, inserted.role_id, ro.code, ro.name, inserted.company_id,
		       inserted.branch_id, inserted.department_id, inserted.valid_from, inserted.valid_to, inserted.created_at
		FROM inserted
		JOIN roles ro ON ro.id = inserted.role_id`,
		req.UserID, req.RoleID, req.CompanyID, assignedByPtr,
	).Scan(&v.ID, &v.UserID, &v.RoleID, &v.RoleCode, &v.RoleName, &v.CompanyID, &v.BranchID, &v.DepartmentID, &v.ValidFrom, &v.ValidTo, &v.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menugaskan role")
		return
	}

	h.events.Publish("rbac.role.assigned", newAuditEvent("rbac.role.assigned", &req.CompanyID, "assign", "user_role", v.ID, v))
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handler) revokeUserRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var companyID string
	err := h.pool.QueryRow(r.Context(), `DELETE FROM user_roles WHERE id = $1 RETURNING company_id`, id).Scan(&companyID)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Penugasan role tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mencabut role")
		return
	}

	h.events.Publish("rbac.role.revoked", newAuditEvent("rbac.role.revoked", &companyID, "revoke", "user_role", id, nil))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
