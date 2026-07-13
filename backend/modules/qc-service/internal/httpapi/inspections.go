package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/qc-service/internal/model"
)

const inspectionColumns = `id, company_id, branch_id, inspection_number, standard_id, product_id, reference_type, reference_id, reference_number, inspected_quantity, passed_quantity, failed_quantity, result, inspection_date, notes, inspected_by, created_at, updated_at`

func scanInspection(row pgx.Row, i *model.QualityInspection) error {
	return row.Scan(&i.ID, &i.CompanyID, &i.BranchID, &i.InspectionNumber, &i.StandardID, &i.ProductID, &i.ReferenceType, &i.ReferenceID, &i.ReferenceNumber,
		&i.InspectedQuantity, &i.PassedQuantity, &i.FailedQuantity, &i.Result, &i.InspectionDate, &i.Notes, &i.InspectedBy, &i.CreatedAt, &i.UpdatedAt)
}

func (h *Handler) listInspections(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + inspectionColumns + ` FROM quality_inspections WHERE company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY inspection_date DESC, inspection_number DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data inspeksi")
		return
	}
	defer rows.Close()

	inspections := []model.QualityInspection{}
	for rows.Next() {
		var insp model.QualityInspection
		if err := scanInspection(rows, &insp); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data inspeksi")
			return
		}
		inspections = append(inspections, insp)
	}
	writeJSON(w, http.StatusOK, inspections)
}

var validQCReferenceTypes = map[string]bool{
	"PURCHASE_ORDER": true,
	"WORK_ORDER":     true,
	"MANUAL":         true,
}

type createInspectionRequest struct {
	CompanyID         string  `json:"company_id"`
	BranchID          *string `json:"branch_id"`
	StandardID        string  `json:"standard_id"`
	ReferenceType     string  `json:"reference_type"`
	ReferenceID       string  `json:"reference_id"`
	ReferenceNumber   string  `json:"reference_number"`
	InspectedQuantity float64 `json:"inspected_quantity"`
	PassedQuantity    float64 `json:"passed_quantity"`
	FailedQuantity    float64 `json:"failed_quantity"`
	InspectionDate    string  `json:"inspection_date"`
	Notes             string  `json:"notes"`
}

// createInspection adalah catatan hasil inspeksi yang sudah final saat
// dibuat (bukan draft yang butuh langkah lanjutan) -- QC sengaja dibuat
// lebih ringan dari modul lain, tidak memicu mutasi stok otomatis di
// warehouse-service (lihat komentar di migrations/001_init.sql). Hasil
// koreksi stok atas barang FAIL dilakukan manual lewat Stock Opname/manual
// movement di Warehouse.
func (h *Handler) createInspection(w http.ResponseWriter, r *http.Request) {
	var req createInspectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.ReferenceType == "" {
		req.ReferenceType = "MANUAL"
	}
	if req.CompanyID == "" || req.StandardID == "" || req.InspectionDate == "" || req.InspectedQuantity <= 0 {
		writeError(w, http.StatusBadRequest, "company_id, standard_id, inspection_date, dan inspected_quantity > 0 wajib diisi")
		return
	}
	if !validQCReferenceTypes[req.ReferenceType] {
		writeError(w, http.StatusBadRequest, "reference_type tidak valid")
		return
	}
	if req.PassedQuantity < 0 || req.FailedQuantity < 0 {
		writeError(w, http.StatusBadRequest, "passed_quantity dan failed_quantity tidak boleh negatif")
		return
	}
	if req.PassedQuantity+req.FailedQuantity > req.InspectedQuantity {
		writeError(w, http.StatusBadRequest, "passed_quantity + failed_quantity tidak boleh melebihi inspected_quantity")
		return
	}
	inspectionDate, err := time.Parse("2006-01-02", req.InspectionDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "inspection_date harus format YYYY-MM-DD")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var standard model.QualityStandard
	err = scanStandard(tx.QueryRow(ctx, `SELECT `+standardColumns+` FROM quality_standards WHERE id = $1`, req.StandardID), &standard)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Standar mutu tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat standar mutu")
		return
	}
	if !standard.IsActive {
		writeError(w, http.StatusConflict, "Standar mutu ini sudah nonaktif")
		return
	}

	result := "PARTIAL"
	if req.FailedQuantity == 0 {
		result = "PASS"
	} else if req.PassedQuantity == 0 {
		result = "FAIL"
	}

	period := req.InspectionDate[:7]
	inspectionNumber, err := nextSequence(ctx, tx, req.CompanyID, "quality_inspections", "inspection_number", "INS", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor inspeksi")
		return
	}

	var referenceID *string
	if req.ReferenceID != "" {
		referenceID = &req.ReferenceID
	}
	var referenceNumber *string
	if req.ReferenceNumber != "" {
		referenceNumber = &req.ReferenceNumber
	}
	actor := actorFromHeader(r)

	var insp model.QualityInspection
	err = scanInspection(tx.QueryRow(ctx, `
		INSERT INTO quality_inspections (company_id, branch_id, inspection_number, standard_id, product_id, reference_type, reference_id, reference_number, inspected_quantity, passed_quantity, failed_quantity, result, inspection_date, notes, inspected_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING `+inspectionColumns,
		req.CompanyID, req.BranchID, inspectionNumber, standard.ID, standard.ProductID, req.ReferenceType, referenceID, referenceNumber,
		req.InspectedQuantity, req.PassedQuantity, req.FailedQuantity, result, inspectionDate, req.Notes, actor,
	), &insp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat inspeksi")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan inspeksi")
		return
	}

	h.events.Publish("qc.inspection.created", newAuditEvent("qc.inspection.created", actor, &insp.CompanyID, "create", "quality_inspection", insp.ID, insp))
	writeJSON(w, http.StatusCreated, insp)
}

func (h *Handler) getInspection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var insp model.QualityInspection
	err := scanInspection(h.pool.QueryRow(r.Context(), `SELECT `+inspectionColumns+` FROM quality_inspections WHERE id = $1`, id), &insp)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Inspeksi tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat inspeksi")
		return
	}
	writeJSON(w, http.StatusOK, insp)
}

func nextSequence(ctx context.Context, tx pgx.Tx, companyID, table, column, prefix, period string) (string, error) {
	var count int
	likePattern := prefix + "-" + strings.ReplaceAll(period, "-", "") + "-%"
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE company_id = $1 AND %s LIKE $2`, table, column)
	if err := tx.QueryRow(ctx, query, companyID, likePattern).Scan(&count); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%04d", prefix, strings.ReplaceAll(period, "-", ""), count+1), nil
}
