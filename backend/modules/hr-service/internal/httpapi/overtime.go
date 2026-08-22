package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/hr-service/internal/model"
)

// monthlyHoursDivisor: pembagi upah sebulan menjadi upah sejam (1/173), Pasal
// 61 PP 35/2021. Dipakai HANYA sebagai default kalau hourly_rate tidak dikirim.
const monthlyHoursDivisor = 173

const overtimeColumns = `id, company_id, branch_id, employee_id, employee_name, work_date, hours, is_holiday,
	hourly_rate, amount, COALESCE(description, ''), status, COALESCE(rejection_reason, ''), decided_at, decided_by,
	payroll_run_id, created_by, created_at, updated_at`

func scanOvertime(row pgx.Row, o *model.OvertimeLog) error {
	return row.Scan(&o.ID, &o.CompanyID, &o.BranchID, &o.EmployeeID, &o.EmployeeName, &o.WorkDate, &o.Hours,
		&o.IsHoliday, &o.HourlyRate, &o.Amount, &o.Description, &o.Status, &o.RejectionReason, &o.DecidedAt,
		&o.DecidedBy, &o.PayrollRunID, &o.CreatedBy, &o.CreatedAt, &o.UpdatedAt)
}

func (h *Handler) listOvertime(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	query := `SELECT ` + overtimeColumns + ` FROM overtime_logs WHERE company_id = $1`
	args := []any{companyID}

	if employeeID := r.URL.Query().Get("employee_id"); employeeID != "" {
		args = append(args, employeeID)
		query += ` AND employee_id = $` + strconv.Itoa(len(args))
	}
	if status := r.URL.Query().Get("status"); status != "" {
		args = append(args, strings.ToUpper(status))
		query += ` AND status = $` + strconv.Itoa(len(args))
	}
	if period := r.URL.Query().Get("period"); period != "" {
		args = append(args, period)
		query += ` AND to_char(work_date, 'YYYY-MM') = $` + strconv.Itoa(len(args))
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY work_date DESC, created_at DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data lembur")
		return
	}
	defer rows.Close()

	logs := []model.OvertimeLog{}
	for rows.Next() {
		var o model.OvertimeLog
		if err := scanOvertime(rows, &o); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data lembur")
			return
		}
		logs = append(logs, o)
	}
	writeJSON(w, http.StatusOK, logs)
}

type overtimePayload struct {
	CompanyID   string   `json:"company_id"`
	BranchID    *string  `json:"branch_id"`
	EmployeeID  string   `json:"employee_id"`
	WorkDate    string   `json:"work_date"` // YYYY-MM-DD
	Hours       float64  `json:"hours"`
	IsHoliday   bool     `json:"is_holiday"`
	HourlyRate  *float64 `json:"hourly_rate"`
	Description string   `json:"description"`
}

