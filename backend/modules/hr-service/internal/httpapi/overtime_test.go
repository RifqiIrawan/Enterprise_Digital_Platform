package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type overtimeView struct {
	ID              string  `json:"id"`
	EmployeeID      string  `json:"employee_id"`
	EmployeeName    string  `json:"employee_name"`
	Hours           float64 `json:"hours"`
	IsHoliday       bool    `json:"is_holiday"`
	HourlyRate      float64 `json:"hourly_rate"`
	Amount          float64 `json:"amount"`
	Status          string  `json:"status"`
	RejectionReason string  `json:"rejection_reason"`
	PayrollRunID    *string `json:"payroll_run_id"`
	BranchID        *string `json:"branch_id"`
}

// salaryFor50kRate: 8.650.000 / 173 = tepat 50.000 per jam, sehingga nilai
// lembur di test bisa dicek sebagai angka bulat tanpa terganggu pembulatan.
const salaryFor50kRate = 8_650_000

func mustCreateOvertime(t *testing.T, srv *httptest.Server, companyID, employeeID, workDate string, hours float64, isHoliday bool) overtimeView {
	t.Helper()
	resp := postJSON(t, srv.URL+"/overtime", map[string]any{
		"company_id": companyID, "employee_id": employeeID, "work_date": workDate,
		"hours": hours, "is_holiday": isHoliday, "description": "test",
	})
	requireStatus(t, resp, http.StatusCreated)
	var o overtimeView
	resp.decode(t, &o)
	return o
}

func TestCreateOvertime_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, salaryFor50kRate, 0)

	base := func(over map[string]any) map[string]any {
		payload := map[string]any{"company_id": companyID, "employee_id": emp.ID, "work_date": "2026-08-04", "hours": 2}
		for k, v := range over {
			payload[k] = v
		}
		return payload
	}

	cases := map[string]map[string]any{
		"missing employee_id":   {"company_id": companyID, "work_date": "2026-08-04", "hours": 2},
		"missing work_date":     {"company_id": companyID, "employee_id": emp.ID, "hours": 2},
		"zero hours":            base(map[string]any{"hours": 0}),
		"hours above daily cap": base(map[string]any{"hours": 13}),
		"negative hourly_rate":  base(map[string]any{"hourly_rate": -1}),
		"bad work_date format":  base(map[string]any{"work_date": "04-08-2026"}),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			requireStatus(t, postJSON(t, srv.URL+"/overtime", payload), http.StatusBadRequest)
		})
	}
}

// TestCreateOvertime_AppliesStatutoryMultipliers pins the pay tiers from
// Kepmenaker 102/2004 art. 11 (see calculateOvertimeAmount): they are the whole
// reason overtime is a separate table instead of a free-text allowance.
func TestCreateOvertime_AppliesStatutoryMultipliers(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := []struct {
		name       string
		hours      float64
		isHoliday  bool
		wantAmount float64 // rate 50.000/jam
	}{
		{"workday first hour is 1.5x", 1, false, 75_000},
		{"workday fractional hour prorates within the tier", 0.5, false, 37_500},
		{"workday hours after the first are 2x", 2, false, 175_000},
		{"holiday first 8 hours are 2x", 3, true, 300_000},
		{"holiday 9th hour is 3x and the rest 4x", 10, true, 1_150_000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			emp := mustSeedEmployee(t, srv, companyID, salaryFor50kRate, 0)
			o := mustCreateOvertime(t, srv, companyID, emp.ID, "2026-08-04", tc.hours, tc.isHoliday)
			if o.HourlyRate != 50_000 {
				t.Fatalf("hourly_rate = %.2f, want 50000.00 derived from basic_salary/173", o.HourlyRate)
			}
			if o.Amount != tc.wantAmount {
				t.Errorf("amount = %.2f, want %.2f", o.Amount, tc.wantAmount)
			}
		})
	}
}

// TestCreateOvertime_ExplicitRateWins: an explicit hourly_rate must override
// the salary-derived default (project billing rates differ from payroll rates).
func TestCreateOvertime_ExplicitRateWins(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, salaryFor50kRate, 0)

	resp := postJSON(t, srv.URL+"/overtime", map[string]any{
		"company_id": companyID, "employee_id": emp.ID, "work_date": "2026-08-04",
		"hours": 2, "hourly_rate": 100_000,
	})
	requireStatus(t, resp, http.StatusCreated)
	var o overtimeView
	resp.decode(t, &o)
	if o.HourlyRate != 100_000 || o.Amount != 350_000 {
		t.Errorf("got rate=%.2f amount=%.2f, want 100000.00 / 350000.00 (1.5x + 2x)", o.HourlyRate, o.Amount)
	}
}

func TestCreateOvertime_DuplicateDayConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, salaryFor50kRate, 0)

	mustCreateOvertime(t, srv, companyID, emp.ID, "2026-08-04", 2, false)
	requireStatus(t, postJSON(t, srv.URL+"/overtime", map[string]any{
		"company_id": companyID, "employee_id": emp.ID, "work_date": "2026-08-04", "hours": 3,
	}), http.StatusConflict)
}

