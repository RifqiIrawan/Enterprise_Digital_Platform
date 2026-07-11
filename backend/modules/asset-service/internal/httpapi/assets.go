package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/asset-service/internal/model"
)

const assetColumns = `id, company_id, branch_id, warehouse_id, asset_code, name, category, acquisition_date, acquisition_cost, status, notes, created_at, updated_at`

func scanAsset(row pgx.Row, a *model.Asset) error {
	return row.Scan(&a.ID, &a.CompanyID, &a.BranchID, &a.WarehouseID, &a.AssetCode, &a.Name, &a.Category, &a.AcquisitionDate, &a.AcquisitionCost, &a.Status, &a.Notes, &a.CreatedAt, &a.UpdatedAt)
}

func (h *Handler) listAssets(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT `+assetColumns+` FROM assets WHERE company_id = $1 ORDER BY asset_code ASC`, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data aset")
		return
	}
	defer rows.Close()

	assets := []model.Asset{}
	for rows.Next() {
		var a model.Asset
		if err := scanAsset(rows, &a); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data aset")
			return
		}
		assets = append(assets, a)
	}
	writeJSON(w, http.StatusOK, assets)
}

type assetRequest struct {
	CompanyID       string  `json:"company_id"`
	BranchID        *string `json:"branch_id"`
	WarehouseID     *string `json:"warehouse_id"`
	AssetCode       string  `json:"asset_code"`
	Name            string  `json:"name"`
	Category        string  `json:"category"`
	AcquisitionDate string  `json:"acquisition_date"`
	AcquisitionCost float64 `json:"acquisition_cost"`
	Notes           string  `json:"notes"`
}

func (h *Handler) createAsset(w http.ResponseWriter, r *http.Request) {
	var req assetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.AssetCode = strings.TrimSpace(req.AssetCode)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.AssetCode == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id, asset_code, dan name wajib diisi")
		return
	}

	var acquisitionDate *time.Time
	if req.AcquisitionDate != "" {
		parsed, err := time.Parse("2006-01-02", req.AcquisitionDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "acquisition_date harus format YYYY-MM-DD")
			return
		}
		acquisitionDate = &parsed
	}

	var a model.Asset
	err := scanAsset(h.pool.QueryRow(r.Context(), `
		INSERT INTO assets (company_id, branch_id, warehouse_id, asset_code, name, category, acquisition_date, acquisition_cost, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+assetColumns,
		req.CompanyID, req.BranchID, req.WarehouseID, req.AssetCode, req.Name, req.Category, acquisitionDate, req.AcquisitionCost, req.Notes,
	), &a)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Kode aset sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat aset")
		return
	}

	h.events.Publish("asset.asset.created", newAuditEvent("asset.asset.created", actorFromHeader(r), &a.CompanyID, "create", "asset", a.ID, a))
	writeJSON(w, http.StatusCreated, a)
}

type updateAssetRequest struct {
	WarehouseID *string `json:"warehouse_id"`
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	Status      string  `json:"status"`
	Notes       string  `json:"notes"`
}

func (h *Handler) updateAsset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateAssetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name wajib diisi")
		return
	}
	if req.Status != "ACTIVE" && req.Status != "MAINTENANCE" && req.Status != "DISPOSED" {
		writeError(w, http.StatusBadRequest, "status harus ACTIVE, MAINTENANCE, atau DISPOSED")
		return
	}

	var a model.Asset
	err := scanAsset(h.pool.QueryRow(r.Context(), `
		UPDATE assets SET warehouse_id = $1, name = $2, category = $3, status = $4, notes = $5, updated_at = now()
		WHERE id = $6
		RETURNING `+assetColumns,
		req.WarehouseID, req.Name, req.Category, req.Status, req.Notes, id,
	), &a)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Aset tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui aset")
		return
	}

	h.events.Publish("asset.asset.updated", newAuditEvent("asset.asset.updated", actorFromHeader(r), &a.CompanyID, "update", "asset", a.ID, a))
	writeJSON(w, http.StatusOK, a)
}
