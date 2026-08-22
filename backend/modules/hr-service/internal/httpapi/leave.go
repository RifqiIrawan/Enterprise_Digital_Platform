package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/hr-service/internal/model"
)

// UNPAID adalah satu-satunya jenis yang memotong gaji; sisanya dibayar penuh
// dan justru DIHITUNG SEBAGAI HADIR saat pro-rata payroll (lihat leaveDays di
// payroll.go), supaya cuti tahunan tidak diam-diam memotong gaji lewat jalur
// absensi.
var validLeaveTypes = map[string]bool{
	"ANNUAL": true, "SICK": true, "MATERNITY": true, "UNPAID": true, "OTHER": true,
}

// maxLeaveRangeDays membatasi rentang satu pengajuan. Bukan aturan HR, cuma
// pagar supaya rentang salah ketik (mis. tahun keliru) tidak memaksa
// workingDaysBetween menghitung ribuan hari.
const maxLeaveRangeDays = 366

const leaveColumns = `id, company_id, branch_id, employee_id, employee_name, leave_type, start_date, end_date,
	total_days, COALESCE(reason, ''), status, COALESCE(rejection_reason, ''), submitted_at, decided_at, decided_by,
	created_by, created_at, updated_at`

func scanLeave(row pgx.Row, l *model.LeaveRequest) error {
	return row.Scan(&l.ID, &l.CompanyID, &l.BranchID, &l.EmployeeID, &l.EmployeeName, &l.LeaveType, &l.StartDate,
		&l.EndDate, &l.TotalDays, &l.Reason, &l.Status, &l.RejectionReason, &l.SubmittedAt, &l.DecidedAt,
		&l.DecidedBy, &l.CreatedBy, &l.CreatedAt, &l.UpdatedAt)
}

func (h *Handler) listLeaveRequests(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	query := `SELECT ` + leaveColumns + ` FROM leave_requests WHERE company_id = $1`
	args := []any{companyID}

	if employeeID := r.URL.Query().Get("employee_id"); employeeID != "" {
		args = append(args, employeeID)
		query += ` AND employee_id = $` + strconv.Itoa(len(args))
	}
	if status := r.URL.Query().Get("status"); status != "" {
		args = append(args, strings.ToUpper(status))
		query += ` AND status = $` + strconv.Itoa(len(args))
	}
	// Filter periode memakai OVERLAP, bukan kecocokan bulan start_date saja:
	// cuti 28 Juli - 3 Agustus harus muncul di periode Juli MAUPUN Agustus,
	// karena keduanya memang terdampak.
	if period := r.URL.Query().Get("period"); period != "" {
		args = append(args, period)
		p := `$` + strconv.Itoa(len(args))
		query += ` AND start_date < (to_date(` + p + `, 'YYYY-MM') + INTERVAL '1 month') AND end_date >= to_date(` + p + `, 'YYYY-MM')`
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY start_date DESC, created_at DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data cuti")
		return
	}
	defer rows.Close()

	requests := []model.LeaveRequest{}
	for rows.Next() {
		var l model.LeaveRequest
		if err := scanLeave(rows, &l); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data cuti")
			return
		}
		requests = append(requests, l)
	}
	writeJSON(w, http.StatusOK, requests)
}

type leaveRequestPayload struct {
	CompanyID  string  `json:"company_id"`
	BranchID   *string `json:"branch_id"`
	EmployeeID string  `json:"employee_id"`
	LeaveType  string  `json:"leave_type"`
	StartDate  string  `json:"start_date"` // YYYY-MM-DD
	EndDate    string  `json:"end_date"`   // YYYY-MM-DD
	Reason     string  `json:"reason"`
}

// parseLeaveDates memvalidasi & mengurai rentang tanggal pengajuan. Dipakai
// createLeaveRequest dan updateLeaveRequest supaya aturannya persis sama.
func parseLeaveDates(startStr, endStr string) (time.Time, time.Time, string) {
	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		return time.Time{}, time.Time{}, "start_date harus format YYYY-MM-DD"
	}
	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		return time.Time{}, time.Time{}, "end_date harus format YYYY-MM-DD"
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, "end_date tidak boleh lebih awal dari start_date"
	}
	if end.Sub(start) > maxLeaveRangeDays*24*time.Hour {
		return time.Time{}, time.Time{}, "rentang cuti terlalu panjang (maksimal 1 tahun)"
	}
	return start, end, ""
}

