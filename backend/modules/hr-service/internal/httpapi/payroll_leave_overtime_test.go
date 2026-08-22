package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// Test-test di file ini menguji satu-satunya alasan cuti & lembur dibangun
// sebagai transaksi ber-status: pengaruhnya ke uang di payroll. Kalau salah
// satu di antaranya rusak, angka gaji ikut salah tanpa ada error yang terlihat.

// TestProcessPayroll_ApprovedOvertimeRaisesGross: lembur APPROVED menambah
// gross, lembur DRAFT tidak. Ini pembeda utama antara "sudah disetujui" dan
// "baru dicatat".
func TestProcessPayroll_ApprovedOvertimeRaisesGross(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, salaryFor50kRate, 0)

	approved := mustCreateOvertime(t, srv, companyID, emp.ID, "2026-08-04", 2, false) // 175.000
	requireStatus(t, postJSON(t, srv.URL+"/overtime/"+approved.ID+"/approve", nil), http.StatusOK)
	mustCreateOvertime(t, srv, companyID, emp.ID, "2026-08-05", 4, false) // DRAFT, harus diabaikan

	run := mustProcessPayroll(t, srv, companyID, "2026-08")
	d := run.Details[0]

	if d.OvertimePay != 175_000 {
		t.Errorf("overtime_pay = %.2f, want 175000.00 (hanya lembur APPROVED)", d.OvertimePay)
	}
	if d.OvertimeHours != 2 {
		t.Errorf("overtime_hours = %.2f, want 2", d.OvertimeHours)
	}
	if run.TotalOvertime != 175_000 {
		t.Errorf("run total_overtime = %.2f, want 175000.00", run.TotalOvertime)
	}
	wantGross := round2(d.BasicSalary + d.TotalAllowance + d.OvertimePay)
	if round2(d.GrossSalary) != wantGross {
		t.Errorf("gross_salary = %.2f, want basic+allowance+overtime = %.2f", d.GrossSalary, wantGross)
	}
}

// TestProcessPayroll_StampsAndLocksOvertime: lembur yang sudah ikut terhitung
// ditandai payroll_run_id dan statusnya terkunci -- kalau tidak, menolaknya
// setelah payroll diproses akan membuat gross tidak bisa direkonsiliasi lagi.
func TestProcessPayroll_StampsAndLocksOvertime(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, salaryFor50kRate, 0)
	o := mustCreateOvertime(t, srv, companyID, emp.ID, "2026-08-04", 2, false)
	requireStatus(t, postJSON(t, srv.URL+"/overtime/"+o.ID+"/approve", nil), http.StatusOK)

	run := mustProcessPayroll(t, srv, companyID, "2026-08")

	resp := getJSON(t, srv.URL+"/overtime?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var logs []overtimeView
	resp.decode(t, &logs)
	if len(logs) != 1 {
		t.Fatalf("expected 1 overtime row, got %d", len(logs))
	}
	if logs[0].PayrollRunID == nil || *logs[0].PayrollRunID != run.ID {
		t.Fatalf("payroll_run_id = %v, want %s", logs[0].PayrollRunID, run.ID)
	}

	rejected := postJSON(t, srv.URL+"/overtime/"+o.ID+"/reject", map[string]any{"rejection_reason": "salah input"})
	requireStatus(t, rejected, http.StatusConflict)
}

