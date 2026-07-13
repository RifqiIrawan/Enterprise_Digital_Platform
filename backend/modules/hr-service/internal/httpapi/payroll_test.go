package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type payrollDetailView struct {
	EmployeeID     string  `json:"employee_id"`
	BasicSalary    float64 `json:"basic_salary"`
	TotalAllowance float64 `json:"total_allowance"`
	GrossSalary    float64 `json:"gross_salary"`
	PPh21          float64 `json:"pph21"`
	TotalDeduction float64 `json:"total_deduction"`
	NetSalary      float64 `json:"net_salary"`
	WorkingDays    int     `json:"working_days"`
	PresentDays    int     `json:"present_days"`
}

type payrollRunView struct {
	ID             string              `json:"id"`
	CompanyID      string              `json:"company_id"`
	Period         string              `json:"period"`
	Status         string              `json:"status"`
	TotalEmployees int                 `json:"total_employees"`
	TotalGross     float64             `json:"total_gross"`
	TotalPPh21     float64             `json:"total_pph21"`
	TotalBPJS      float64             `json:"total_bpjs"`
	TotalDeduction float64             `json:"total_deduction"`
	TotalNet       float64             `json:"total_net"`
	JournalID      *string             `json:"journal_id"`
	Details        []payrollDetailView `json:"details"`
}

func TestProcessPayroll_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"missing company_id":  {"period": "2026-08"},
		"bad period format":   {"company_id": companyID, "period": "08/2026"},
		"period wrong length": {"company_id": companyID, "period": "2026-8"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/payroll-runs", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestProcessPayroll_NoActiveEmployees(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/payroll-runs", map[string]any{"company_id": companyID, "period": "2026-08"})
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestProcessPayroll_DuplicatePeriodConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	mustSeedEmployee(t, srv, companyID, 10_000_000, 1_000_000)

	first := postJSON(t, srv.URL+"/payroll-runs", map[string]any{"company_id": companyID, "period": "2026-08"})
	requireStatus(t, first, http.StatusCreated)

	second := postJSON(t, srv.URL+"/payroll-runs", map[string]any{"company_id": companyID, "period": "2026-08"})
	requireStatus(t, second, http.StatusConflict)
}

func TestProcessPayroll_FullPresenceWhenNoAttendanceLogged(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 10_000_000, 1_000_000)

	resp := postJSON(t, srv.URL+"/payroll-runs", map[string]any{"company_id": companyID, "period": "2026-08"})
	requireStatus(t, resp, http.StatusCreated)

	var run payrollRunView
	resp.decode(t, &run)

	if run.Status != "DRAFT" {
		t.Errorf("status = %q, want DRAFT", run.Status)
	}
	if run.TotalEmployees != 1 {
		t.Fatalf("total_employees = %d, want 1", run.TotalEmployees)
	}
	if len(run.Details) != 1 || run.Details[0].EmployeeID != emp.ID {
		t.Fatalf("expected 1 detail for the seeded employee, got %+v", run.Details)
	}
	d := run.Details[0]

	// No attendance logged at all => full presence assumed (see presentDays()
	// doc-comment in payroll.go: total==0 means "no records", not "0 present").
	if d.PresentDays != d.WorkingDays {
		t.Errorf("present_days = %d, want equal to working_days = %d (full presence assumed)", d.PresentDays, d.WorkingDays)
	}
	if d.BasicSalary != 10_000_000 {
		t.Errorf("basic_salary = %.2f, want 10000000.00 (no proration at full presence)", d.BasicSalary)
	}
	// Structural invariants, not golden values: avoid re-deriving the PPh21/BPJS
	// formulas here, just confirm the totals are internally consistent.
	if d.GrossSalary != d.BasicSalary+d.TotalAllowance {
		t.Errorf("gross_salary = %.2f, want basic+allowance = %.2f", d.GrossSalary, d.BasicSalary+d.TotalAllowance)
	}
	wantNet := round2(d.GrossSalary - d.TotalDeduction)
	if round2(d.NetSalary) != wantNet {
		t.Errorf("net_salary = %.2f, want gross - total_deduction = %.2f", d.NetSalary, wantNet)
	}
	if run.TotalGross != d.GrossSalary {
		t.Errorf("run total_gross = %.2f, want %.2f", run.TotalGross, d.GrossSalary)
	}
	if round2(run.TotalDeduction) != round2(run.TotalPPh21+run.TotalBPJS) {
		t.Errorf("run total_deduction = %.2f, want pph21+bpjs = %.2f", run.TotalDeduction, run.TotalPPh21+run.TotalBPJS)
	}
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

