package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateAttendance_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)

	cases := map[string]map[string]any{
		"missing employee_id": {"company_id": companyID, "log_date": "2026-07-01"},
		"missing log_date":    {"company_id": companyID, "employee_id": emp.ID},
		"invalid status":      {"company_id": companyID, "employee_id": emp.ID, "log_date": "2026-07-01", "status": "ON_VACATION"},
		"bad log_date format": {"company_id": companyID, "employee_id": emp.ID, "log_date": "01-07-2026"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/attendance", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateAttendance_BadCheckInFormat(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)

	resp := postJSON(t, srv.URL+"/attendance", map[string]any{
		"company_id": companyID, "employee_id": emp.ID, "log_date": "2026-07-01", "check_in": "not-a-timestamp",
	})
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestCreateAttendance_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)

	resp := postJSON(t, srv.URL+"/attendance", map[string]any{
		"company_id": companyID, "employee_id": emp.ID, "log_date": "2026-07-01",
		"check_in": "2026-07-01T08:00:00Z", "check_out": "2026-07-01T17:00:00Z",
	})
	requireStatus(t, resp, http.StatusCreated)

	var a struct {
		Status string `json:"status"`
		Source string `json:"source"`
	}
	resp.decode(t, &a)
	if a.Status != "PRESENT" {
		t.Errorf("status = %q, want default PRESENT", a.Status)
	}
	if a.Source != "MANUAL" {
		t.Errorf("source = %q, want default MANUAL", a.Source)
	}
}

func TestCreateAttendance_DuplicateDayConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)

	payload := map[string]any{"company_id": companyID, "employee_id": emp.ID, "log_date": "2026-07-02", "status": "PRESENT"}
	requireStatus(t, postJSON(t, srv.URL+"/attendance", payload), http.StatusCreated)
	requireStatus(t, postJSON(t, srv.URL+"/attendance", payload), http.StatusConflict)
}

func TestUpdateAttendance_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/attendance/"+uuid.NewString(), map[string]any{
		"status": "LATE",
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateAttendance_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)

	createResp := postJSON(t, srv.URL+"/attendance", map[string]any{
		"company_id": companyID, "employee_id": emp.ID, "log_date": "2026-07-03", "status": "PRESENT",
	})
	requireStatus(t, createResp, http.StatusCreated)
	var created struct {
		ID string `json:"id"`
	}
	createResp.decode(t, &created)

	updateResp := doRequest(t, http.MethodPut, srv.URL+"/attendance/"+created.ID, map[string]any{
		"status": "LATE",
	}, "")
	requireStatus(t, updateResp, http.StatusOK)
	var updated struct {
		Status string `json:"status"`
	}
	updateResp.decode(t, &updated)
	if updated.Status != "LATE" {
		t.Errorf("status = %q, want LATE", updated.Status)
	}
}

func TestListAttendance_FiltersByEmployeeAndPeriod(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	empA := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)
	empB := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)

	requireStatus(t, postJSON(t, srv.URL+"/attendance", map[string]any{
		"company_id": companyID, "employee_id": empA.ID, "log_date": "2026-06-15", "status": "PRESENT",
	}), http.StatusCreated)
	requireStatus(t, postJSON(t, srv.URL+"/attendance", map[string]any{
		"company_id": companyID, "employee_id": empA.ID, "log_date": "2026-07-15", "status": "PRESENT",
	}), http.StatusCreated)
	requireStatus(t, postJSON(t, srv.URL+"/attendance", map[string]any{
		"company_id": companyID, "employee_id": empB.ID, "log_date": "2026-07-15", "status": "PRESENT",
	}), http.StatusCreated)

	resp := getJSON(t, srv.URL+"/attendance?company_id="+companyID+"&employee_id="+empA.ID+"&period=2026-07")
	requireStatus(t, resp, http.StatusOK)
	var logs []struct {
		EmployeeID string `json:"employee_id"`
	}
	resp.decode(t, &logs)
	if len(logs) != 1 || logs[0].EmployeeID != empA.ID {
		t.Fatalf("expected exactly 1 log for empA in 2026-07, got %+v", logs)
	}
}

// TestListAttendance_FilteredByBranch confirms branch_id filtering is
// NULL-inclusive: a branch filter must still surface unassigned (NULL
// branch_id) rows alongside that branch's own rows.
func TestListAttendance_FilteredByBranch(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	branchA := uuid.NewString()
	branchB := uuid.NewString()

	mkLog := func(branchID *string, logDate string) {
		emp := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)
		requireStatus(t, postJSON(t, srv.URL+"/attendance", map[string]any{
			"company_id": companyID, "branch_id": branchID, "employee_id": emp.ID, "log_date": logDate, "status": "PRESENT",
		}), http.StatusCreated)
	}
	mkLog(&branchA, "2026-07-10")
	mkLog(nil, "2026-07-10")
	mkLog(&branchB, "2026-07-10")

	resp := getJSON(t, srv.URL+"/attendance?company_id="+companyID+"&branch_id="+branchA)
	requireStatus(t, resp, http.StatusOK)
	var logs []struct {
		BranchID *string `json:"branch_id"`
	}
	resp.decode(t, &logs)
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs (branchA + NULL), got %d: %+v", len(logs), logs)
	}
	for _, l := range logs {
		if l.BranchID != nil && *l.BranchID == branchB {
			t.Errorf("branchB log leaked into branchA-filtered results: %+v", logs)
		}
	}
}
