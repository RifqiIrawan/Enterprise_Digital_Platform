package httpapi_test

import (
	"maps"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateEmployee_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	base := func() map[string]any {
		return map[string]any{
			"company_id": companyID, "employee_code": "EMP-001", "first_name": "Budi",
			"email": "budi@example.test", "hire_date": "2024-01-15",
		}
	}

	cases := map[string]map[string]any{
		"missing company_id":      withOverride(base(), "company_id", ""),
		"missing employee_code":   withOverride(base(), "employee_code", ""),
		"missing first_name":      withOverride(base(), "first_name", ""),
		"missing email":           withOverride(base(), "email", ""),
		"missing hire_date":       withOverride(base(), "hire_date", ""),
		"invalid employment_type": withOverride(base(), "employment_type", "FREELANCE"),
		"invalid ptkp_status":     withOverride(base(), "ptkp_status", "X/9"),
		"bad hire_date format":    withOverride(base(), "hire_date", "15-01-2024"),
		"negative basic_salary":   withOverride(base(), "basic_salary", -100),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/employees", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func withOverride(m map[string]any, key string, value any) map[string]any {
	out := make(map[string]any, len(m))
	maps.Copy(out, m)
	out[key] = value
	return out
}

func TestCreateEmployee_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/employees", map[string]any{
		"company_id": companyID, "employee_code": "EMP-100", "first_name": "Siti", "last_name": "Aminah",
		"email": "siti@example.test", "hire_date": "2023-06-01",
		"basic_salary": 8_000_000, "monthly_allowance": 500_000,
	})
	requireStatus(t, resp, http.StatusCreated)

	var e struct {
		ID             string `json:"id"`
		CompanyID      string `json:"company_id"`
		EmploymentType string `json:"employment_type"`
		Status         string `json:"status"`
		PTKPStatus     string `json:"ptkp_status"`
		IsActive       bool   `json:"is_active"`
	}
	resp.decode(t, &e)

	if e.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", e.CompanyID, companyID)
	}
	if e.EmploymentType != "PERMANENT" {
		t.Errorf("employment_type = %q, want default PERMANENT", e.EmploymentType)
	}
	if e.Status != "ACTIVE" {
		t.Errorf("status = %q, want default ACTIVE", e.Status)
	}
	if e.PTKPStatus != "TK/0" {
		t.Errorf("ptkp_status = %q, want default TK/0", e.PTKPStatus)
	}
	if !e.IsActive {
		t.Error("expected is_active to default true")
	}
}

func TestCreateEmployee_DuplicateCodeConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	payload := map[string]any{
		"company_id": companyID, "employee_code": "EMP-DUP", "first_name": "Dupe",
		"email": "dupe@example.test", "hire_date": "2024-01-01",
	}
	requireStatus(t, postJSON(t, srv.URL+"/employees", payload), http.StatusCreated)

	dup := withOverride(payload, "email", "dupe2@example.test") // same code, different email
	requireStatus(t, postJSON(t, srv.URL+"/employees", dup), http.StatusConflict)
}

func TestCreateEmployee_DuplicateEmailConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	payload := map[string]any{
		"company_id": companyID, "employee_code": "EMP-A", "first_name": "A",
		"email": "same@example.test", "hire_date": "2024-01-01",
	}
	requireStatus(t, postJSON(t, srv.URL+"/employees", payload), http.StatusCreated)

	dup := withOverride(payload, "employee_code", "EMP-B") // different code, same email
	requireStatus(t, postJSON(t, srv.URL+"/employees", dup), http.StatusConflict)
}

func TestUpdateEmployee_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/employees/"+uuid.NewString(), map[string]any{
		"first_name": "X", "employment_type": "PERMANENT", "status": "ACTIVE", "ptkp_status": "TK/0",
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateEmployee_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 5_000_000, 0)

	resp := doRequest(t, http.MethodPut, srv.URL+"/employees/"+emp.ID, map[string]any{
		"first_name": "Renamed", "employment_type": "CONTRACT", "status": "ON_LEAVE",
		"ptkp_status": "K/1", "basic_salary": 6_000_000, "monthly_allowance": 200_000, "is_active": true,
	}, "")
	requireStatus(t, resp, http.StatusOK)

	var updated struct {
		FirstName      string `json:"first_name"`
		EmploymentType string `json:"employment_type"`
		Status         string `json:"status"`
	}
	resp.decode(t, &updated)
	if updated.FirstName != "Renamed" || updated.EmploymentType != "CONTRACT" || updated.Status != "ON_LEAVE" {
		t.Errorf("unexpected update result: %+v", updated)
	}
}

func TestListEmployees_ScopedByCompanyAndStatus(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	mustSeedEmployee(t, srv, companyA, 5_000_000, 0)
	empB := mustSeedEmployee(t, srv, companyB, 5_000_000, 0)

	resp := getJSON(t, srv.URL+"/employees?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 employee scoped to companyA, got %+v", list)
	}

	// Terminate empB, then filter companyB by status=ACTIVE should exclude it.
	doRequest(t, http.MethodPut, srv.URL+"/employees/"+empB.ID, map[string]any{
		"first_name": "Test", "employment_type": "PERMANENT", "status": "TERMINATED", "ptkp_status": "TK/0",
	}, "")
	activeResp := getJSON(t, srv.URL+"/employees?company_id="+companyB+"&status=ACTIVE")
	requireStatus(t, activeResp, http.StatusOK)
	var activeList []struct{}
	activeResp.decode(t, &activeList)
	if len(activeList) != 0 {
		t.Errorf("expected 0 ACTIVE employees in companyB after termination, got %d", len(activeList))
	}
}
