package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/ticketing-service/internal/model"
)

const ticketCategoryColumns = `id, company_id, branch_id, category_code, name, description, is_active, created_at, updated_at`

func scanTicketCategory(row pgx.Row, c *model.TicketCategory) error {
	return row.Scan(&c.ID, &c.CompanyID, &c.BranchID, &c.CategoryCode, &c.Name, &c.Description, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
}

func (h *Handler) listTicketCategories(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + ticketCategoryColumns + ` FROM ticket_categories WHERE company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY name ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data category")
		return
	}
	defer rows.Close()

	categories := []model.TicketCategory{}
	for rows.Next() {
		var c model.TicketCategory
		if err := scanTicketCategory(rows, &c); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data category")
			return
		}
		categories = append(categories, c)
	}
	writeJSON(w, http.StatusOK, categories)
}

type ticketCategoryRequest struct {
	CompanyID    string  `json:"company_id"`
	BranchID     *string `json:"branch_id"`
	CategoryCode string  `json:"category_code"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	IsActive     *bool   `json:"is_active"`
}

func (h *Handler) createTicketCategory(w http.ResponseWriter, r *http.Request) {
	var req ticketCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.CategoryCode = strings.TrimSpace(req.CategoryCode)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.CategoryCode == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id, category_code, dan name wajib diisi")
		return
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	var c model.TicketCategory
	err := scanTicketCategory(h.pool.QueryRow(r.Context(), `
		INSERT INTO ticket_categories (company_id, branch_id, category_code, name, description, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+ticketCategoryColumns,
		req.CompanyID, req.BranchID, req.CategoryCode, req.Name, req.Description, isActive,
	), &c)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Kode category sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat category")
		return
	}

	h.events.Publish("ticketing.category.created", newAuditEvent("ticketing.category.created", actorFromHeader(r), &c.CompanyID, "create", "ticket_category", c.ID, c))
	writeJSON(w, http.StatusCreated, c)
}

type updateTicketCategoryRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active"`
}

func (h *Handler) updateTicketCategory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateTicketCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name wajib diisi")
		return
	}

	var c model.TicketCategory
	err := scanTicketCategory(h.pool.QueryRow(r.Context(), `
		UPDATE ticket_categories SET name = $1, description = $2, is_active = $3, updated_at = now()
		WHERE id = $4
		RETURNING `+ticketCategoryColumns,
		req.Name, req.Description, req.IsActive, id,
	), &c)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Category tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui category")
		return
	}

	h.events.Publish("ticketing.category.updated", newAuditEvent("ticketing.category.updated", actorFromHeader(r), &c.CompanyID, "update", "ticket_category", c.ID, c))
	writeJSON(w, http.StatusOK, c)
}
