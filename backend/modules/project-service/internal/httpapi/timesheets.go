package httpapi

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/project-service/internal/financeclient"
	"github.com/enterprise-digital-platform/project-service/internal/model"
)

const timesheetColumns = `id, company_id, branch_id, project_id, task_id, employee_id, employee_name, work_date, hours, hourly_rate, amount, description, status, approved_at, posted_at, journal_entry_id, created_by_user_id, created_at, updated_at`

// monthlyHoursDivisor: konversi gaji bulanan ke tarif per jam memakai pembagi
// 173, angka standar perhitungan upah sejam di Indonesia (1/173 x upah
// sebulan, Pasal 61 PP 35/2021). Dipakai HANYA sebagai default kalau
// hourly_rate tidak dikirim -- tarif yang dikirim eksplisit selalu menang,
// karena tarif billing proyek sering berbeda dari gaji karyawan.
const monthlyHoursDivisor = 173

func scanTimesheet(row pgx.Row, t *model.Timesheet) error {
	return row.Scan(&t.ID, &t.CompanyID, &t.BranchID, &t.ProjectID, &t.TaskID, &t.EmployeeID, &t.EmployeeName, &t.WorkDate, &t.Hours, &t.HourlyRate, &t.Amount, &t.Description, &t.Status, &t.ApprovedAt, &t.PostedAt, &t.JournalEntryID, &t.CreatedByUserID, &t.CreatedAt, &t.UpdatedAt)
}

func (h *Handler) listTimesheets(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + timesheetColumns + ` FROM timesheets WHERE company_id = $1`
	args := []any{companyID}
	if projectID := r.URL.Query().Get("project_id"); projectID != "" {
		args = append(args, projectID)
		query += ` AND project_id = $` + strconv.Itoa(len(args))
	}
	if status := r.URL.Query().Get("status"); status != "" {
		args = append(args, status)
		query += ` AND status = $` + strconv.Itoa(len(args))
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY work_date DESC, created_at DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data timesheet")
		return
	}
	defer rows.Close()

	timesheets := []model.Timesheet{}
	for rows.Next() {
		var t model.Timesheet
		if err := scanTimesheet(rows, &t); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data timesheet")
			return
		}
		timesheets = append(timesheets, t)
	}
	writeJSON(w, http.StatusOK, timesheets)
}

type timesheetRequest struct {
	CompanyID   string   `json:"company_id"`
	BranchID    *string  `json:"branch_id"`
	ProjectID   string   `json:"project_id"`
	TaskID      string   `json:"task_id"`
	EmployeeID  string   `json:"employee_id"`
	WorkDate    string   `json:"work_date"`
	Hours       float64  `json:"hours"`
	HourlyRate  *float64 `json:"hourly_rate"`
	Description string   `json:"description"`
}

