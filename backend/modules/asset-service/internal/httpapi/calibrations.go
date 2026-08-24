package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/asset-service/internal/model"
)

const calibrationColumns = `id, company_id, branch_id, asset_id, scheduled_date, performed_date, interval_months, next_due_date, result, certificate_number, performed_by, status, notes, created_at, updated_at`

var calibrationResults = map[string]bool{"PASS": true, "FAIL": true, "ADJUSTED": true}

func scanCalibration(row pgx.Row, c *model.Calibration) error {
	return row.Scan(&c.ID, &c.CompanyID, &c.BranchID, &c.AssetID, &c.ScheduledDate, &c.PerformedDate, &c.IntervalMonths,
		&c.NextDueDate, &c.Result, &c.CertificateNumber, &c.PerformedBy, &c.Status, &c.Notes, &c.CreatedAt, &c.UpdatedAt)
}

func (h *Handler) listCalibrations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	companyID := q.Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + calibrationColumns + ` FROM calibrations WHERE company_id = $1`
	args := []any{companyID}
	addFilter := func(value, clause string) {
		if value == "" {
			return
		}
		args = append(args, value)
		query += ` AND ` + clause + ` $` + strconv.Itoa(len(args))
	}
	if branchID := q.Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	addFilter(q.Get("asset_id"), `asset_id =`)
	addFilter(q.Get("status"), `status =`)
	// due_before menjawab pertanyaan yang paling sering ditanyakan ke daftar
	// ini: mana yang jatuh tempo sebelum tanggal tertentu.
	addFilter(q.Get("due_before"), `scheduled_date <=`)
	query += ` ORDER BY scheduled_date ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data kalibrasi")
		return
	}
	defer rows.Close()

	calibrations := []model.Calibration{}
	for rows.Next() {
		var c model.Calibration
		if err := scanCalibration(rows, &c); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data kalibrasi")
			return
		}
		calibrations = append(calibrations, c)
	}
	writeJSON(w, http.StatusOK, calibrations)
}

type createCalibrationRequest struct {
	CompanyID      string  `json:"company_id"`
	BranchID       *string `json:"branch_id"`
	AssetID        string  `json:"asset_id"`
	ScheduledDate  string  `json:"scheduled_date"`
	IntervalMonths *int    `json:"interval_months"`
	Notes          string  `json:"notes"`
}

func (h *Handler) createCalibration(w http.ResponseWriter, r *http.Request) {
	var req createCalibrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.AssetID == "" || req.ScheduledDate == "" {
		writeError(w, http.StatusBadRequest, "company_id, asset_id, dan scheduled_date wajib diisi")
		return
	}
	scheduledDate, err := time.Parse("2006-01-02", req.ScheduledDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "scheduled_date harus format YYYY-MM-DD")
		return
	}
	if req.IntervalMonths != nil && *req.IntervalMonths <= 0 {
		writeError(w, http.StatusBadRequest, "interval_months harus lebih besar dari 0 (kosongkan untuk kalibrasi sekali saja)")
		return
	}

	ctx := r.Context()
	var assetStatus string
	err = h.pool.QueryRow(ctx, `SELECT status FROM assets WHERE id = $1 AND company_id = $2`, req.AssetID, req.CompanyID).Scan(&assetStatus)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Aset tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat aset")
		return
	}
	if assetStatus == "DISPOSED" {
		writeError(w, http.StatusConflict, "Aset ini sudah dilepas (DISPOSED)")
		return
	}

	var c model.Calibration
	err = scanCalibration(h.pool.QueryRow(ctx, `
		INSERT INTO calibrations (company_id, branch_id, asset_id, scheduled_date, interval_months, notes)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+calibrationColumns,
		req.CompanyID, req.BranchID, req.AssetID, scheduledDate, req.IntervalMonths, req.Notes), &c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat jadwal kalibrasi")
		return
	}

	h.events.Publish("asset.calibration.scheduled", newAuditEvent("asset.calibration.scheduled", actorFromHeader(r), &c.CompanyID, "create", "calibration", c.ID, c))
	writeJSON(w, http.StatusCreated, c)
}

type completeCalibrationRequest struct {
	PerformedDate     string `json:"performed_date"`
	Result            string `json:"result"`
	CertificateNumber string `json:"certificate_number"`
	PerformedBy       string `json:"performed_by"`
	Notes             string `json:"notes"`
}

type completedCalibrationResponse struct {
	model.Calibration
	// NextCalibration diisi kalau kalibrasi berikutnya otomatis dijadwalkan.
	NextCalibration *model.Calibration `json:"next_calibration,omitempty"`
}

