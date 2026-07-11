package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/warehouse-service/internal/model"
)

const productColumns = `id, company_id, branch_id, sku, name, unit, category, cost_price, is_active, created_at, updated_at`

func scanProduct(row pgx.Row, p *model.Product) error {
	return row.Scan(&p.ID, &p.CompanyID, &p.BranchID, &p.SKU, &p.Name, &p.Unit, &p.Category, &p.CostPrice, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
}

func (h *Handler) listProducts(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	rows, err := h.pool.Query(r.Context(), `SELECT `+productColumns+` FROM products WHERE company_id = $1 ORDER BY sku ASC`, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data produk")
		return
	}
	defer rows.Close()

	products := []model.Product{}
	for rows.Next() {
		var p model.Product
		if err := scanProduct(rows, &p); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data produk")
			return
		}
		products = append(products, p)
	}
	writeJSON(w, http.StatusOK, products)
}

type productRequest struct {
	CompanyID string  `json:"company_id"`
	BranchID  *string `json:"branch_id"`
	SKU       string  `json:"sku"`
	Name      string  `json:"name"`
	Unit      string  `json:"unit"`
	Category  string  `json:"category"`
	CostPrice float64 `json:"cost_price"`
}

func (h *Handler) createProduct(w http.ResponseWriter, r *http.Request) {
	var req productRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.SKU = strings.TrimSpace(req.SKU)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.SKU == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id, sku, dan name wajib diisi")
		return
	}
	if req.Unit == "" {
		req.Unit = "pcs"
	}

	var p model.Product
	err := scanProduct(h.pool.QueryRow(r.Context(), `
		INSERT INTO products (company_id, branch_id, sku, name, unit, category, cost_price)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+productColumns,
		req.CompanyID, req.BranchID, req.SKU, req.Name, req.Unit, req.Category, req.CostPrice,
	), &p)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "SKU atau nama produk sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat produk")
		return
	}

	h.events.Publish("warehouse.product.created", newAuditEvent("warehouse.product.created", actorFromHeader(r), &p.CompanyID, "create", "product", p.ID, p))
	writeJSON(w, http.StatusCreated, p)
}

type updateProductRequest struct {
	Name      string  `json:"name"`
	Unit      string  `json:"unit"`
	Category  string  `json:"category"`
	CostPrice float64 `json:"cost_price"`
	IsActive  bool    `json:"is_active"`
}

func (h *Handler) updateProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name wajib diisi")
		return
	}
	if req.Unit == "" {
		req.Unit = "pcs"
	}

	var p model.Product
	err := scanProduct(h.pool.QueryRow(r.Context(), `
		UPDATE products SET name = $1, unit = $2, category = $3, cost_price = $4, is_active = $5, updated_at = now()
		WHERE id = $6
		RETURNING `+productColumns,
		req.Name, req.Unit, req.Category, req.CostPrice, req.IsActive, id,
	), &p)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Produk tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui produk")
		return
	}

	h.events.Publish("warehouse.product.updated", newAuditEvent("warehouse.product.updated", actorFromHeader(r), &p.CompanyID, "update", "product", p.ID, p))
	writeJSON(w, http.StatusOK, p)
}
