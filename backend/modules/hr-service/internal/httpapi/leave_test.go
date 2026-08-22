package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type leaveView struct {
	ID              string  `json:"id"`
	EmployeeID      string  `json:"employee_id"`
	EmployeeName    string  `json:"employee_name"`
	LeaveType       string  `json:"leave_type"`
	TotalDays       int     `json:"total_days"`
	Status          string  `json:"status"`
	RejectionReason string  `json:"rejection_reason"`
	BranchID        *string `json:"branch_id"`
	SubmittedAt     *string `json:"submitted_at"`
	DecidedAt       *string `json:"decided_at"`
}

// mustCreateLeave creates a DRAFT leave request through the real endpoint.
func mustCreateLeave(t *testing.T, srv *httptest.Server, companyID, employeeID, leaveType, start, end string) leaveView {
	t.Helper()
	resp := postJSON(t, srv.URL+"/leave-requests", map[string]any{
		"company_id": companyID, "employee_id": employeeID, "leave_type": leaveType,
		"start_date": start, "end_date": end, "reason": "test",
	})
	requireStatus(t, resp, http.StatusCreated)
	var l leaveView
	resp.decode(t, &l)
	return l
}

// mustApproveLeave walks a request all the way to APPROVED (DRAFT -> SUBMITTED
// -> APPROVED), the only status that payroll actually reacts to.
func mustApproveLeave(t *testing.T, srv *httptest.Server, id string) {
	t.Helper()
	requireStatus(t, postJSON(t, srv.URL+"/leave-requests/"+id+"/submit", nil), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/leave-requests/"+id+"/approve", nil), http.StatusOK)
}

func TestCreateLeaveRequest_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)

	base := func(over map[string]any) map[string]any {
		payload := map[string]any{
			"company_id": companyID, "employee_id": emp.ID, "leave_type": "ANNUAL",
			"start_date": "2026-08-03", "end_date": "2026-08-07",
		}
		for k, v := range over {
			payload[k] = v
		}
		return payload
	}

	cases := map[string]map[string]any{
		"missing employee_id": {"company_id": companyID, "leave_type": "ANNUAL", "start_date": "2026-08-03", "end_date": "2026-08-07"},
		"missing dates":       {"company_id": companyID, "employee_id": emp.ID, "leave_type": "ANNUAL"},
		"invalid leave_type":  base(map[string]any{"leave_type": "SABBATICAL"}),
		"bad date format":     base(map[string]any{"start_date": "03-08-2026"}),
		"end before start":    base(map[string]any{"start_date": "2026-08-07", "end_date": "2026-08-03"}),
		"range too long":      base(map[string]any{"start_date": "2026-01-01", "end_date": "2027-06-01"}),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			requireStatus(t, postJSON(t, srv.URL+"/leave-requests", payload), http.StatusBadRequest)
		})
	}
}

func TestCreateLeaveRequest_UnknownOrForeignEmployee(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	otherCompany := newCompanyID(t)
	foreign := mustSeedEmployee(t, srv, otherCompany, 5_000_000, 0)

	payload := func(employeeID string) map[string]any {
		return map[string]any{
			"company_id": companyID, "employee_id": employeeID, "leave_type": "ANNUAL",
			"start_date": "2026-08-03", "end_date": "2026-08-07",
		}
	}
	requireStatus(t, postJSON(t, srv.URL+"/leave-requests", payload(uuid.NewString())), http.StatusBadRequest)
	requireStatus(t, postJSON(t, srv.URL+"/leave-requests", payload(foreign.ID)), http.StatusBadRequest)
}

// TestCreateLeaveRequest_CountsWeekdaysOnly pins total_days to working days:
// a Mon-Fri range is 5, and stretching it across the weekend to the next
// Tuesday must add 2 (Mon+Tue), not 4 calendar days.
func TestCreateLeaveRequest_CountsWeekdaysOnly(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	empA := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)
	weekOnly := mustCreateLeave(t, srv, companyID, empA.ID, "ANNUAL", "2026-08-03", "2026-08-07")
	if weekOnly.TotalDays != 5 {
		t.Errorf("total_days = %d for Mon-Fri, want 5", weekOnly.TotalDays)
	}
	if weekOnly.Status != "DRAFT" {
		t.Errorf("status = %q, want DRAFT", weekOnly.Status)
	}
	if weekOnly.EmployeeName == "" {
		t.Error("employee_name snapshot is empty, want the employee's full name")
	}

	empB := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)
	acrossWeekend := mustCreateLeave(t, srv, companyID, empB.ID, "ANNUAL", "2026-08-03", "2026-08-11")
	if acrossWeekend.TotalDays != 7 {
		t.Errorf("total_days = %d for Mon-next Tue, want 7 (weekend excluded)", acrossWeekend.TotalDays)
	}
}

