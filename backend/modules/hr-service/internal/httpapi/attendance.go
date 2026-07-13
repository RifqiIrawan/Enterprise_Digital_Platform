package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/hr-service/internal/model"
)

var validAttendanceStatuses = map[string]bool{
	"PRESENT": true, "LATE": true, "EARLY_LEAVE": true, "ABSENT": true, "LEAVE": true,
}

const attendanceColumns = `id, company_id, branch_id, employee_id, log_date, check_in, check_out, source, status, created_at`

func scanAttendance(row pgx.Row, a *model.AttendanceLog) error {
	return row.Scan(&a.ID, &a.CompanyID, &a.BranchID, &a.EmployeeID, &a.LogDate, &a.CheckIn, &a.CheckOut, &a.Source, &a.Status, &a.CreatedAt)
}

func (h *Handler) listAttendance(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	query := `SELECT ` + attendanceColumns + ` FROM attendance_logs WHERE company_id = $1`
	args := []any{companyID}

	if employeeID := r.URL.Query().Get("employee_id"); employeeID != "" {
		args = append(args, employeeID)
		query += " AND employee_id = $" + strconv.Itoa(len(args))
	}
	if period := r.URL.Query().Get("period"); period != "" {
		args = append(args, period)
		query += " AND to_char(log_date, 'YYYY-MM') = $" + strconv.Itoa(len(args))
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += " AND (branch_id = $" + strconv.Itoa(len(args)) + " OR branch_id IS NULL)"
	}
	query += " ORDER BY log_date DESC"

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data absensi")
		return
	}
	defer rows.Close()

	logs := []model.AttendanceLog{}
	for rows.Next() {
		var a model.AttendanceLog
		if err := scanAttendance(rows, &a); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data absensi")
			return
		}
		logs = append(logs, a)
	}
	writeJSON(w, http.StatusOK, logs)
}

type attendanceRequest struct {
	CompanyID  string  `json:"company_id"`
	BranchID   *string `json:"branch_id"`
	EmployeeID string  `json:"employee_id"`
	LogDate    string  `json:"log_date"` // YYYY-MM-DD
	CheckIn    *string `json:"check_in"` // RFC3339, opsional
	CheckOut   *string `json:"check_out"`
	Status     string  `json:"status"`
}

func (h *Handler) createAttendance(w http.ResponseWriter, r *http.Request) {
	var req attendanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Status = strings.ToUpper(strings.TrimSpace(req.Status))
	if req.Status == "" {
		req.Status = "PRESENT"
	}
	if req.CompanyID == "" || req.EmployeeID == "" || req.LogDate == "" {
		writeError(w, http.StatusBadRequest, "company_id, employee_id, dan log_date wajib diisi")
		return
	}
	if !validAttendanceStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "status absensi tidak valid")
		return
	}
	logDate, err := time.Parse("2006-01-02", req.LogDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "log_date harus format YYYY-MM-DD")
		return
	}
	checkIn, err := parseOptionalTime(req.CheckIn)
	if err != nil {
		writeError(w, http.StatusBadRequest, "check_in harus format RFC3339")
		return
	}
	checkOut, err := parseOptionalTime(req.CheckOut)
	if err != nil {
		writeError(w, http.StatusBadRequest, "check_out harus format RFC3339")
		return
	}

	var a model.AttendanceLog
	err = h.pool.QueryRow(r.Context(), `
		INSERT INTO attendance_logs (company_id, branch_id, employee_id, log_date, check_in, check_out, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+attendanceColumns,
		req.CompanyID, req.BranchID, req.EmployeeID, logDate, checkIn, checkOut, req.Status,
	).Scan(&a.ID, &a.CompanyID, &a.BranchID, &a.EmployeeID, &a.LogDate, &a.CheckIn, &a.CheckOut, &a.Source, &a.Status, &a.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Sudah ada catatan absensi untuk karyawan ini di tanggal tersebut")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal mencatat absensi")
		return
	}

	h.events.Publish("hr.attendance.created", newAuditEvent("hr.attendance.created", actorFromHeader(r), &a.CompanyID, "create", "attendance_log", a.ID, a))
	writeJSON(w, http.StatusCreated, a)
}

func (h *Handler) updateAttendance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req attendanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Status = strings.ToUpper(strings.TrimSpace(req.Status))
	if !validAttendanceStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "status absensi tidak valid")
		return
	}
	checkIn, err := parseOptionalTime(req.CheckIn)
	if err != nil {
		writeError(w, http.StatusBadRequest, "check_in harus format RFC3339")
		return
	}
	checkOut, err := parseOptionalTime(req.CheckOut)
	if err != nil {
		writeError(w, http.StatusBadRequest, "check_out harus format RFC3339")
		return
	}

	var a model.AttendanceLog
	err = h.pool.QueryRow(r.Context(), `
		UPDATE attendance_logs SET check_in = $1, check_out = $2, status = $3
		WHERE id = $4
		RETURNING `+attendanceColumns,
		checkIn, checkOut, req.Status, id,
	).Scan(&a.ID, &a.CompanyID, &a.BranchID, &a.EmployeeID, &a.LogDate, &a.CheckIn, &a.CheckOut, &a.Source, &a.Status, &a.CreatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Catatan absensi tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui absensi")
		return
	}

	h.events.Publish("hr.attendance.updated", newAuditEvent("hr.attendance.updated", actorFromHeader(r), &a.CompanyID, "update", "attendance_log", a.ID, a))
	writeJSON(w, http.StatusOK, a)
}

func parseOptionalTime(v *string) (*time.Time, error) {
	if v == nil || *v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *v)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