func (h *Handler) createLeaveRequest(w http.ResponseWriter, r *http.Request) {
	var req leaveRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.LeaveType = strings.ToUpper(strings.TrimSpace(req.LeaveType))
	if req.CompanyID == "" || req.EmployeeID == "" || req.StartDate == "" || req.EndDate == "" {
		writeError(w, http.StatusBadRequest, "company_id, employee_id, start_date, dan end_date wajib diisi")
		return
	}
	if !validLeaveTypes[req.LeaveType] {
		writeError(w, http.StatusBadRequest, "leave_type tidak valid")
		return
	}
	start, end, msg := parseLeaveDates(req.StartDate, req.EndDate)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	ctx := r.Context()
	emp, errMsg, status := h.activeEmployee(ctx, req.EmployeeID, req.CompanyID)
	if errMsg != "" {
		writeError(w, status, errMsg)
		return
	}

	// Rentang cuti yang bertumpuk pada karyawan yang sama tidak masuk akal dan
	// membuat rekap hari cuti per periode menghitung hari yang sama dua kali.
	overlaps, err := h.hasOverlappingLeave(ctx, req.EmployeeID, start, end, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa cuti yang bertumpuk")
		return
	}
	if overlaps {
		writeError(w, http.StatusConflict, "Sudah ada pengajuan cuti aktif yang bertumpuk dengan rentang tanggal ini")
		return
	}

	// total_days memakai kalender company: tanggal merah di tengah rentang cuti
	// tidak ikut memotong jatah, dan tidak dihitung sebagai hari cuti di payroll.
	totalDays, err := h.workingDaysBetween(ctx, req.CompanyID, start, end)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghitung hari kerja")
		return
	}

	var l model.LeaveRequest
	err = scanLeave(h.pool.QueryRow(ctx, `
		INSERT INTO leave_requests (company_id, branch_id, employee_id, employee_name, leave_type, start_date, end_date, total_days, reason, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING `+leaveColumns,
		req.CompanyID, req.BranchID, req.EmployeeID, employeeFullName(emp), req.LeaveType, start, end,
		totalDays, nullIfEmpty(req.Reason), actorFromHeader(r),
	), &l)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan pengajuan cuti")
		return
	}

	h.events.Publish("hr.leave.created", newAuditEvent("hr.leave.created", actorFromHeader(r), &l.CompanyID, "create", "leave_request", l.ID, l))
	writeJSON(w, http.StatusCreated, l)
}

// updateLeaveRequest hanya boleh saat DRAFT. Begitu diajukan (SUBMITTED),
// isinya dikunci -- kalau boleh diubah setelah itu, penyetuju bisa menyetujui
// rentang yang berbeda dari yang dia baca.
func (h *Handler) updateLeaveRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req leaveRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.LeaveType = strings.ToUpper(strings.TrimSpace(req.LeaveType))
	if !validLeaveTypes[req.LeaveType] {
		writeError(w, http.StatusBadRequest, "leave_type tidak valid")
		return
	}
	start, end, msg := parseLeaveDates(req.StartDate, req.EndDate)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	ctx := r.Context()
	var existing model.LeaveRequest
	err := scanLeave(h.pool.QueryRow(ctx, `SELECT `+leaveColumns+` FROM leave_requests WHERE id = $1`, id), &existing)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Pengajuan cuti tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat pengajuan cuti")
		return
	}
	if existing.Status != "DRAFT" {
		writeError(w, http.StatusConflict, "Hanya pengajuan berstatus DRAFT yang bisa diubah (status sekarang: "+existing.Status+")")
		return
	}

	overlaps, err := h.hasOverlappingLeave(ctx, existing.EmployeeID, start, end, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa cuti yang bertumpuk")
		return
	}
	if overlaps {
		writeError(w, http.StatusConflict, "Sudah ada pengajuan cuti aktif yang bertumpuk dengan rentang tanggal ini")
		return
	}

	totalDays, err := h.workingDaysBetween(ctx, existing.CompanyID, start, end)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghitung hari kerja")
		return
	}

	var l model.LeaveRequest
	err = scanLeave(h.pool.QueryRow(ctx, `
		UPDATE leave_requests SET leave_type = $1, start_date = $2, end_date = $3, total_days = $4, reason = $5, updated_at = now()
		WHERE id = $6
		RETURNING `+leaveColumns,
		req.LeaveType, start, end, totalDays, nullIfEmpty(req.Reason), id,
	), &l)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui pengajuan cuti")
		return
	}

	h.events.Publish("hr.leave.updated", newAuditEvent("hr.leave.updated", actorFromHeader(r), &l.CompanyID, "update", "leave_request", l.ID, l))
	writeJSON(w, http.StatusOK, l)
}