func TestCreateLeaveRequest_OverlapConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)
	first := mustCreateLeave(t, srv, companyID, emp.ID, "ANNUAL", "2026-08-03", "2026-08-07")

	overlapping := map[string]any{
		"company_id": companyID, "employee_id": emp.ID, "leave_type": "SICK",
		"start_date": "2026-08-06", "end_date": "2026-08-10",
	}
	requireStatus(t, postJSON(t, srv.URL+"/leave-requests", overlapping), http.StatusConflict)

	// A rejected request must not keep blocking the same dates: reject the
	// first one, then the overlapping range becomes available again.
	requireStatus(t, postJSON(t, srv.URL+"/leave-requests/"+first.ID+"/submit", nil), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/leave-requests/"+first.ID+"/reject", map[string]any{"rejection_reason": "beban kerja"}), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/leave-requests", overlapping), http.StatusCreated)
}

func TestLeaveWorkflow_StatusTransitions(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)
	l := mustCreateLeave(t, srv, companyID, emp.ID, "ANNUAL", "2026-08-03", "2026-08-07")

	// Approving straight from DRAFT skips the submission step.
	requireStatus(t, postJSON(t, srv.URL+"/leave-requests/"+l.ID+"/approve", nil), http.StatusConflict)

	submitted := postJSON(t, srv.URL+"/leave-requests/"+l.ID+"/submit", nil)
	requireStatus(t, submitted, http.StatusOK)
	var afterSubmit leaveView
	submitted.decode(t, &afterSubmit)
	if afterSubmit.Status != "SUBMITTED" || afterSubmit.SubmittedAt == nil {
		t.Fatalf("after submit: status = %q, submitted_at = %v; want SUBMITTED with a timestamp", afterSubmit.Status, afterSubmit.SubmittedAt)
	}

	// Editing is locked once submitted.
	requireStatus(t, doRequest(t, http.MethodPut, srv.URL+"/leave-requests/"+l.ID, map[string]any{
		"leave_type": "SICK", "start_date": "2026-08-03", "end_date": "2026-08-04",
	}, ""), http.StatusConflict)

	approved := postJSON(t, srv.URL+"/leave-requests/"+l.ID+"/approve", nil)
	requireStatus(t, approved, http.StatusOK)
	var afterApprove leaveView
	approved.decode(t, &afterApprove)
	if afterApprove.Status != "APPROVED" || afterApprove.DecidedAt == nil {
		t.Fatalf("after approve: status = %q, decided_at = %v; want APPROVED with a timestamp", afterApprove.Status, afterApprove.DecidedAt)
	}

	// Approved leave can still be cancelled while no payroll covers it.
	cancelled := postJSON(t, srv.URL+"/leave-requests/"+l.ID+"/cancel", nil)
	requireStatus(t, cancelled, http.StatusOK)
	var afterCancel leaveView
	cancelled.decode(t, &afterCancel)
	if afterCancel.Status != "CANCELLED" {
		t.Errorf("status = %q, want CANCELLED", afterCancel.Status)
	}
}

func TestRejectLeaveRequest_RequiresReason(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)
	l := mustCreateLeave(t, srv, companyID, emp.ID, "ANNUAL", "2026-08-03", "2026-08-07")
	requireStatus(t, postJSON(t, srv.URL+"/leave-requests/"+l.ID+"/submit", nil), http.StatusOK)

	requireStatus(t, postJSON(t, srv.URL+"/leave-requests/"+l.ID+"/reject", map[string]any{"rejection_reason": "   "}), http.StatusBadRequest)

	resp := postJSON(t, srv.URL+"/leave-requests/"+l.ID+"/reject", map[string]any{"rejection_reason": "kuota habis"})
	requireStatus(t, resp, http.StatusOK)
	var rejected leaveView
	resp.decode(t, &rejected)
	if rejected.Status != "REJECTED" || rejected.RejectionReason != "kuota habis" {
		t.Errorf("got status=%q reason=%q, want REJECTED / \"kuota habis\"", rejected.Status, rejected.RejectionReason)
	}
}

