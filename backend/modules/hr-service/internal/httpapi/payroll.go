package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/hr-service/internal/financeclient"
	"github.com/enterprise-digital-platform/hr-service/internal/model"
)

const payrollRunColumns = `id, company_id, branch_id, period, status, total_employees, total_gross, total_pph21,
	total_bpjs, total_deduction, total_net, journal_id, posted_by, posted_at, created_at`

func scanPayrollRun(row pgx.Row, run *model.PayrollRun) error {
	return row.Scan(&run.ID, &run.CompanyID, &run.BranchID, &run.Period, &run.Status, &run.TotalEmployees,
		&run.TotalGross, &run.TotalPPh21, &run.TotalBPJS, &run.TotalDeduction, &run.TotalNet,
		&run.JournalID, &run.PostedBy, &run.PostedAt, &run.CreatedAt)
}

func (h *Handler) listPayrollRuns(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	query := `SELECT ` + payrollRunColumns + ` FROM payroll_runs WHERE company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY period DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data payroll")
		return
	}
	defer rows.Close()

	runs := []model.PayrollRun{}
	for rows.Next() {
		var run model.PayrollRun
		if err := scanPayrollRun(rows, &run); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data payroll")
			return
		}
		runs = append(runs, run)
	}
	writeJSON(w, http.StatusOK, runs)
}

type processPayrollRequest struct {
	CompanyID string  `json:"company_id"`
	BranchID  *string `json:"branch_id"`
	Period    string  `json:"period"` // YYYY-MM
}

// processPayroll menghitung payroll untuk seluruh karyawan ACTIVE di company
// pada periode tertentu dan menyimpannya sebagai payroll_run berstatus DRAFT.
// Perhitungan mengikuti pola calculatePayroll() di 20_Implementation_Guide.md
// (basic salary pro-rata berdasarkan kehadiran, PPh 21 & BPJS disederhanakan).
func (h *Handler) processPayroll(w http.ResponseWriter, r *http.Request) {
	var req processPayrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || len(req.Period) != 7 {
		writeError(w, http.StatusBadRequest, "company_id dan period (format YYYY-MM) wajib diisi")
		return
	}
	periodStart, err := time.Parse("2006-01", req.Period)
	if err != nil {
		writeError(w, http.StatusBadRequest, "period harus format YYYY-MM")
		return
	}

	ctx := r.Context()

	var exists bool
	if err := h.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM payroll_runs WHERE company_id = $1 AND period = $2)`, req.CompanyID, req.Period).Scan(&exists); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa payroll run")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "Payroll untuk periode ini sudah pernah diproses")
		return
	}

	employees, err := h.activeEmployees(ctx, req.CompanyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data karyawan")
		return
	}
	if len(employees) == 0 {
		writeError(w, http.StatusBadRequest, "Tidak ada karyawan aktif untuk diproses payroll-nya")
		return
	}

	workingDays := countWeekdays(periodStart)

	details := make([]model.PayrollDetail, 0, len(employees))
	var totalGross, totalPPh21, totalBPJS, totalNet float64
	for _, emp := range employees {
		presentDays, err := h.presentDays(ctx, emp.ID, req.Period)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal menghitung kehadiran")
			return
		}
		// Jika belum ada catatan absensi sama sekali untuk karyawan di periode
		// ini, anggap hadir penuh supaya payroll tetap bisa diproses tanpa
		// data absensi (mis. demo/awal implementasi).
		if presentDays < 0 {
			presentDays = workingDays
		}

		detail := calculatePayrollDetail(emp, workingDays, presentDays)
		details = append(details, detail)
		totalGross += detail.GrossSalary
		totalPPh21 += detail.PPh21
		totalBPJS += detail.BPJSKesehatanEmp + detail.BPJSTKJHTEmp + detail.BPJSTKJPEmp
		totalNet += detail.NetSalary
	}
	totalDeduction := totalPPh21 + totalBPJS

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var run model.PayrollRun
	err = tx.QueryRow(ctx, `
		INSERT INTO payroll_runs (company_id, branch_id, period, total_employees, total_gross, total_pph21, total_bpjs, total_deduction, total_net)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+payrollRunColumns,
		req.CompanyID, req.BranchID, req.Period, len(details), totalGross, totalPPh21, totalBPJS, totalDeduction, totalNet,
	).Scan(&run.ID, &run.CompanyID, &run.BranchID, &run.Period, &run.Status, &run.TotalEmployees,
		&run.TotalGross, &run.TotalPPh21, &run.TotalBPJS, &run.TotalDeduction, &run.TotalNet,
		&run.JournalID, &run.PostedBy, &run.PostedAt, &run.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat payroll run")
		return
	}

	for i := range details {
		d := &details[i]
		d.PayrollRunID = run.ID
		if err := tx.QueryRow(ctx, `
			INSERT INTO payroll_details (payroll_run_id, employee_id, employee_name, basic_salary, total_allowance,
			                             gross_salary, pph21, bpjs_kesehatan_emp, bpjs_tk_jht_emp, bpjs_tk_jp_emp,
			                             total_deduction, net_salary, working_days, present_days)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			RETURNING id, created_at`,
			d.PayrollRunID, d.EmployeeID, d.EmployeeName, d.BasicSalary, d.TotalAllowance, d.GrossSalary, d.PPh21,
			d.BPJSKesehatanEmp, d.BPJSTKJHTEmp, d.BPJSTKJPEmp, d.TotalDeduction, d.NetSalary, d.WorkingDays, d.PresentDays,
		).Scan(&d.ID, &d.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal menyimpan detail payroll")
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan payroll run")
		return
	}

	h.events.Publish("hr.payroll.processed", newAuditEvent("hr.payroll.processed", actorFromHeader(r), &run.CompanyID, "create", "payroll_run", run.ID, run))
	writeJSON(w, http.StatusCreated, payrollRunWithDetails{PayrollRun: run, Details: details})
}

type payrollRunWithDetails struct {
	model.PayrollRun
	Details []model.PayrollDetail `json:"details"`
}

func (h *Handler) getPayrollRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var run model.PayrollRun
	err := scanPayrollRun(h.pool.QueryRow(ctx, `SELECT `+payrollRunColumns+` FROM payroll_runs WHERE id = $1`, id), &run)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Payroll run tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat payroll run")
		return
	}

	rows, err := h.pool.Query(ctx, `
		SELECT id, payroll_run_id, employee_id, employee_name, basic_salary, total_allowance, gross_salary, pph21,
		       bpjs_kesehatan_emp, bpjs_tk_jht_emp, bpjs_tk_jp_emp, total_deduction, net_salary, working_days, present_days, created_at
		FROM payroll_details WHERE payroll_run_id = $1 ORDER BY employee_name ASC`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat detail payroll")
		return
	}
	defer rows.Close()

	details := []model.PayrollDetail{}
	for rows.Next() {
		var d model.PayrollDetail
		if err := rows.Scan(&d.ID, &d.PayrollRunID, &d.EmployeeID, &d.EmployeeName, &d.BasicSalary, &d.TotalAllowance,
			&d.GrossSalary, &d.PPh21, &d.BPJSKesehatanEmp, &d.BPJSTKJHTEmp, &d.BPJSTKJPEmp, &d.TotalDeduction,
			&d.NetSalary, &d.WorkingDays, &d.PresentDays, &d.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca detail payroll")
			return
		}
		details = append(details, d)
	}
	writeJSON(w, http.StatusOK, payrollRunWithDetails{PayrollRun: run, Details: details})
}

type postPayrollRunRequest struct {
	ExpenseAccountID       string `json:"expense_account_id"`
	SalaryPayableAccountID string `json:"salary_payable_account_id"`
	TaxPayableAccountID    string `json:"tax_payable_account_id"`
	BPJSPayableAccountID   string `json:"bpjs_payable_account_id"`
}

// postPayrollRun mem-posting payroll run DRAFT ke General Ledger finance-service
// lewat panggilan HTTP langsung (lihat internal/financeclient), lalu menandai
// run sebagai POSTED. Karena ini dua database terpisah tanpa distributed
// transaction, urutan disengaja: panggil finance-service dulu, baru update
// status lokal setelah sukses -- kalau finance-service gagal, run tetap DRAFT
// dan bisa dicoba post ulang.
func (h *Handler) postPayrollRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	var req postPayrollRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.ExpenseAccountID == "" || req.SalaryPayableAccountID == "" {
		writeError(w, http.StatusBadRequest, "expense_account_id dan salary_payable_account_id wajib diisi")
		return
	}

	var run model.PayrollRun
	err := scanPayrollRun(h.pool.QueryRow(ctx, `SELECT `+payrollRunColumns+` FROM payroll_runs WHERE id = $1`, id), &run)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Payroll run tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat payroll run")
		return
	}
	if run.Status != "DRAFT" {
		writeError(w, http.StatusConflict, "Payroll run sudah tidak berstatus DRAFT")
		return
	}
	if run.TotalPPh21 > 0 && req.TaxPayableAccountID == "" {
		writeError(w, http.StatusBadRequest, "tax_payable_account_id wajib diisi karena ada potongan PPh21")
		return
	}
	if run.TotalBPJS > 0 && req.BPJSPayableAccountID == "" {
		writeError(w, http.StatusBadRequest, "bpjs_payable_account_id wajib diisi karena ada potongan BPJS")
		return
	}

	lines := []financeclient.JournalLineInput{
		{AccountID: req.ExpenseAccountID, DebitAmount: run.TotalGross, Description: "Beban Gaji " + run.Period},
	}
	if run.TotalNet > 0 {
		lines = append(lines, financeclient.JournalLineInput{AccountID: req.SalaryPayableAccountID, CreditAmount: run.TotalNet, Description: "Hutang Gaji " + run.Period})
	}
	if run.TotalPPh21 > 0 {
		lines = append(lines, financeclient.JournalLineInput{AccountID: req.TaxPayableAccountID, CreditAmount: run.TotalPPh21, Description: "Hutang PPh21 " + run.Period})
	}
	if run.TotalBPJS > 0 {
		lines = append(lines, financeclient.JournalLineInput{AccountID: req.BPJSPayableAccountID, CreditAmount: run.TotalBPJS, Description: "Hutang BPJS " + run.Period})
	}

	runID := run.ID
	entry, err := h.finance.CreateAndPostJournalEntry(headerValue(actor), financeclient.CreateJournalEntryRequest{
		CompanyID:     run.CompanyID,
		BranchID:      run.BranchID,
		EntryDate:     run.Period + "-01",
		Description:   "Posting payroll " + run.Period,
		ReferenceType: "payroll",
		ReferenceID:   &runID,
		Lines:         lines,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Gagal posting ke finance-service: %v", err))
		return
	}

	err = scanPayrollRun(h.pool.QueryRow(ctx, `
		UPDATE payroll_runs SET status = 'POSTED', journal_id = $1, posted_by = $2, posted_at = now()
		WHERE id = $3
		RETURNING `+payrollRunColumns, entry.ID, actor, id), &run)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Jurnal berhasil dibuat di finance-service, tetapi gagal memperbarui status payroll run lokal")
		return
	}

	h.events.Publish("hr.payroll.posted", newAuditEvent("hr.payroll.posted", actor, &run.CompanyID, "post", "payroll_run", run.ID, run))
	writeJSON(w, http.StatusOK, run)
}

func headerValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (h *Handler) activeEmployees(ctx context.Context, companyID string) ([]model.Employee, error) {
	rows, err := h.pool.Query(ctx, `SELECT `+employeeColumns+` FROM employees WHERE company_id = $1 AND status = 'ACTIVE' ORDER BY employee_code ASC`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	employees := []model.Employee{}
	for rows.Next() {
		var e model.Employee
		if err := scanEmployee(rows, &e); err != nil {
			return nil, err
		}
		employees = append(employees, e)
	}
	return employees, rows.Err()
}

// presentDays menghitung jumlah hari hadir (PRESENT atau LATE) karyawan di
// suatu periode. Mengembalikan -1 jika belum ada catatan absensi sama sekali
// di periode tersebut (dibedakan dari 0 hari hadir yang sengaja tercatat absen).
func (h *Handler) presentDays(ctx context.Context, employeeID, period string) (int, error) {
	var total, present int
	err := h.pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE status IN ('PRESENT', 'LATE'))
		FROM attendance_logs WHERE employee_id = $1 AND to_char(log_date, 'YYYY-MM') = $2`,
		employeeID, period,
	).Scan(&total, &present)
	if err != nil {
		return 0, err
	}
	if total == 0 {
		return -1, nil
	}
	return present, nil
}