func (h *Handler) createOvertime(w http.ResponseWriter, r *http.Request) {
	var req overtimePayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.EmployeeID == "" || req.WorkDate == "" {
		writeError(w, http.StatusBadRequest, "company_id, employee_id, dan work_date wajib diisi")
		return
	}
	if req.Hours <= 0 || req.Hours > 12 {
		writeError(w, http.StatusBadRequest, "hours harus lebih dari 0 dan maksimal 12")
		return
	}
	if req.HourlyRate != nil && *req.HourlyRate < 0 {
		writeError(w, http.StatusBadRequest, "hourly_rate tidak boleh negatif")
		return
	}
	workDate, err := time.Parse("2006-01-02", req.WorkDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "work_date harus format YYYY-MM-DD")
		return
	}

	ctx := r.Context()
	emp, errMsg, status := h.activeEmployee(ctx, req.EmployeeID, req.CompanyID)
	if errMsg != "" {
		writeError(w, status, errMsg)
		return
	}

	// Mencatat lembur di periode yang payroll-nya sudah diposting tidak ada
	// gunanya: gross periode itu sudah masuk GL dan tidak akan dihitung ulang.
	period, err := postedPayrollPeriodInRange(ctx, h.pool, req.CompanyID, workDate, workDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa payroll terkait")
		return
	}
	if period != "" {
		writeError(w, http.StatusConflict, "Payroll periode "+period+" sudah diposting ke GL, lembur pada periode itu tidak bisa dicatat lagi")
		return
	}

	hourlyRate := round2(emp.BasicSalary / monthlyHoursDivisor)
	if req.HourlyRate != nil {
		hourlyRate = round2(*req.HourlyRate)
	}

	// is_holiday tidak lagi bergantung pada checkbox saja: akhir pekan dan
	// tanggal merah di kalender company otomatis dihitung sebagai hari libur
	// (pengali Kepmenaker 102/2004 yang lebih tinggi). Nilai true dari pemanggil
	// tetap dihormati -- itu jalan keluar untuk kasus yang tidak ada di
	// kalender, mis. libur pengganti yang belum sempat didata.
	isHoliday := req.IsHoliday
	if !isHoliday {
		isHoliday, err = h.isNonWorkingDay(ctx, req.CompanyID, workDate)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memeriksa kalender hari libur")
			return
		}
	}
	amount := calculateOvertimeAmount(req.Hours, hourlyRate, isHoliday)

	var o model.OvertimeLog
	err = scanOvertime(h.pool.QueryRow(ctx, `
		INSERT INTO overtime_logs (company_id, branch_id, employee_id, employee_name, work_date, hours, is_holiday, hourly_rate, amount, description, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+overtimeColumns,
		req.CompanyID, req.BranchID, req.EmployeeID, employeeFullName(emp), workDate, req.Hours, isHoliday,
		hourlyRate, amount, nullIfEmpty(req.Description), actorFromHeader(r),
	), &o)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Sudah ada catatan lembur untuk karyawan ini di tanggal tersebut")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal mencatat lembur")
		return
	}

	h.events.Publish("hr.overtime.created", newAuditEvent("hr.overtime.created", actorFromHeader(r), &o.CompanyID, "create", "overtime_log", o.ID, o))
	writeJSON(w, http.StatusCreated, o)
}

// updateOvertime hanya boleh saat DRAFT, dengan alasan yang sama seperti cuti:
// nilai yang sudah disetujui adalah nilai yang akan dibayar.
func (h *Handler) updateOvertime(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req overtimePayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.Hours <= 0 || req.Hours > 12 {
		writeError(w, http.StatusBadRequest, "hours harus lebih dari 0 dan maksimal 12")
		return
	}
	if req.HourlyRate != nil && *req.HourlyRate < 0 {
		writeError(w, http.StatusBadRequest, "hourly_rate tidak boleh negatif")
		return
	}

	ctx := r.Context()
	var existing model.OvertimeLog
	err := scanOvertime(h.pool.QueryRow(ctx, `SELECT `+overtimeColumns+` FROM overtime_logs WHERE id = $1`, id), &existing)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Catatan lembur tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat catatan lembur")
		return
	}
	if existing.Status != "DRAFT" {
		writeError(w, http.StatusConflict, "Hanya lembur berstatus DRAFT yang bisa diubah (status sekarang: "+existing.Status+")")
		return
	}

	hourlyRate := existing.HourlyRate
	if req.HourlyRate != nil {
		hourlyRate = round2(*req.HourlyRate)
	}

	// Aturan yang sama seperti saat mencatat: kalender menentukan, centang
	// manual hanya bisa menambah (bukan membatalkan) status hari libur.
	isHoliday := req.IsHoliday
	if !isHoliday {
		isHoliday, err = h.isNonWorkingDay(ctx, existing.CompanyID, existing.WorkDate)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memeriksa kalender hari libur")
			return
		}
	}
	amount := calculateOvertimeAmount(req.Hours, hourlyRate, isHoliday)

	var o model.OvertimeLog
	err = scanOvertime(h.pool.QueryRow(ctx, `
		UPDATE overtime_logs SET hours = $1, is_holiday = $2, hourly_rate = $3, amount = $4, description = $5, updated_at = now()
		WHERE id = $6
		RETURNING `+overtimeColumns,
		req.Hours, isHoliday, hourlyRate, amount, nullIfEmpty(req.Description), id,
	), &o)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui catatan lembur")
		return
	}

	h.events.Publish("hr.overtime.updated", newAuditEvent("hr.overtime.updated", actorFromHeader(r), &o.CompanyID, "update", "overtime_log", o.ID, o))
	writeJSON(w, http.StatusOK, o)
}

func (h *Handler) approveOvertime(w http.ResponseWriter, r *http.Request) {
	h.transitionOvertime(w, r, []string{"DRAFT"}, "APPROVED", "hr.overtime.approved", "Hanya lembur DRAFT yang bisa disetujui")
}

// rejectOvertime boleh dari APPROVED juga selama lembur itu belum ikut sebuah
// payroll run (payroll_run_id masih NULL) -- lihat transitionOvertime.
func (h *Handler) rejectOvertime(w http.ResponseWriter, r *http.Request) {
	h.transitionOvertime(w, r, []string{"DRAFT", "APPROVED"}, "REJECTED", "hr.overtime.rejected", "Lembur ini sudah tidak bisa ditolak")
}

func (h *Handler) transitionOvertime(w http.ResponseWriter, r *http.Request, from []string, to, eventType, notAllowedMsg string) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	var req leaveDecisionRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if to == "REJECTED" && strings.TrimSpace(req.RejectionReason) == "" {
		writeError(w, http.StatusBadRequest, "rejection_reason wajib diisi saat menolak lembur")
		return
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var o model.OvertimeLog
	err = scanOvertime(tx.QueryRow(ctx, `SELECT `+overtimeColumns+` FROM overtime_logs WHERE id = $1 FOR UPDATE`, id), &o)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Catatan lembur tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat catatan lembur")
		return
	}
	if !slices.Contains(from, o.Status) {
		writeError(w, http.StatusConflict, notAllowedMsg+" (status sekarang: "+o.Status+")")
		return
	}
	// payroll_run_id adalah pagar yang paling tegas: begitu nilai lembur ini
	// ikut terhitung di sebuah payroll run, statusnya tidak boleh berubah lagi.
	if o.PayrollRunID != nil {
		writeError(w, http.StatusConflict, "Lembur ini sudah ikut terhitung di payroll run, statusnya tidak bisa diubah lagi")
		return
	}
	period, err := postedPayrollPeriodInRange(ctx, tx, o.CompanyID, o.WorkDate, o.WorkDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa payroll terkait")
		return
	}
	if period != "" {
		writeError(w, http.StatusConflict, "Payroll periode "+period+" sudah diposting ke GL, status lembur pada periode itu tidak bisa diubah lagi")
		return
	}

	setClause := `status = $1, decided_at = now(), decided_by = $3, updated_at = now()`
	args := []any{to, id, actor}
	if to == "REJECTED" {
		setClause += `, rejection_reason = $4`
		args = append(args, strings.TrimSpace(req.RejectionReason))
	} else {
		setClause += `, rejection_reason = NULL`
	}

	err = scanOvertime(tx.QueryRow(ctx, `UPDATE overtime_logs SET `+setClause+` WHERE id = $2 RETURNING `+overtimeColumns, args...), &o)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status lembur")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan perubahan lembur")
		return
	}

	h.events.Publish(eventType, newAuditEvent(eventType, actor, &o.CompanyID, "update", "overtime_log", o.ID, o))
	writeJSON(w, http.StatusOK, o)
}

// calculateOvertimeAmount memakai pengali upah lembur Kepmenaker 102/2004
// Pasal 11 untuk pola kerja 5 hari seminggu:
//
//   - Hari kerja  : jam ke-1 dibayar 1,5x, jam berikutnya 2x.
//   - Hari libur  : jam ke-1 s/d 8 dibayar 2x, jam ke-9 3x, jam ke-10 dst 4x.
//
// Jam pecahan (mis. 1,5 jam) ikut dihitung proporsional pada tier tempat jam
// itu jatuh -- aturan aslinya berbasis jam penuh, tapi menolak input pecahan
// akan membuat pencatatan lembur 30 menit mustahil.
func calculateOvertimeAmount(hours, hourlyRate float64, isHoliday bool) float64 {
	if hours <= 0 || hourlyRate <= 0 {
		return 0
	}
	return round2(overtimeMultipliedHours(hours, isHoliday) * hourlyRate)
}

// overtimeMultipliedHours mengubah jam lembur menjadi "jam ekuivalen upah"
// (jam x pengali), dipisah dari perkalian tarif supaya pembagian tier-nya bisa
// diuji langsung tanpa ikut membawa pembulatan rupiah.
func overtimeMultipliedHours(hours float64, isHoliday bool) float64 {
	if hours <= 0 {
		return 0
	}
	if isHoliday {
		first8 := math.Min(hours, 8)
		ninth := math.Min(math.Max(hours-8, 0), 1)
		rest := math.Max(hours-9, 0)
		return first8*2 + ninth*3 + rest*4
	}
	first := math.Min(hours, 1)
	rest := math.Max(hours-1, 0)
	return first*1.5 + rest*2
}
