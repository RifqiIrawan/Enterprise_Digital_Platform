package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/qc-service/internal/model"
)

const standardColumns = `id, company_id, branch_id, standard_code, name, product_id, criteria, is_active, created_at, updated_at`

func scanStandard(row pgx.Row, s *model.QualityStandard) error {
	return row.Scan(&s.ID, &s.CompanyID, &s.BranchID, &s.StandardCode, &s.Name, &s.ProductID, &s.Criteria, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)
}

func (h *Handler) listStandards(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT `+standardColumns+` FROM quality_standards WHERE company_id = $1 ORDER BY standard_code ASC`, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data standar mutu")
		return
	}
	defer rows.Close()

	standards := []model.QualityStandard{}
	for rows.Next() {
		var s model.QualityStandard
		if err := scanStandard(rows, &s); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data standar mutu")
			return
		}
		standards = append(standards, s)
	}
	writeJSON(w, http.StatusOK, standards)
}

type standardRequest struct {
	CompanyID    string  `json:"company_id"`
	BranchID     *string `json:"branch_id"`
	StandardCode string  `json:"standard_code"`
	Name         string  `json:"name"`
	ProductID    string  `json:"product_id"`
	Criteria     string  `json:"criteria"`
}

func (h *Handler) createStandard(w http.ResponseWriter, r *http.Request) {
	var req standardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.StandardCode = strings.TrimSpace(req.StandardCode)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.StandardCode == "" || req.Name == "" || req.ProductID == "" {
		writeError(w, http.StatusBadRequest, "company_id, standard_code, name, dan product_id wajib diisi")
		return
	}

	var s model.QualityStandard
	err := scanStandard(h.pool.QueryRow(r.Context(), `
		INSERT INTO quality_standards (company_id, branch_id, standard_code, name, product_id, criteria)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+standardColumns,
		req.CompanyID, req.BranchID, req.StandardCode, req.Name, req.ProductID, req.Criteria,
	), &s)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Kode standar sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat standar mutu")
		return
	}

	h.events.Publish("qc.standard.created", newAuditEvent("qc.standard.created", actorFromHeader(r), &s.CompanyID, "create", "quality_standard", s.ID, s))
	writeJSON(w, http.StatusCreated, s)
}

type updateStandardRequest struct {
	Name     string `json:"name"`
	Criteria string `json:"criteria"`
	IsActive bool   `json:"is_active"`
}

func (h *Handler) updateStandard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateStandardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name wajib diisi")
		return
	}

	var s model.QualityStandard
	err := scanStandard(h.pool.QueryRow(r.Context(), `
		UPDATE quality_standards SET name = $1, criteria = $2, is_active = $3, updated_at = now()
		WHERE id = $4
		RETURNING `+standardColumns,
		req.Name, req.Criteria, req.IsActive, id,
	), &s)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Standar mutu tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui standar mutu")
		return
	}

	h.events.Publish("qc.standard.updated", newAuditEvent("qc.standard.updated", actorFromHeader(r), &s.CompanyID, "update", "quality_standard", s.ID, s))
	writeJSON(w, http.StatusOK, s)
}
