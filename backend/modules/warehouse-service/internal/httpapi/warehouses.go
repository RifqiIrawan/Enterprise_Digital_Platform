package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/warehouse-service/internal/model"
)

const warehouseColumns = `id, company_id, branch_id, code, name, address, is_active, created_at, updated_at`

func scanWarehouse(row pgx.Row, wh *model.Warehouse) error {
	return row.Scan(&wh.ID, &wh.CompanyID, &wh.BranchID, &wh.Code, &wh.Name, &wh.Address, &wh.IsActive, &wh.CreatedAt, &wh.UpdatedAt)
}

func (h *Handler) listWarehouses(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	rows, err := h.pool.Query(r.Context(), `SELECT `+warehouseColumns+` FROM warehouses WHERE company_id = $1 ORDER BY code ASC`, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data gudang")
		return
	}
	defer rows.Close()

	warehouses := []model.Warehouse{}
	for rows.Next() {
		var wh model.Warehouse
		if err := scanWarehouse(rows, &wh); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data gudang")
			return
		}
		warehouses = append(warehouses, wh)
	}
	writeJSON(w, http.StatusOK, warehouses)
}

type warehouseRequest struct {
	CompanyID string  `json:"company_id"`
	BranchID  *string `json:"branch_id"`
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Address   string  `json:"address"`
}

func (h *Handler) createWarehouse(w http.ResponseWriter, r *http.Request) {
	var req warehouseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.Code == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id, code, dan name wajib diisi")
		return
	}

	var wh model.Warehouse
	err := scanWarehouse(h.pool.QueryRow(r.Context(), `
		INSERT INTO warehouses (company_id, branch_id, code, name, address)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+warehouseColumns,
		req.CompanyID, req.BranchID, req.Code, req.Name, req.Address,
	), &wh)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Kode gudang sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat gudang")
		return
	}

	h.events.Publish("warehouse.warehouse.created", newAuditEvent("warehouse.warehouse.created", actorFromHeader(r), &wh.CompanyID, "create", "warehouse", wh.ID, wh))
	writeJSON(w, http.StatusCreated, wh)
}

type updateWarehouseRequest struct {
	Name     string `json:"name"`
	Address  string `json:"address"`
	IsActive bool   `json:"is_active"`
}

func (h *Handler) updateWarehouse(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateWarehouseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name wajib diisi")
		return
	}

	var wh model.Warehouse
	err := scanWarehouse(h.pool.QueryRow(r.Context(), `
		UPDATE warehouses SET name = $1, address = $2, is_active = $3, updated_at = now()
		WHERE id = $4
		RETURNING `+warehouseColumns,
		req.Name, req.Address, req.IsActive, id,
	), &wh)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Gudang tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui gudang")
		return
	}

	h.events.Publish("warehouse.warehouse.updated", newAuditEvent("warehouse.warehouse.updated", actorFromHeader(r), &wh.CompanyID, "update", "warehouse", wh.ID, wh))
	writeJSON(w, http.StatusOK, wh)
}