func countWeekdays(monthStart time.Time) int {
	count := 0
	for d := monthStart; d.Month() == monthStart.Month(); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			count++
		}
	}
	return count
}

// calculatePayrollDetail meniru calculatePayroll() di 20_Implementation_Guide.md:
// basic salary pro-rata terhadap kehadiran, allowance tetap, lalu potongan
// PPh21 (progresif disederhanakan, tanpa TER) dan BPJS (Kesehatan, JHT, JP
// porsi karyawan).
func calculatePayrollDetail(emp model.Employee, workingDays, presentDays int) model.PayrollDetail {
	ratio := 1.0
	if workingDays > 0 {
		ratio = float64(presentDays) / float64(workingDays)
	}
	basicSalary := emp.BasicSalary * ratio
	grossSalary := basicSalary + emp.MonthlyAllowance

	bpjsKesEmp := min(grossSalary*0.01, 120000)
	bpjsJhtEmp := grossSalary * 0.02
	bpjsJpEmp := min(grossSalary*0.01, 93600)

	pph21Annual := calculatePPh21Annual(grossSalary*12, emp.PTKPStatus)
	pph21Monthly := pph21Annual / 12

	totalDeduction := pph21Monthly + bpjsKesEmp + bpjsJhtEmp + bpjsJpEmp
	netSalary := grossSalary - totalDeduction

	return model.PayrollDetail{
		EmployeeID:       emp.ID,
		EmployeeName:     employeeFullName(emp),
		BasicSalary:      round2(basicSalary),
		TotalAllowance:   round2(emp.MonthlyAllowance),
		GrossSalary:      round2(grossSalary),
		PPh21:            round2(pph21Monthly),
		BPJSKesehatanEmp: round2(bpjsKesEmp),
		BPJSTKJHTEmp:     round2(bpjsJhtEmp),
		BPJSTKJPEmp:      round2(bpjsJpEmp),
		TotalDeduction:   round2(totalDeduction),
		NetSalary:        round2(netSalary),
		WorkingDays:      int16(workingDays),
		PresentDays:      int16(presentDays),
	}
}