func TestProcessPayroll_ProratesBasicSalaryOnPartialAttendance(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 10_000_000, 1_000_000)

	// Log a single PRESENT day in the period: presentDays(1) will be far below
	// working days for any real month, so basic salary must be prorated down.
	requireStatus(t, postJSON(t, srv.URL+"/attendance", map[string]any{
		"company_id": companyID, "employee_id": emp.ID, "log_date": "2026-09-02", "status": "PRESENT",
	}), http.StatusCreated)

	resp := postJSON(t, srv.URL+"/payroll-runs", map[string]any{"company_id": companyID, "period": "2026-09"})
	requireStatus(t, resp, http.StatusCreated)
	var run payrollRunView
	resp.decode(t, &run)

	d := run.Details[0]
	if d.PresentDays != 1 {
		t.Fatalf("present_days = %d, want 1", d.PresentDays)
	}
	if d.WorkingDays <= 1 {
		t.Fatalf("working_days = %d, expected > 1 for a full month", d.WorkingDays)
	}
	if d.BasicSalary >= 10_000_000 {
		t.Errorf("basic_salary = %.2f, want prorated below full 10000000.00 (present_days=1 of %d)", d.BasicSalary, d.WorkingDays)
	}
	if d.BasicSalary <= 0 {
		t.Errorf("basic_salary = %.2f, want > 0", d.BasicSalary)
	}
}

func TestGetPayrollRun_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/payroll-runs/"+uuid.NewString())
	requireStatus(t, resp, http.StatusNotFound)
}

func mustProcessPayroll(t *testing.T, srv *httptest.Server, companyID, period string) payrollRunView {
	t.Helper()
	resp := postJSON(t, srv.URL+"/payroll-runs", map[string]any{"company_id": companyID, "period": period})
	requireStatus(t, resp, http.StatusCreated)
	var run payrollRunView
	resp.decode(t, &run)
	return run
}

// TestListPayrollRuns_FilteredByBranch confirms branch_id filtering is
// NULL-inclusive (see TestListEmployees_FilteredByBranch for rationale).
// Each payroll run needs its own period (UNIQUE company_id+period), so three
// different periods stand in for three separate runs tagged to different branches.
func TestListPayrollRuns_FilteredByBranch(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	mustSeedEmployee(t, srv, companyID, 5_000_000, 0)
	branchA := uuid.NewString()
	branchB := uuid.NewString()

	mkRun := func(period string, branchID *string) {
		requireStatus(t, postJSON(t, srv.URL+"/payroll-runs", map[string]any{
			"company_id": companyID, "branch_id": branchID, "period": period,
		}), http.StatusCreated)
	}
	mkRun("2027-01", &branchA)
	mkRun("2027-02", nil)
	mkRun("2027-03", &branchB)

	resp := getJSON(t, srv.URL+"/payroll-runs?company_id="+companyID+"&branch_id="+branchA)
	requireStatus(t, resp, http.StatusOK)
	var runs []struct {
		BranchID *string `json:"branch_id"`
	}
	resp.decode(t, &runs)
	if len(runs) != 2 {
		t.Fatalf("expected 2 payroll runs (branchA + NULL), got %d: %+v", len(runs), runs)
	}
	for _, run := range runs {
		if run.BranchID != nil && *run.BranchID == branchB {
			t.Errorf("branchB payroll run leaked into branchA-filtered results: %+v", runs)
		}
	}
}