// TestProcessPayroll_PaidLeaveDoesNotCutSalary adalah inti integrasi cuti:
// hari yang tercatat LEAVE di absensi (jadi tidak terhitung hadir) harus
// dikembalikan sebagai hari dibayar kalau ada cuti berbayar yang disetujui.
// Tanpa ini, cuti tahunan diam-diam memotong gaji lewat pro-rata.
func TestProcessPayroll_PaidLeaveDoesNotCutSalary(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	// Dua karyawan dengan absensi identik: 3 hari PRESENT + 2 hari LEAVE.
	// Bedanya hanya satu -- yang kedua punya pengajuan cuti tahunan yang
	// disetujui untuk 2 hari itu.
	seedAttendance := func(employeeID string) {
		for _, day := range []struct {
			date   string
			status string
		}{
			{"2026-08-03", "PRESENT"}, {"2026-08-04", "PRESENT"}, {"2026-08-05", "PRESENT"},
			{"2026-08-06", "LEAVE"}, {"2026-08-07", "LEAVE"},
		} {
			requireStatus(t, postJSON(t, srv.URL+"/attendance", map[string]any{
				"company_id": companyID, "employee_id": employeeID, "log_date": day.date, "status": day.status,
			}), http.StatusCreated)
		}
	}

	withoutLeave := mustSeedEmployee(t, srv, companyID, 10_000_000, 0)
	seedAttendance(withoutLeave.ID)

	withLeave := mustSeedEmployee(t, srv, companyID, 10_000_000, 0)
	seedAttendance(withLeave.ID)
	l := mustCreateLeave(t, srv, companyID, withLeave.ID, "ANNUAL", "2026-08-06", "2026-08-07")
	mustApproveLeave(t, srv, l.ID)

	run := mustProcessPayroll(t, srv, companyID, "2026-08")
	byEmployee := map[string]payrollDetailView{}
	for _, d := range run.Details {
		byEmployee[d.EmployeeID] = d
	}

	plain := byEmployee[withoutLeave.ID]
	onLeave := byEmployee[withLeave.ID]

	if plain.PresentDays != 3 {
		t.Fatalf("karyawan tanpa cuti: present_days = %d, want 3", plain.PresentDays)
	}
	if onLeave.PresentDays != 5 {
		t.Fatalf("karyawan dengan cuti berbayar: present_days = %d, want 5 (3 hadir + 2 cuti dibayar)", onLeave.PresentDays)
	}
	if onLeave.PaidLeaveDays != 2 || onLeave.UnpaidLeaveDays != 0 {
		t.Errorf("paid/unpaid leave days = %d/%d, want 2/0", onLeave.PaidLeaveDays, onLeave.UnpaidLeaveDays)
	}
	if onLeave.BasicSalary <= plain.BasicSalary {
		t.Errorf("basic_salary dengan cuti berbayar = %.2f, harus LEBIH BESAR dari tanpa cuti = %.2f", onLeave.BasicSalary, plain.BasicSalary)
	}
}

// TestProcessPayroll_UnpaidLeaveCutsSalary: jalur sebaliknya, saat belum ada
// catatan absensi sama sekali (karyawan dianggap hadir penuh), cuti tanpa gaji
// yang disetujui harus mengurangi hari dibayar.
func TestProcessPayroll_UnpaidLeaveCutsSalary(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	full := mustSeedEmployee(t, srv, companyID, 10_000_000, 0)
	unpaid := mustSeedEmployee(t, srv, companyID, 10_000_000, 0)
	l := mustCreateLeave(t, srv, companyID, unpaid.ID, "UNPAID", "2026-08-03", "2026-08-07") // 5 hari kerja
	mustApproveLeave(t, srv, l.ID)

	run := mustProcessPayroll(t, srv, companyID, "2026-08")
	byEmployee := map[string]payrollDetailView{}
	for _, d := range run.Details {
		byEmployee[d.EmployeeID] = d
	}

	present := byEmployee[full.ID]
	cut := byEmployee[unpaid.ID]

	if present.PresentDays != present.WorkingDays {
		t.Fatalf("karyawan tanpa cuti: present_days = %d, want working_days = %d", present.PresentDays, present.WorkingDays)
	}
	if cut.PresentDays != cut.WorkingDays-5 {
		t.Errorf("present_days = %d, want working_days-5 = %d", cut.PresentDays, cut.WorkingDays-5)
	}
	if cut.UnpaidLeaveDays != 5 || cut.PaidLeaveDays != 0 {
		t.Errorf("paid/unpaid leave days = %d/%d, want 0/5", cut.PaidLeaveDays, cut.UnpaidLeaveDays)
	}
	if cut.BasicSalary >= present.BasicSalary {
		t.Errorf("basic_salary dengan cuti tanpa gaji = %.2f, harus LEBIH KECIL dari %.2f", cut.BasicSalary, present.BasicSalary)
	}
}

// TestProcessPayroll_LeaveCrossingMonthCountsPerPeriod: cuti yang menyeberang
// bulan hanya boleh memotong hari di bulan yang bersangkutan. Ini alasan
// leaveDaysInPeriod memecah rentang per hari dan tidak memakai total_days.
func TestProcessPayroll_LeaveCrossingMonthCountsPerPeriod(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 10_000_000, 0)

	// 30-31 Juli (Kam-Jum) + 3-4 Agustus (Sen-Sel); 1-2 Agustus akhir pekan.
	l := mustCreateLeave(t, srv, companyID, emp.ID, "UNPAID", "2026-07-30", "2026-08-04")
	mustApproveLeave(t, srv, l.ID)
	if l.TotalDays != 4 {
		t.Fatalf("total_days = %d, want 4 hari kerja", l.TotalDays)
	}

	july := mustProcessPayroll(t, srv, companyID, "2026-07")
	august := mustProcessPayroll(t, srv, companyID, "2026-08")

	if july.Details[0].UnpaidLeaveDays != 2 {
		t.Errorf("Juli unpaid_leave_days = %d, want 2", july.Details[0].UnpaidLeaveDays)
	}
	if august.Details[0].UnpaidLeaveDays != 2 {
		t.Errorf("Agustus unpaid_leave_days = %d, want 2", august.Details[0].UnpaidLeaveDays)
	}
}