// createTimesheet. Proyek WAJIB ACTIVE -- jam kerja pada proyek yang masih
// PLANNING atau sudah ON_HOLD/COMPLETED/CANCELLED tidak punya arti biaya yang
// bisa dipertanggungjawabkan. Ini lebih ketat daripada assertProjectOpen yang
// dipakai tugas (tugas boleh disusun saat perencanaan).
func (h *Handler) createTimesheet(w http.ResponseWriter, r *http.Request) {
	var req timesheetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.ProjectID == "" || req.EmployeeID == "" {
		writeError(w, http.StatusBadRequest, "company_id, project_id, dan employee_id wajib diisi")
		return
	}
	if req.Hours <= 0 || req.Hours > 24 {
		writeError(w, http.StatusBadRequest, "hours harus lebih dari 0 dan maksimal 24")
		return
	}
	if req.HourlyRate != nil && *req.HourlyRate < 0 {
		writeError(w, http.StatusBadRequest, "hourly_rate tidak boleh negatif")
		return
	}
	workDate := time.Now()
	if req.WorkDate != "" {
		parsed, err := time.Parse("2006-01-02", req.WorkDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "work_date harus format YYYY-MM-DD")
			return
		}
		workDate = parsed
	}

	ctx := r.Context()
	var projectStatus string
	err := h.pool.QueryRow(ctx, `SELECT status FROM projects WHERE id = $1 AND company_id = $2`, req.ProjectID, req.CompanyID).Scan(&projectStatus)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusBadRequest, "Proyek tidak ditemukan di company ini")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat proyek")
		return
	}
	if projectStatus != "ACTIVE" {
		writeError(w, http.StatusConflict, "Timesheet hanya bisa dicatat pada proyek ACTIVE (status sekarang: "+projectStatus+")")
		return
	}

	// task_id opsional, tapi kalau diisi harus tugas milik proyek yang sama --
	// tanpa cek ini, jam kerja bisa tercatat di proyek A dengan tugas proyek B
	// dan rekap per tugas jadi tidak bisa dipercaya.
	var taskID *string
	if req.TaskID != "" {
		var taskProjectID string
		err := h.pool.QueryRow(ctx, `SELECT project_id FROM tasks WHERE id = $1 AND company_id = $2`, req.TaskID, req.CompanyID).Scan(&taskProjectID)
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusBadRequest, "Tugas tidak ditemukan di company ini")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memuat tugas")
			return
		}
		if taskProjectID != req.ProjectID {
			writeError(w, http.StatusBadRequest, "Tugas tersebut milik proyek lain")
			return
		}
		taskID = &req.TaskID
	}

	employee, err := h.hr.GetEmployee(req.EmployeeID)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Gagal memuat karyawan dari hr-service: %v", err))
		return
	}
	if employee.CompanyID != req.CompanyID {
		writeError(w, http.StatusBadRequest, "Karyawan tersebut milik company lain")
		return
	}
	if employee.Status != "ACTIVE" {
		writeError(w, http.StatusConflict, "Karyawan tidak berstatus ACTIVE (status sekarang: "+employee.Status+")")
		return
	}

	hourlyRate := 0.0
	if req.HourlyRate != nil {
		hourlyRate = *req.HourlyRate
	} else {
		hourlyRate = round2(employee.BasicSalary / monthlyHoursDivisor)
	}
	amount := round2(req.Hours * hourlyRate)

	var t model.Timesheet
	err = scanTimesheet(h.pool.QueryRow(ctx, `
		INSERT INTO timesheets (company_id, branch_id, project_id, task_id, employee_id, employee_name, work_date, hours, hourly_rate, amount, description, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+timesheetColumns,
		req.CompanyID, req.BranchID, req.ProjectID, taskID, req.EmployeeID, employee.FullName(), workDate, req.Hours, hourlyRate, amount, req.Description, actorFromHeader(r),
	), &t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mencatat timesheet")
		return
	}

	h.events.Publish("project.timesheet.created", newAuditEvent("project.timesheet.created", actorFromHeader(r), &t.CompanyID, "create", "timesheet", t.ID, t))
	writeJSON(w, http.StatusCreated, t)
}

func (h *Handler) approveTimesheet(w http.ResponseWriter, r *http.Request) {
	h.transitionTimesheet(w, r, []string{"DRAFT"}, "APPROVED", "approved_at", "project.timesheet.approved", "Timesheet hanya bisa disetujui dari status DRAFT")
}

// rejectTimesheet boleh dari APPROVED juga (persetujuan bisa ditarik selama
// belum diposting ke GL), tapi TIDAK dari POSTED -- biaya yang sudah masuk
// jurnal tidak bisa dihapus dengan mengubah status di sini; koreksinya lewat
// jurnal balik di finance-service.
func (h *Handler) rejectTimesheet(w http.ResponseWriter, r *http.Request) {
	h.transitionTimesheet(w, r, []string{"DRAFT", "APPROVED"}, "REJECTED", "", "project.timesheet.rejected", "Timesheet yang sudah diposting ke GL tidak bisa ditolak")
}

func (h *Handler) transitionTimesheet(w http.ResponseWriter, r *http.Request, from []string, to, timestampColumn, eventType, notAllowedMsg string) {
	id := r.PathValue("id")
	ctx := r.Context()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var t model.Timesheet
	err = scanTimesheet(tx.QueryRow(ctx, `SELECT `+timesheetColumns+` FROM timesheets WHERE id = $1 FOR UPDATE`, id), &t)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Timesheet tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat timesheet")
		return
	}
	if !contains(from, t.Status) {
		writeError(w, http.StatusConflict, notAllowedMsg+" (status sekarang: "+t.Status+")")
		return
	}

	setClause := `status = $1, updated_at = now()`
	if timestampColumn != "" {
		setClause += `, ` + timestampColumn + ` = now()`
	}
	err = scanTimesheet(tx.QueryRow(ctx, `UPDATE timesheets SET `+setClause+` WHERE id = $2 RETURNING `+timesheetColumns, to, id), &t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status timesheet")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan perubahan timesheet")
		return
	}

	h.events.Publish(eventType, newAuditEvent(eventType, actorFromHeader(r), &t.CompanyID, "update", "timesheet", t.ID, t))
	writeJSON(w, http.StatusOK, t)
}

type postCostRequest struct {
	CompanyID        string `json:"company_id"`
	ExpenseAccountID string `json:"expense_account_id"`
	PayableAccountID string `json:"payable_account_id"`
	EntryDate        string `json:"entry_date"`
}

type postCostResponse struct {
	Project        model.Project `json:"project"`
	JournalEntryID string        `json:"journal_entry_id"`
	PostedCount    int           `json:"posted_count"`
	PostedAmount   float64       `json:"posted_amount"`
}

// postProjectCost adalah inti integrasi finance modul ini: semua timesheet
// APPROVED milik satu proyek dijadikan SATU journal entry di finance-service
// (debit beban proyek, kredit hutang gaji/akrual), lalu ditandai POSTED dan
// ditambahkan ke projects.actual_cost. Account ID-nya dikirim pemanggil, pola
// identik postPayrollRun di hr-service -- pemilihan akun COA adalah keputusan
// akuntansi, bukan sesuatu yang boleh ditebak service ini.
//
// Dua keputusan desain yang sengaja BERBEDA dari deliverDeliveryOrder di
// fleet-service (yang memanggil service lain SEBELUM membuka transaksi):
//
//  1. Baris timesheet dikunci `SELECT ... FOR UPDATE` DAN transaksinya tetap
//     terbuka selama panggilan HTTP ke finance-service. Menahan lock selama
//     panggilan jaringan memang bukan hal yang disukai, tapi di sini itulah
//     yang mencegah dua post-cost bersamaan sama-sama membaca himpunan
//     APPROVED yang sama lalu memposting biaya yang sama dua kali ke GL.
//     Kalau lock dilepas dulu seperti pola fleet-service, uang yang dobel
//     masuk jurnal tidak bisa diperbaiki otomatis.
//  2. Panggilan finance tetap dilakukan SEBELUM commit lokal (sama seperti
//     fleet-service): kalau finance-service gagal, transaksi lokal di-rollback
//     dan timesheet tetap APPROVED -- bisa dicoba lagi. Urutan sebaliknya akan
//     meninggalkan timesheet POSTED tanpa jurnal yang mendasarinya.
func (h *Handler) postProjectCost(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	actor := actorFromHeader(r)

	var req postCostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.ExpenseAccountID == "" || req.PayableAccountID == "" {
		writeError(w, http.StatusBadRequest, "company_id, expense_account_id, dan payable_account_id wajib diisi")
		return
	}
	entryDate := time.Now().Format("2006-01-02")
	if req.EntryDate != "" {
		if _, err := time.Parse("2006-01-02", req.EntryDate); err != nil {
			writeError(w, http.StatusBadRequest, "entry_date harus format YYYY-MM-DD")
			return
		}
		entryDate = req.EntryDate
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var p model.Project
	err = scanProject(tx.QueryRow(ctx, `SELECT `+projectColumns+` FROM projects WHERE id = $1 AND company_id = $2 FOR UPDATE`, projectID, req.CompanyID), &p)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Proyek tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat proyek")
		return
	}
	if p.Status != "ACTIVE" {
		writeError(w, http.StatusConflict, "Biaya hanya bisa diposting untuk proyek ACTIVE (status sekarang: "+p.Status+")")
		return
	}

	rows, err := tx.Query(ctx, `SELECT id, amount FROM timesheets WHERE project_id = $1 AND status = 'APPROVED' ORDER BY work_date FOR UPDATE`, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat timesheet yang disetujui")
		return
	}
	var ids []string
	var total float64
	for rows.Next() {
		var id string
		var amount float64
		if err := rows.Scan(&id, &amount); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "Gagal membaca timesheet")
			return
		}
		ids = append(ids, id)
		total += amount
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca timesheet")
		return
	}

	if len(ids) == 0 {
		writeError(w, http.StatusConflict, "Tidak ada timesheet berstatus APPROVED untuk diposting")
		return
	}
	total = round2(total)
	if total <= 0 {
		// Semua timesheet APPROVED bernilai 0 (mis. tarifnya belum diisi):
		// jurnal nol tidak diterima finance-service (dan memang tidak berarti
		// apa-apa), jadi ditolak di sini dengan pesan yang jelas.
		writeError(w, http.StatusConflict, "Total biaya timesheet yang disetujui adalah 0; periksa tarif per jam sebelum memposting")
		return
	}

	entry, err := h.finance.CreateAndPostJournalEntry(headerValue(actor), financeclient.CreateJournalEntryRequest{
		CompanyID:     req.CompanyID,
		BranchID:      p.BranchID,
		EntryDate:     entryDate,
		Description:   "Biaya proyek " + p.ProjectCode + " - " + p.Name,
		ReferenceType: "project_cost",
		ReferenceID:   &p.ID,
		Lines: []financeclient.JournalLineInput{
			{AccountID: req.ExpenseAccountID, DebitAmount: total, Description: "Beban proyek " + p.ProjectCode},
			{AccountID: req.PayableAccountID, CreditAmount: total, Description: "Hutang biaya proyek " + p.ProjectCode},
		},
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Gagal memposting jurnal ke finance-service: %v", err))
		return
	}

	if _, err := tx.Exec(ctx, `UPDATE timesheets SET status = 'POSTED', posted_at = now(), journal_entry_id = $1, updated_at = now() WHERE id = ANY($2)`, entry.ID, ids); err != nil {
		writeError(w, http.StatusInternalServerError, "Jurnal sudah diposting di finance-service, tetapi gagal menandai timesheet POSTED")
		return
	}
	err = scanProject(tx.QueryRow(ctx, `UPDATE projects SET actual_cost = actual_cost + $1, updated_at = now() WHERE id = $2 RETURNING `+projectColumns, total, projectID), &p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Jurnal sudah diposting di finance-service, tetapi gagal memperbarui realisasi biaya proyek")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Jurnal sudah diposting di finance-service, tetapi gagal menyimpan perubahan lokal")
		return
	}

	h.events.Publish("project.cost.posted", newAuditEvent("project.cost.posted", actor, &p.CompanyID, "update", "project", p.ID, map[string]any{
		"project_id":       p.ID,
		"journal_entry_id": entry.ID,
		"posted_count":     len(ids),
		"posted_amount":    total,
	}))
	writeJSON(w, http.StatusOK, postCostResponse{Project: p, JournalEntryID: entry.ID, PostedCount: len(ids), PostedAmount: total})
}

// round2 membulatkan ke 2 desimal supaya nilai yang dikirim ke finance-service
// cocok dengan kolom NUMERIC(18,2) di kedua sisi -- tanpa ini, sisa presisi
// float64 bisa membuat total debit dan kredit dianggap tidak balance.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