// completeCalibration mencatat hasil kalibrasi, dan dua hal menyusul otomatis:
//
//   - PASS/ADJUSTED dengan interval_months terisi → kalibrasi berikutnya
//     langsung dijadwalkan pada performed_date + interval. Kalibrasi adalah
//     kewajiban berulang; menyerahkan penjadwalan ulang pada ingatan orang
//     adalah cara paling mudah membuat alat ukur kedaluwarsa tanpa ada yang
//     menyadarinya.
//   - FAIL → status asetnya jadi MAINTENANCE dan kalibrasi berikutnya SENGAJA
//     tidak dijadwalkan. Alat yang gagal kalibrasi bukan alat yang perlu
//     diperiksa lagi beberapa bulan lagi, tapi alat yang hasil ukurnya tidak
//     bisa dipakai sekarang; kapan kalibrasi berikutnya tergantung kapan
//     perbaikannya selesai, dan itu keputusan orang, bukan tebakan service ini.
func (h *Handler) completeCalibration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	var req completeCalibrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Result = strings.ToUpper(strings.TrimSpace(req.Result))
	if !calibrationResults[req.Result] {
		writeError(w, http.StatusBadRequest, "result harus PASS, FAIL, atau ADJUSTED")
		return
	}
	performedDate := time.Now()
	if req.PerformedDate != "" {
		parsed, err := time.Parse("2006-01-02", req.PerformedDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "performed_date harus format YYYY-MM-DD")
			return
		}
		performedDate = parsed
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var current model.Calibration
	err = scanCalibration(tx.QueryRow(ctx, `SELECT `+calibrationColumns+` FROM calibrations WHERE id = $1 FOR UPDATE`, id), &current)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Jadwal kalibrasi tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat jadwal kalibrasi")
		return
	}
	if current.Status != "SCHEDULED" {
		writeError(w, http.StatusConflict, "Jadwal kalibrasi tidak berstatus SCHEDULED")
		return
	}

	var nextDue *time.Time
	if req.Result != "FAIL" && current.IntervalMonths != nil {
		due := performedDate.AddDate(0, *current.IntervalMonths, 0)
		nextDue = &due
	}

	var c model.Calibration
	err = scanCalibration(tx.QueryRow(ctx, `
		UPDATE calibrations
		SET status = 'COMPLETED', performed_date = $1, result = $2, next_due_date = $3,
		    certificate_number = NULLIF($4, ''), performed_by = NULLIF($5, ''),
		    notes = CASE WHEN $6 <> '' THEN $6 ELSE notes END, updated_at = now()
		WHERE id = $7 AND status = 'SCHEDULED'
		RETURNING `+calibrationColumns,
		performedDate, req.Result, nextDue, strings.TrimSpace(req.CertificateNumber), strings.TrimSpace(req.PerformedBy), req.Notes, id), &c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan hasil kalibrasi")
		return
	}

	var next *model.Calibration
	if nextDue != nil {
		var created model.Calibration
		err = scanCalibration(tx.QueryRow(ctx, `
			INSERT INTO calibrations (company_id, branch_id, asset_id, scheduled_date, interval_months, notes)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING `+calibrationColumns,
			c.CompanyID, c.BranchID, c.AssetID, *nextDue, c.IntervalMonths,
			"Dijadwalkan otomatis dari kalibrasi "+c.PerformedDate.Format("2006-01-02")), &created)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Hasil kalibrasi tersimpan, tetapi gagal menjadwalkan kalibrasi berikutnya")
			return
		}
		next = &created
	}

	if req.Result == "FAIL" {
		if _, err := tx.Exec(ctx, `UPDATE assets SET status = 'MAINTENANCE', updated_at = now() WHERE id = $1 AND status = 'ACTIVE'`, c.AssetID); err != nil {
			writeError(w, http.StatusInternalServerError, "Hasil kalibrasi tersimpan, tetapi gagal memperbarui status aset")
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan hasil kalibrasi")
		return
	}

	h.events.Publish("asset.calibration.completed", newAuditEvent("asset.calibration.completed", actor, &c.CompanyID, "update", "calibration", c.ID, c))
	writeJSON(w, http.StatusOK, completedCalibrationResponse{Calibration: c, NextCalibration: next})
}

func (h *Handler) cancelCalibration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)

	var c model.Calibration
	err := scanCalibration(h.pool.QueryRow(r.Context(), `
		UPDATE calibrations SET status = 'CANCELLED', updated_at = now()
		WHERE id = $1 AND status = 'SCHEDULED'
		RETURNING `+calibrationColumns, id), &c)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Jadwal kalibrasi tidak ditemukan atau tidak berstatus SCHEDULED")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membatalkan jadwal kalibrasi")
		return
	}

	h.events.Publish("asset.calibration.cancelled", newAuditEvent("asset.calibration.cancelled", actor, &c.CompanyID, "update", "calibration", c.ID, c))
	writeJSON(w, http.StatusOK, c)
}