func TestOvertimeWorkflow_ApproveRejectAndEditLock(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, salaryFor50kRate, 0)
	o := mustCreateOvertime(t, srv, companyID, emp.ID, "2026-08-04", 2, false)

	// Editing while DRAFT recalculates the amount from the new hours.
	edited := doRequest(t, http.MethodPut, srv.URL+"/overtime/"+o.ID, map[string]any{"hours": 3}, "")
	requireStatus(t, edited, http.StatusOK)
	var afterEdit overtimeView
	edited.decode(t, &afterEdit)
	if afterEdit.Amount != 275_000 {
		t.Errorf("amount after edit = %.2f, want 275000.00 (1x1.5 + 2x2 @ 50k)", afterEdit.Amount)
	}

	requireStatus(t, postJSON(t, srv.URL+"/overtime/"+o.ID+"/approve", nil), http.StatusOK)
	// Approved rows are locked against edits and against a second approval.
	requireStatus(t, doRequest(t, http.MethodPut, srv.URL+"/overtime/"+o.ID, map[string]any{"hours": 4}, ""), http.StatusConflict)
	requireStatus(t, postJSON(t, srv.URL+"/overtime/"+o.ID+"/approve", nil), http.StatusConflict)

	// Rejecting an approved row is still allowed while no payroll used it,
	// but the reason is mandatory.
	requireStatus(t, postJSON(t, srv.URL+"/overtime/"+o.ID+"/reject", nil), http.StatusBadRequest)
	rejected := postJSON(t, srv.URL+"/overtime/"+o.ID+"/reject", map[string]any{"rejection_reason": "tidak diperintahkan"})
	requireStatus(t, rejected, http.StatusOK)
	var afterReject overtimeView
	rejected.decode(t, &afterReject)
	if afterReject.Status != "REJECTED" || afterReject.RejectionReason != "tidak diperintahkan" {
		t.Errorf("got status=%q reason=%q, want REJECTED / \"tidak diperintahkan\"", afterReject.Status, afterReject.RejectionReason)
	}
}

func TestOvertime_NotFound(t *testing.T) {
	srv := newServer(t)
	id := uuid.NewString()
	requireStatus(t, doRequest(t, http.MethodPut, srv.URL+"/overtime/"+id, map[string]any{"hours": 2}, ""), http.StatusNotFound)
	requireStatus(t, postJSON(t, srv.URL+"/overtime/"+id+"/approve", nil), http.StatusNotFound)
}

func TestListOvertime_FiltersByPeriodAndStatus(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, salaryFor50kRate, 0)

	july := mustCreateOvertime(t, srv, companyID, emp.ID, "2026-07-15", 2, false)
	mustCreateOvertime(t, srv, companyID, emp.ID, "2026-08-04", 2, false)
	requireStatus(t, postJSON(t, srv.URL+"/overtime/"+july.ID+"/approve", nil), http.StatusOK)

	resp := getJSON(t, srv.URL+"/overtime?company_id="+companyID+"&period=2026-07")
	requireStatus(t, resp, http.StatusOK)
	var byPeriod []overtimeView
	resp.decode(t, &byPeriod)
	if len(byPeriod) != 1 || byPeriod[0].ID != july.ID {
		t.Fatalf("period filter returned %+v, want only the July row", byPeriod)
	}

	resp = getJSON(t, srv.URL+"/overtime?company_id="+companyID+"&status=DRAFT")
	requireStatus(t, resp, http.StatusOK)
	var byStatus []overtimeView
	resp.decode(t, &byStatus)
	if len(byStatus) != 1 || byStatus[0].Status != "DRAFT" {
		t.Fatalf("status filter returned %+v, want only the DRAFT row", byStatus)
	}
}

// TestListOvertime_FilteredByBranch confirms branch_id filtering is
// NULL-inclusive, same rule as every other list endpoint in this service.
func TestListOvertime_FilteredByBranch(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	branchA := uuid.NewString()
	branchB := uuid.NewString()

	mkOvertime := func(branchID *string) {
		emp := mustSeedEmployee(t, srv, companyID, salaryFor50kRate, 0)
		requireStatus(t, postJSON(t, srv.URL+"/overtime", map[string]any{
			"company_id": companyID, "branch_id": branchID, "employee_id": emp.ID,
			"work_date": "2026-08-04", "hours": 2,
		}), http.StatusCreated)
	}
	mkOvertime(&branchA)
	mkOvertime(nil)
	mkOvertime(&branchB)

	resp := getJSON(t, srv.URL+"/overtime?company_id="+companyID+"&branch_id="+branchA)
	requireStatus(t, resp, http.StatusOK)
	var got []overtimeView
	resp.decode(t, &got)
	if len(got) != 2 {
		t.Fatalf("expected 2 rows (branchA + NULL), got %d: %+v", len(got), got)
	}
	for _, o := range got {
		if o.BranchID != nil && *o.BranchID == branchB {
			t.Errorf("branchB row leaked into branchA-filtered results: %+v", got)
		}
	}
}