func TestPostPayrollRun_ValidationErrors(t *testing.T) {
	srv, _ := newServerWithFinanceStub(t, false)
	companyID := newCompanyID(t)
	mustSeedEmployee(t, srv, companyID, 10_000_000, 1_000_000)
	run := mustProcessPayroll(t, srv, companyID, "2026-10")

	cases := map[string]map[string]any{
		"missing expense_account_id": {
			"salary_payable_account_id": uuid.NewString(), "tax_payable_account_id": uuid.NewString(), "bpjs_payable_account_id": uuid.NewString(),
		},
		"missing salary_payable_account_id": {
			"expense_account_id": uuid.NewString(), "tax_payable_account_id": uuid.NewString(), "bpjs_payable_account_id": uuid.NewString(),
		},
		"missing tax_payable_account_id (pph21 > 0)": {
			"expense_account_id": uuid.NewString(), "salary_payable_account_id": uuid.NewString(), "bpjs_payable_account_id": uuid.NewString(),
		},
		"missing bpjs_payable_account_id (bpjs > 0)": {
			"expense_account_id": uuid.NewString(), "salary_payable_account_id": uuid.NewString(), "tax_payable_account_id": uuid.NewString(),
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/payroll-runs/"+run.ID+"/post", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestPostPayrollRun_NotFound(t *testing.T) {
	srv, _ := newServerWithFinanceStub(t, false)
	resp := postJSON(t, srv.URL+"/payroll-runs/"+uuid.NewString()+"/post", map[string]any{
		"expense_account_id": uuid.NewString(), "salary_payable_account_id": uuid.NewString(),
	})
	requireStatus(t, resp, http.StatusNotFound)
}

func TestPostPayrollRun_Success(t *testing.T) {
	srv, calls := newServerWithFinanceStub(t, false)
	companyID := newCompanyID(t)
	mustSeedEmployee(t, srv, companyID, 10_000_000, 1_000_000)
	run := mustProcessPayroll(t, srv, companyID, "2026-11")

	expenseAcc := uuid.NewString()
	salaryAcc := uuid.NewString()
	taxAcc := uuid.NewString()
	bpjsAcc := uuid.NewString()

	resp := postJSON(t, srv.URL+"/payroll-runs/"+run.ID+"/post", map[string]any{
		"expense_account_id": expenseAcc, "salary_payable_account_id": salaryAcc,
		"tax_payable_account_id": taxAcc, "bpjs_payable_account_id": bpjsAcc,
	})
	requireStatus(t, resp, http.StatusOK)

	var posted payrollRunView
	resp.decode(t, &posted)
	if posted.Status != "POSTED" {
		t.Errorf("status = %q, want POSTED", posted.Status)
	}
	if posted.JournalID == nil {
		t.Fatal("expected journal_id to be set after posting")
	}

	if len(*calls) != 2 {
		t.Fatalf("expected 2 calls to finance-service (create + post), got %d: %+v", len(*calls), *calls)
	}
	createCall := (*calls)[0]
	if createCall.path != "/journal-entries" {
		t.Errorf("first call path = %q, want /journal-entries", createCall.path)
	}

	var sent struct {
		CompanyID     string `json:"company_id"`
		EntryDate     string `json:"entry_date"`
		ReferenceType string `json:"reference_type"`
		Lines         []struct {
			AccountID    string  `json:"account_id"`
			DebitAmount  float64 `json:"debit_amount"`
			CreditAmount float64 `json:"credit_amount"`
		} `json:"lines"`
	}
	if err := json.Unmarshal(createCall.body, &sent); err != nil {
		t.Fatalf("decode journal entry sent to finance-service: %v", err)
	}
	if sent.CompanyID != companyID {
		t.Errorf("sent company_id = %q, want %q", sent.CompanyID, companyID)
	}
	if sent.EntryDate != "2026-11-01" {
		t.Errorf("sent entry_date = %q, want 2026-11-01", sent.EntryDate)
	}
	if sent.ReferenceType != "payroll" {
		t.Errorf("sent reference_type = %q, want payroll", sent.ReferenceType)
	}
	// Every employee has gross > 0 => pph21 > 0 and bpjs > 0 in this scenario,
	// so all 4 lines (expense, salary payable, tax payable, bpjs payable) must
	// be present and balanced.
	if len(sent.Lines) != 4 {
		t.Fatalf("expected 4 journal lines, got %d: %+v", len(sent.Lines), sent.Lines)
	}
	var totalDebit, totalCredit float64
	var sawExpenseDebit, sawSalaryCredit, sawTaxCredit, sawBPJSCredit float64
	for _, l := range sent.Lines {
		totalDebit += l.DebitAmount
		totalCredit += l.CreditAmount
		switch l.AccountID {
		case expenseAcc:
			sawExpenseDebit = l.DebitAmount
		case salaryAcc:
			sawSalaryCredit = l.CreditAmount
		case taxAcc:
			sawTaxCredit = l.CreditAmount
		case bpjsAcc:
			sawBPJSCredit = l.CreditAmount
		}
	}
	if round2(totalDebit) != round2(totalCredit) {
		t.Errorf("journal sent to finance-service is unbalanced: debit %.2f, credit %.2f", totalDebit, totalCredit)
	}
	if sawExpenseDebit != run.TotalGross {
		t.Errorf("expense account debit = %.2f, want total_gross %.2f", sawExpenseDebit, run.TotalGross)
	}
	if sawSalaryCredit != run.TotalNet {
		t.Errorf("salary payable credit = %.2f, want total_net %.2f", sawSalaryCredit, run.TotalNet)
	}
	if sawTaxCredit != run.TotalPPh21 {
		t.Errorf("tax payable credit = %.2f, want total_pph21 %.2f", sawTaxCredit, run.TotalPPh21)
	}
	if sawBPJSCredit != run.TotalBPJS {
		t.Errorf("bpjs payable credit = %.2f, want total_bpjs %.2f", sawBPJSCredit, run.TotalBPJS)
	}

	// Posting an already-POSTED run must be rejected without calling finance-service again.
	repost := postJSON(t, srv.URL+"/payroll-runs/"+run.ID+"/post", map[string]any{
		"expense_account_id": expenseAcc, "salary_payable_account_id": salaryAcc,
		"tax_payable_account_id": taxAcc, "bpjs_payable_account_id": bpjsAcc,
	})
	requireStatus(t, repost, http.StatusConflict)
	if len(*calls) != 2 {
		t.Errorf("expected no additional finance-service calls on conflict, still got %d", len(*calls))
	}
}

func TestPostPayrollRun_FinanceServiceFailureLeavesRunDraft(t *testing.T) {
	srv, calls := newServerWithFinanceStub(t, true) // stub always fails the create call
	companyID := newCompanyID(t)
	mustSeedEmployee(t, srv, companyID, 10_000_000, 1_000_000)
	run := mustProcessPayroll(t, srv, companyID, "2026-12")

	resp := postJSON(t, srv.URL+"/payroll-runs/"+run.ID+"/post", map[string]any{
		"expense_account_id": uuid.NewString(), "salary_payable_account_id": uuid.NewString(),
		"tax_payable_account_id": uuid.NewString(), "bpjs_payable_account_id": uuid.NewString(),
	})
	requireStatus(t, resp, http.StatusBadGateway)
	if len(*calls) != 1 {
		t.Fatalf("expected exactly 1 attempted finance-service call, got %d", len(*calls))
	}

	// The run must remain DRAFT locally since finance-service never confirmed the posting.
	getResp := getJSON(t, srv.URL+"/payroll-runs/"+run.ID)
	requireStatus(t, getResp, http.StatusOK)
	var reloaded payrollRunView
	getResp.decode(t, &reloaded)
	if reloaded.Status != "DRAFT" {
		t.Errorf("status = %q, want DRAFT after finance-service failure", reloaded.Status)
	}
	if reloaded.JournalID != nil {
		t.Errorf("journal_id = %v, want nil after finance-service failure", reloaded.JournalID)
	}
}
