package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/sales-service/internal/model"
)

const customerColumns = `id, company_id, branch_id, customer_code, name, email, phone, address, tax_id, is_active, created_at, updated_at`

func scanCustomer(row pgx.Row, c *model.Customer) error {
	return row.Scan(&c.ID, &c.CompanyID, &c.BranchID, &c.CustomerCode, &c.Name, &c.Email, &c.Phone, &c.Address, &c.TaxID, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
}

func (h *Handler) listCustomers(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	rows, err := h.pool.Query(r.Context(), `SELECT `+customerColumns+` FROM customers WHERE company_id = $1 ORDER BY customer_code ASC`, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data customer")
		return
	}
	defer rows.Close()

	customers := []model.Customer{}
	for rows.Next() {
		var c model.Customer
		if err := scanCustomer(rows, &c); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data customer")
			return
		}
		customers = append(customers, c)
	}
	writeJSON(w, http.StatusOK, customers)
}

type customerRequest struct {
	CompanyID    string  `json:"company_id"`
	BranchID     *string `json:"branch_id"`
	CustomerCode string  `json:"customer_code"`
	Name         string  `json:"name"`
	Email        string  `json:"email"`
	Phone        string  `json:"phone"`
	Address      string  `json:"address"`
	TaxID        string  `json:"tax_id"`
}

func (h *Handler) createCustomer(w http.ResponseWriter, r *http.Request) {
	var req customerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.CustomerCode = strings.TrimSpace(req.CustomerCode)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.CustomerCode == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id, customer_code, dan name wajib diisi")
		return
	}

	var c model.Customer
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO customers (company_id, branch_id, customer_code, name, email, phone, address, tax_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+customerColumns,
		req.CompanyID, req.BranchID, req.CustomerCode, req.Name, req.Email, req.Phone, req.Address, req.TaxID,
	).Scan(&c.ID, &c.CompanyID, &c.BranchID, &c.CustomerCode, &c.Name, &c.Email, &c.Phone, &c.Address, &c.TaxID, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Customer code sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat customer")
		return
	}

	h.events.Publish("sales.customer.created", newAuditEvent("sales.customer.created", actorFromHeader(r), &c.CompanyID, "create", "customer", c.ID, c))
	writeJSON(w, http.StatusCreated, c)
}

type updateCustomerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Address  string `json:"address"`
	TaxID    string `json:"tax_id"`
	IsActive bool   `json:"is_active"`
}

func (h *Handler) updateCustomer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateCustomerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name wajib diisi")
		return
	}

	var c model.Customer
	err := h.pool.QueryRow(r.Context(), `
		UPDATE customers SET name = $1, email = $2, phone = $3, address = $4, tax_id = $5, is_active = $6, updated_at = now()
		WHERE id = $7
		RETURNING `+customerColumns,
		req.Name, req.Email, req.Phone, req.Address, req.TaxID, req.IsActive, id,
	).Scan(&c.ID, &c.CompanyID, &c.BranchID, &c.CustomerCode, &c.Name, &c.Email, &c.Phone, &c.Address, &c.TaxID, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Customer tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui customer")
		return
	}

	h.events.Publish("sales.customer.updated", newAuditEvent("sales.customer.updated", actorFromHeader(r), &c.CompanyID, "update", "customer", c.ID, c))
	writeJSON(w, http.StatusOK, c)
}