func TestUpdateLeaveRequest_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/leave-requests/"+uuid.NewString(), map[string]any{
		"leave_type": "ANNUAL", "start_date": "2026-08-03", "end_date": "2026-08-07",
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

// TestCancelLeaveRequest_BlockedAfterPayrollPosted is the cross-module guard:
// once the payroll covering those dates is in the GL, the approved leave that
// shaped it can no longer be withdrawn.
func TestCancelLeaveRequest_BlockedAfterPayrollPosted(t *testing.T) {
	srv, _ := newServerWithFinanceStub(t, false)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 10_000_000, 1_000_000)
	l := mustCreateLeave(t, srv, companyID, emp.ID, "UNPAID", "2026-08-03", "2026-08-07")
	mustApproveLeave(t, srv, l.ID)

	run := mustProcessPayroll(t, srv, companyID, "2026-08")
	requireStatus(t, postJSON(t, srv.URL+"/payroll-runs/"+run.ID+"/post", map[string]any{
		"expense_account_id": uuid.NewString(), "salary_payable_account_id": uuid.NewString(),
		"tax_payable_account_id": uuid.NewString(), "bpjs_payable_account_id": uuid.NewString(),
	}), http.StatusOK)

	requireStatus(t, postJSON(t, srv.URL+"/leave-requests/"+l.ID+"/cancel", nil), http.StatusConflict)
}

// TestListLeaveRequests_FilterByPeriodOverlaps: a request spanning the month
// boundary belongs to BOTH periods, because both are affected by it.
func TestListLeaveRequests_FilterByPeriodOverlaps(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)
	mustCreateLeave(t, srv, companyID, emp.ID, "ANNUAL", "2026-07-30", "2026-08-04")

	for _, period := range []string{"2026-07", "2026-08"} {
		resp := getJSON(t, srv.URL+"/leave-requests?company_id="+companyID+"&period="+period)
		requireStatus(t, resp, http.StatusOK)
		var got []leaveView
		resp.decode(t, &got)
		if len(got) != 1 {
			t.Errorf("period %s: got %d requests, want 1 (the range crosses the month boundary)", period, len(got))
		}
	}

	resp := getJSON(t, srv.URL+"/leave-requests?company_id="+companyID+"&period=2026-09")
	requireStatus(t, resp, http.StatusOK)
	var september []leaveView
	resp.decode(t, &september)
	if len(september) != 0 {
		t.Errorf("period 2026-09: got %d requests, want 0", len(september))
	}
}

// TestListLeaveRequests_FilteredByBranch confirms branch_id filtering is
// NULL-inclusive, same rule as every other list endpoint in this service.
func TestListLeaveRequests_FilteredByBranch(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	branchA := uuid.NewString()
	branchB := uuid.NewString()

	mkLeave := func(branchID *string) {
		emp := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)
		requireStatus(t, postJSON(t, srv.URL+"/leave-requests", map[string]any{
			"company_id": companyID, "branch_id": branchID, "employee_id": emp.ID,
			"leave_type": "ANNUAL", "start_date": "2026-08-03", "end_date": "2026-08-07",
		}), http.StatusCreated)
	}
	mkLeave(&branchA)
	mkLeave(nil)
	mkLeave(&branchB)

	resp := getJSON(t, srv.URL+"/leave-requests?company_id="+companyID+"&branch_id="+branchA)
	requireStatus(t, resp, http.StatusOK)
	var got []leaveView
	resp.decode(t, &got)
	if len(got) != 2 {
		t.Fatalf("expected 2 requests (branchA + NULL), got %d: %+v", len(got), got)
	}
	for _, l := range got {
		if l.BranchID != nil && *l.BranchID == branchB {
			t.Errorf("branchB request leaked into branchA-filtered results: %+v", got)
		}
	}
}