type leaveDecisionRequest struct {
	RejectionReason string `json:"rejection_reason"`
}

func (h *Handler) submitLeaveRequest(w http.ResponseWriter, r *http.Request) {
	h.transitionLeave(w, r, []string{"DRAFT"}, "SUBMITTED", "hr.leave.submitted", "Hanya pengajuan DRAFT yang bisa diajukan")
}

func (h *Handler) approveLeaveRequest(w http.ResponseWriter, r *http.Request) {
	h.transitionLeave(w, r, []string{"SUBMITTED"}, "APPROVED", "hr.leave.approved", "Hanya pengajuan SUBMITTED yang bisa disetujui")
}

func (h *Handler) rejectLeaveRequest(w http.ResponseWriter, r *http.Request) {
	h.transitionLeave(w, r, []string{"SUBMITTED"}, "REJECTED", "hr.leave.rejected", "Hanya pengajuan SUBMITTED yang bisa ditolak")
}

// cancelLeaveRequest juga boleh dari APPROVED -- pembatalan cuti yang sudah
// disetujui itu hal biasa. Yang menghentikannya bukan status, melainkan
// payroll periode terkait yang sudah POSTED (dicek di transitionLeave).
func (h *Handler) cancelLeaveRequest(w http.ResponseWriter, r *http.Request) {
	h.transitionLeave(w, r, []string{"DRAFT", "SUBMITTED", "APPROVED"}, "CANCELLED", "hr.leave.cancelled", "Pengajuan ini sudah tidak bisa dibatalkan")
}

// transitionLeave menjalankan semua perpindahan status cuti. Selain aturan
// status asal, ada satu pagar lintas-modul: pengajuan APPROVED yang periodenya
// sudah diposting payroll TIDAK boleh dicabut lagi (reject/cancel), karena
// pro-rata gaji periode itu sudah terlanjur dihitung dari hari cutinya dan
// sudah masuk jurnal GL. Koreksinya lewat jurnal balik di finance-service,
// bukan dengan diam-diam mengubah data sumbernya di sini.
func (h *Handler) transitionLeave(w http.ResponseWriter, r *http.Request, from []string, to, eventType, notAllowedMsg string) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	var req leaveDecisionRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // body opsional kecuali untuk REJECTED
	}
	if to == "REJECTED" && strings.TrimSpace(req.RejectionReason) == "" {
		writeError(w, http.StatusBadRequest, "rejection_reason wajib diisi saat menolak pengajuan cuti")
		return
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var l model.LeaveRequest
	err = scanLeave(tx.QueryRow(ctx, `SELECT `+leaveColumns+` FROM leave_requests WHERE id = $1 FOR UPDATE`, id), &l)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Pengajuan cuti tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat pengajuan cuti")
		return
	}
	if !slices.Contains(from, l.Status) {
		writeError(w, http.StatusConflict, notAllowedMsg+" (status sekarang: "+l.Status+")")
		return
	}

	// Jatah cuti tahunan diperiksa DI SINI, bukan saat pengajuan dibuat: yang
	// benar-benar memakan jatah adalah cuti yang disetujui. Menahannya sejak
	// pengajuan hanya memindahkan percakapan "boleh tidak saya ambil" ke luar
	// sistem, dan membuat draft yang wajar jadi tidak bisa disimpan.
	if to == "APPROVED" {
		if msg, err := h.annualQuotaCheck(ctx, l); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memeriksa jatah cuti tahunan")
			return
		} else if msg != "" {
			writeError(w, http.StatusConflict, msg)
			return
		}
	}

	if l.Status == "APPROVED" && (to == "CANCELLED" || to == "REJECTED") {
		period, err := postedPayrollPeriodInRange(ctx, tx, l.CompanyID, l.StartDate, l.EndDate)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memeriksa payroll terkait")
			return
		}
		if period != "" {
			writeError(w, http.StatusConflict, "Payroll periode "+period+" sudah diposting ke GL, cuti yang sudah disetujui pada periode itu tidak bisa dicabut lagi")
			return
		}
	}

	setClause := `status = $1, updated_at = now()`
	switch to {
	case "SUBMITTED":
		setClause += `, submitted_at = now()`
	case "APPROVED":
		setClause += `, decided_at = now(), decided_by = $3, rejection_reason = NULL`
	case "REJECTED":
		setClause += `, decided_at = now(), decided_by = $3, rejection_reason = $4`
	}

	args := []any{to, id}
	if to == "APPROVED" || to == "REJECTED" {
		args = append(args, actor)
	}
	if to == "REJECTED" {
		args = append(args, strings.TrimSpace(req.RejectionReason))
	}

	err = scanLeave(tx.QueryRow(ctx, `UPDATE leave_requests SET `+setClause+` WHERE id = $2 RETURNING `+leaveColumns, args...), &l)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status pengajuan cuti")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan perubahan pengajuan cuti")
		return
	}

	h.events.Publish(eventType, newAuditEvent(eventType, actor, &l.CompanyID, "update", "leave_request", l.ID, l))
	writeJSON(w, http.StatusOK, l)
}