func employeeFullName(emp model.Employee) string {
	if emp.LastName == "" {
		return emp.FirstName
	}
	return emp.FirstName + " " + emp.LastName
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

// ptkpAnnual adalah Penghasilan Tidak Kena Pajak per tahun sesuai status,
// mengikuti ketentuan umum PPh21 Indonesia (UU HPP).
var ptkpAnnual = map[string]float64{
	"TK/0": 54_000_000,
	"TK/1": 58_500_000,
	"TK/2": 63_000_000,
	"TK/3": 67_500_000,
	"K/0":  58_500_000,
	"K/1":  63_000_000,
	"K/2":  67_500_000,
	"K/3":  72_000_000,
}

// calculatePPh21Annual menghitung PPh21 tahunan dengan tarif progresif
// (bukan metode TER penuh -- disederhanakan sesuai catatan di
// 20_Implementation_Guide.md). Biaya jabatan 5% (maks Rp 6.000.000/tahun)
// dikurangkan sebelum PTKP. Kontribusi BPJS karyawan tidak diperhitungkan
// sebagai pengurang PKP di model sederhana ini.
func calculatePPh21Annual(grossAnnual float64, ptkpStatus string) float64 {
	biayaJabatan := grossAnnual * 0.05
	if biayaJabatan > 6_000_000 {
		biayaJabatan = 6_000_000
	}
	netAnnual := grossAnnual - biayaJabatan

	ptkp, ok := ptkpAnnual[ptkpStatus]
	if !ok {
		ptkp = ptkpAnnual["TK/0"]
	}
	pkp := netAnnual - ptkp
	if pkp <= 0 {
		return 0
	}
	return progressiveTax(pkp)
}

type taxBracket struct {
	upTo float64 // 0 berarti tidak terbatas (bracket terakhir)
	rate float64
}

var taxBrackets = []taxBracket{
	{60_000_000, 0.05},
	{250_000_000, 0.15},
	{500_000_000, 0.25},
	{5_000_000_000, 0.30},
	{0, 0.35},
}

func progressiveTax(pkp float64) float64 {
	var tax float64
	var lower float64
	for _, b := range taxBrackets {
		if b.upTo == 0 || pkp <= b.upTo {
			tax += (pkp - lower) * b.rate
			break
		}
		tax += (b.upTo - lower) * b.rate
		lower = b.upTo
	}
	return tax
}
