package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/purchasing-service/internal/model"
)

const supplierColumns = `id, company_id, branch_id, supplier_code, name, email, phone, address, tax_id, is_active, created_at, updated_at`

func scanSupplier(row pgx.Row, s *model.Supplier) error {
	return row.Scan(&s.ID, &s.CompanyID, &s.BranchID, &s.SupplierCode, &s.Name, &s.Email, &s.Phone, &s.Address, &s.TaxID, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)
}

func (h *Handler) listSuppliers(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	query := `SELECT ` + supplierColumns + ` FROM suppliers WHERE company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY supplier_code ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data supplier")
		return
	}
	defer rows.Close()

	suppliers := []model.Supplier{}
	for rows.Next() {
		var s model.Supplier
		if err := scanSupplier(rows, &s); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data supplier")
			return
		}
		suppliers = append(suppliers, s)
	}
	writeJSON(w, http.StatusOK, suppliers)
}

type supplierRequest struct {
	CompanyID    string  `json:"company_id"`
	BranchID     *string `json:"branch_id"`
	SupplierCode string  `json:"supplier_code"`
	Name         string  `json:"name"`
	Email        string  `json:"email"`
	Phone        string  `json:"phone"`
	Address      string  `json:"address"`
	TaxID        string  `json:"tax_id"`
}

func (h *Handler) createSupplier(w http.ResponseWriter, r *http.Request) {
	var req supplierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.SupplierCode = strings.TrimSpace(req.SupplierCode)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.SupplierCode == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id, supplier_code, dan name wajib diisi")
		return
	}

	var s model.Supplier
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO suppliers (company_id, branch_id, supplier_code, name, email, phone, address, tax_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+supplierColumns,
		req.CompanyID, req.BranchID, req.SupplierCode, req.Name, req.Email, req.Phone, req.Address, req.TaxID,
	).Scan(&s.ID, &s.CompanyID, &s.BranchID, &s.SupplierCode, &s.Name, &s.Email, &s.Phone, &s.Address, &s.TaxID, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Supplier code sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat supplier")
		return
	}

	h.events.Publish("purchasing.supplier.created", newAuditEvent("purchasing.supplier.created", actorFromHeader(r), &s.CompanyID, "create", "supplier", s.ID, s))
	writeJSON(w, http.StatusCreated, s)
}

type updateSupplierRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Address  string `json:"address"`
	TaxID    string `json:"tax_id"`
	IsActive bool   `json:"is_active"`
}

func (h *Handler) updateSupplier(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateSupplierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name wajib diisi")
		return
	}

	var s model.Supplier
	err := h.pool.QueryRow(r.Context(), `
		UPDATE suppliers SET name = $1, email = $2, phone = $3, address = $4, tax_id = $5, is_active = $6, updated_at = now()
		WHERE id = $7
		RETURNING `+supplierColumns,
		req.Name, req.Email, req.Phone, req.Address, req.TaxID, req.IsActive, id,
	).Scan(&s.ID, &s.CompanyID, &s.BranchID, &s.SupplierCode, &s.Name, &s.Email, &s.Phone, &s.Address, &s.TaxID, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Supplier tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui supplier")
		return
	}

	h.events.Publish("purchasing.supplier.updated", newAuditEvent("purchasing.supplier.updated", actorFromHeader(r), &s.CompanyID, "update", "supplier", s.ID, s))
	writeJSON(w, http.StatusOK, s)
}
