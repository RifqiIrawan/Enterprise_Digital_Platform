package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/asset-service/internal/model"
)

const assetColumns = `id, company_id, branch_id, warehouse_id, asset_code, name, category, acquisition_date, acquisition_cost, status, notes, salvage_value, useful_life_months, depreciation_method, depreciation_start_date, accumulated_depreciation, created_at, updated_at`

var depreciationMethods = map[string]bool{"STRAIGHT_LINE": true, "DECLINING_BALANCE": true}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func scanAsset(row pgx.Row, a *model.Asset) error {
	if err := row.Scan(&a.ID, &a.CompanyID, &a.BranchID, &a.WarehouseID, &a.AssetCode, &a.Name, &a.Category,
		&a.AcquisitionDate, &a.AcquisitionCost, &a.Status, &a.Notes,
		&a.SalvageValue, &a.UsefulLifeMonths, &a.DepreciationMethod, &a.DepreciationStartDate, &a.AccumulatedDepreciation,
		&a.CreatedAt, &a.UpdatedAt); err != nil {
		return err
	}
	// Nilai buku selalu diturunkan, tidak pernah disimpan -- dua angka yang
	// bisa berbeda adalah dua angka yang cepat atau lambat AKAN berbeda.
	a.BookValue = round2(a.AcquisitionCost - a.AccumulatedDepreciation)
	return nil
}

func (h *Handler) listAssets(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + assetColumns + ` FROM assets WHERE company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY asset_code ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
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
	CompanyID             string  `json:"company_id"`
	BranchID              *string `json:"branch_id"`
	WarehouseID           *string `json:"warehouse_id"`
	AssetCode             string  `json:"asset_code"`
	Name                  string  `json:"name"`
	Category              string  `json:"category"`
	AcquisitionDate       string  `json:"acquisition_date"`
	AcquisitionCost       float64 `json:"acquisition_cost"`
	Notes                 string  `json:"notes"`
	SalvageValue          float64 `json:"salvage_value"`
	UsefulLifeMonths      *int    `json:"useful_life_months"`
	DepreciationMethod    string  `json:"depreciation_method"`
	DepreciationStartDate string  `json:"depreciation_start_date"`
}

// depreciationInput memvalidasi parameter penyusutan yang dipakai createAsset
// maupun updateAsset. Nilai residu yang melebihi harga perolehan akan membuat
// setiap perhitungan penyusutan negatif, jadi ditolak di sini -- bukan
// dibiarkan muncul sebagai angka aneh di jurnal.
func depreciationInput(method, startDate string, usefulLife *int, salvage, cost float64) (string, *time.Time, string) {
	method = strings.TrimSpace(method)
	if method == "" {
		method = "STRAIGHT_LINE"
	}
	if !depreciationMethods[method] {
		return "", nil, "depreciation_method harus STRAIGHT_LINE atau DECLINING_BALANCE"
	}
	if usefulLife != nil && *usefulLife <= 0 {
		return "", nil, "useful_life_months harus lebih besar dari 0 (kosongkan kalau aset tidak disusutkan)"
	}
	if salvage < 0 {
		return "", nil, "salvage_value tidak boleh negatif"
	}
	if salvage > cost {
		return "", nil, "salvage_value tidak boleh melebihi acquisition_cost"
	}
	var start *time.Time
	if startDate != "" {
		parsed, err := time.Parse("2006-01-02", startDate)
		if err != nil {
			return "", nil, "depreciation_start_date harus format YYYY-MM-DD"
		}
		start = &parsed
	}
	return method, start, ""
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

	method, depreciationStart, msg := depreciationInput(req.DepreciationMethod, req.DepreciationStartDate, req.UsefulLifeMonths, req.SalvageValue, req.AcquisitionCost)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	// Tanggal mulai penyusutan yang tidak diisi jatuh ke tanggal perolehan --
	// itu yang benar di hampir semua kasus, dan memaksa mengisi dua tanggal
	// yang sama hanya membuat orang mengisinya asal.
	if depreciationStart == nil {
		depreciationStart = acquisitionDate
	}

	var a model.Asset
	err := scanAsset(h.pool.QueryRow(r.Context(), `
		INSERT INTO assets (company_id, branch_id, warehouse_id, asset_code, name, category, acquisition_date, acquisition_cost, notes,
		                    salvage_value, useful_life_months, depreciation_method, depreciation_start_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING `+assetColumns,
		req.CompanyID, req.BranchID, req.WarehouseID, req.AssetCode, req.Name, req.Category, acquisitionDate, req.AcquisitionCost, req.Notes,
		req.SalvageValue, req.UsefulLifeMonths, method, depreciationStart,
	), &a)
	if err != nil {
		if isDuplicateKey(err) {
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
	// Field penyusutan OPSIONAL di update: yang tidak dikirim tidak diubah.
	// Ini bukan soal kerapian -- klien yang hanya mengirim nama & status
	// (halaman aset sebelum Fase 5, atau skrip lama) tidak boleh diam-diam
	// menghapus umur manfaat sebuah aset dan menghentikan penyusutannya.
	// useful_life_months = 0 adalah cara eksplisit menyatakan "aset ini tidak
	// lagi disusutkan".
	SalvageValue          *float64 `json:"salvage_value"`
	UsefulLifeMonths      *int     `json:"useful_life_months"`
	DepreciationMethod    string   `json:"depreciation_method"`
	DepreciationStartDate string   `json:"depreciation_start_date"`
}

// updateAsset boleh mengubah parameter penyusutan kapan saja; yang berubah
// hanya perhitungan periode BERIKUTNYA. Entry yang sudah diposting membawa
// nilai bukunya sendiri dan tidak dihitung ulang -- jurnal yang sudah masuk
// buku besar tidak boleh berubah karena master datanya disunting.
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

	ctx := r.Context()
	var current model.Asset
	if err := scanAsset(h.pool.QueryRow(ctx, `SELECT `+assetColumns+` FROM assets WHERE id = $1`, id), &current); err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Aset tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat aset")
		return
	}

	salvage := current.SalvageValue
	if req.SalvageValue != nil {
		salvage = *req.SalvageValue
	}
	life := current.UsefulLifeMonths
	if req.UsefulLifeMonths != nil {
		if *req.UsefulLifeMonths == 0 {
			life = nil
		} else {
			life = req.UsefulLifeMonths
		}
	}
	method := current.DepreciationMethod
	if strings.TrimSpace(req.DepreciationMethod) != "" {
		method = strings.TrimSpace(req.DepreciationMethod)
	}
	method, depreciationStart, msg := depreciationInput(method, req.DepreciationStartDate, life, salvage, current.AcquisitionCost)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	var a model.Asset
	err := scanAsset(h.pool.QueryRow(ctx, `
		UPDATE assets SET warehouse_id = $1, name = $2, category = $3, status = $4, notes = $5,
		       salvage_value = $6, useful_life_months = $7, depreciation_method = $8,
		       depreciation_start_date = COALESCE($9, depreciation_start_date), updated_at = now()
		WHERE id = $10
		RETURNING `+assetColumns,
		req.WarehouseID, req.Name, req.Category, req.Status, req.Notes,
		salvage, life, method, depreciationStart, id,
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