// TestPostPayrollRun_SplitsOvertimeIntoOwnJournalLine memastikan komposisi
// biaya tenaga kerja terbaca dari jurnal, dan -- yang lebih penting -- jurnal
// tetap balance setelah beban dipecah dua baris.
func TestPostPayrollRun_SplitsOvertimeIntoOwnJournalLine(t *testing.T) {
	srv, calls := newServerWithFinanceStub(t, false)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, salaryFor50kRate, 0)
	o := mustCreateOvertime(t, srv, companyID, emp.ID, "2026-08-04", 2, false)
	requireStatus(t, postJSON(t, srv.URL+"/overtime/"+o.ID+"/approve", nil), http.StatusOK)

	run := mustProcessPayroll(t, srv, companyID, "2026-08")
	expenseAccount := uuid.NewString()
	requireStatus(t, postJSON(t, srv.URL+"/payroll-runs/"+run.ID+"/post", map[string]any{
		"expense_account_id": expenseAccount, "salary_payable_account_id": uuid.NewString(),
		"tax_payable_account_id": uuid.NewString(), "bpjs_payable_account_id": uuid.NewString(),
	}), http.StatusOK)

	if len(*calls) == 0 {
		t.Fatal("finance-service tidak dipanggil sama sekali")
	}
	var entry struct {
		Lines []struct {
			AccountID    string  `json:"account_id"`
			DebitAmount  float64 `json:"debit_amount"`
			CreditAmount float64 `json:"credit_amount"`
			Description  string  `json:"description"`
		} `json:"lines"`
	}
	if err := json.Unmarshal((*calls)[0].body, &entry); err != nil {
		t.Fatalf("decode journal entry request: %v", err)
	}

	var overtimeLine, totalDebit, totalCredit float64
	for _, line := range entry.Lines {
		totalDebit += line.DebitAmount
		totalCredit += line.CreditAmount
		if line.Description == "Beban Lembur 2026-08" {
			overtimeLine = line.DebitAmount
			if line.AccountID != expenseAccount {
				t.Errorf("baris lembur memakai akun %s, want expense_account_id %s", line.AccountID, expenseAccount)
			}
		}
	}
	if overtimeLine != 175_000 {
		t.Errorf("baris 'Beban Lembur' = %.2f, want 175000.00", overtimeLine)
	}
	if round2(totalDebit) != round2(run.TotalGross) {
		t.Errorf("total debit = %.2f, want total_gross = %.2f", totalDebit, run.TotalGross)
	}
	if round2(totalDebit) != round2(totalCredit) {
		t.Errorf("jurnal tidak balance: debit %.2f vs credit %.2f", totalDebit, totalCredit)
	}
}

// TestCreateOvertime_BlockedAfterPayrollPosted: mencatat lembur di periode yang
// payroll-nya sudah masuk GL tidak ada gunanya -- gross periode itu tidak akan
// dihitung ulang.
func TestCreateOvertime_BlockedAfterPayrollPosted(t *testing.T) {
	srv, _ := newServerWithFinanceStub(t, false)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, salaryFor50kRate, 0)

	run := mustProcessPayroll(t, srv, companyID, "2026-08")
	requireStatus(t, postJSON(t, srv.URL+"/payroll-runs/"+run.ID+"/post", map[string]any{
		"expense_account_id": uuid.NewString(), "salary_payable_account_id": uuid.NewString(),
		"tax_payable_account_id": uuid.NewString(), "bpjs_payable_account_id": uuid.NewString(),
	}), http.StatusOK)

	requireStatus(t, postJSON(t, srv.URL+"/overtime", map[string]any{
		"company_id": companyID, "employee_id": emp.ID, "work_date": "2026-08-04", "hours": 2,
	}), http.StatusConflict)

	// Periode lain tidak ikut terkunci.
	requireStatus(t, postJSON(t, srv.URL+"/overtime", map[string]any{
		"company_id": companyID, "employee_id": emp.ID, "work_date": "2026-09-02", "hours": 2,
	}), http.StatusCreated)
}