// hasOverlappingLeave mengecek tumpang tindih terhadap pengajuan yang masih
// "hidup" (DRAFT/SUBMITTED/APPROVED). Yang REJECTED/CANCELLED sengaja
// diabaikan supaya rentang yang sudah ditolak bisa diajukan ulang.
func (h *Handler) hasOverlappingLeave(ctx context.Context, employeeID string, start, end time.Time, excludeID string) (bool, error) {
	var exists bool
	err := h.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM leave_requests
			WHERE employee_id = $1
			  AND status IN ('DRAFT', 'SUBMITTED', 'APPROVED')
			  AND start_date <= $3 AND end_date >= $2
			  AND ($4 = '' OR id <> $4::uuid)
		)`, employeeID, start, end, excludeID).Scan(&exists)
	return exists, err
}

// activeEmployee memuat karyawan dan memastikan dia milik company yang benar
// dan berstatus ACTIVE. Mengembalikan (employee, pesan error, status HTTP).
func (h *Handler) activeEmployee(ctx context.Context, employeeID, companyID string) (model.Employee, string, int) {
	var emp model.Employee
	err := scanEmployee(h.pool.QueryRow(ctx, `SELECT `+employeeColumns+` FROM employees WHERE id = $1`, employeeID), &emp)
	if err == pgx.ErrNoRows {
		return emp, "Karyawan tidak ditemukan", http.StatusBadRequest
	} else if err != nil {
		return emp, "Gagal memuat data karyawan", http.StatusInternalServerError
	}
	if emp.CompanyID != companyID {
		return emp, "Karyawan tersebut milik company lain", http.StatusBadRequest
	}
	if emp.Status != "ACTIVE" {
		return emp, "Karyawan tidak berstatus ACTIVE (status sekarang: " + emp.Status + ")", http.StatusConflict
	}
	return emp, "", http.StatusOK
}

// postedPayrollPeriodInRange mengembalikan periode payroll POSTED pertama yang
// beririsan dengan rentang tanggal, atau "" kalau tidak ada. Perbandingannya
// memakai akhir periode eksklusif (awal bulan + 1 bulan) supaya tidak perlu
// menghitung jumlah hari tiap bulan.
func postedPayrollPeriodInRange(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, companyID string, start, end time.Time) (string, error) {
	var period string
	err := q.QueryRow(ctx, `
		SELECT period FROM payroll_runs
		WHERE company_id = $1 AND status = 'POSTED'
		  AND to_date(period, 'YYYY-MM') <= $3
		  AND (to_date(period, 'YYYY-MM') + INTERVAL '1 month') > $2
		ORDER BY period LIMIT 1`, companyID, start, end).Scan(&period)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return period, err
}

func nullIfEmpty(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}
