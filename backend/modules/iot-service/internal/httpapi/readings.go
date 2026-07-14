package httpapi

import (
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/iot-service/internal/model"
)

const readingColumns = `r.id, r.device_id, r.company_id, r.branch_id, r.reading_type, r.value_numeric, r.value_text, r.recorded_at, r.created_at, d.device_code, d.name`

const defaultReadingsLimit = 50
const maxReadingsLimit = 500

func scanReading(row pgx.Row, r *model.Reading) error {
	return row.Scan(&r.ID, &r.DeviceID, &r.CompanyID, &r.BranchID, &r.ReadingType, &r.ValueNumeric, &r.ValueText, &r.RecordedAt, &r.CreatedAt, &r.DeviceCode, &r.DeviceName)
}

// listReadings pakai limit/offset asli (bukan cap hardcoded tanpa
// pagination) -- lihat catatan di rencana implementasi soal
// stock-movements di warehouse-service yang dulu dibatasi 200 baris tanpa
// cara mengambil sisanya.
func (h *Handler) listReadings(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + readingColumns + ` FROM readings r JOIN devices d ON d.id = r.device_id WHERE r.company_id = $1`
	args := []any{companyID}
	if deviceID := r.URL.Query().Get("device_id"); deviceID != "" {
		args = append(args, deviceID)
		query += ` AND r.device_id = $` + strconv.Itoa(len(args))
	}
	if readingType := r.URL.Query().Get("reading_type"); readingType != "" {
		args = append(args, readingType)
		query += ` AND r.reading_type = $` + strconv.Itoa(len(args))
	}
	if from := r.URL.Query().Get("from"); from != "" {
		args = append(args, from)
		query += ` AND r.recorded_at >= $` + strconv.Itoa(len(args))
	}
	if to := r.URL.Query().Get("to"); to != "" {
		args = append(args, to)
		query += ` AND r.recorded_at <= $` + strconv.Itoa(len(args))
	}

	limit := parseIntParam(r.URL.Query().Get("limit"), defaultReadingsLimit, 1, maxReadingsLimit)
	offset := parseIntParam(r.URL.Query().Get("offset"), 0, 0, 1<<31-1)
	args = append(args, limit, offset)
	query += ` ORDER BY r.recorded_at DESC LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data reading")
		return
	}
	defer rows.Close()

	readings := []model.Reading{}
	for rows.Next() {
		var rd model.Reading
		if err := scanReading(rows, &rd); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data reading")
			return
		}
		readings = append(readings, rd)
	}
	writeJSON(w, http.StatusOK, readings)
}

func parseIntParam(raw string, fallback, min, max int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min {
		return fallback
	}
	if n > max {
		return max
	}
	return n
}
