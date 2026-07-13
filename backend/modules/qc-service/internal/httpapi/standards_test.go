package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateStandard_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"missing standard_code": {"company_id": companyID, "name": "Standar Uji", "product_id": uuid.NewString()},
		"missing name":          {"company_id": companyID, "standard_code": "QS-001", "product_id": uuid.NewString()},
		"missing product_id":    {"company_id": companyID, "standard_code": "QS-001", "name": "Standar Uji"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/standards", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateStandard_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/standards", map[string]any{
		"company_id": companyID, "standard_code": "QS-100", "name": "Standar Barang Jadi",
		"product_id": uuid.NewString(), "criteria": "Tidak cacat, warna sesuai",
	})
	requireStatus(t, resp, http.StatusCreated)

	var s struct {
		CompanyID string `json:"company_id"`
		IsActive  bool   `json:"is_active"`
	}
	resp.decode(t, &s)
	if s.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", s.CompanyID, companyID)
	}
	if !s.IsActive {
		t.Error("expected is_active to default true")
	}
}

func TestCreateStandard_DuplicateCodeConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	payload := map[string]any{
		"company_id": companyID, "standard_code": "QS-DUP", "name": "Standar A", "product_id": uuid.NewString(),
	}
	requireStatus(t, postJSON(t, srv.URL+"/standards", payload), http.StatusCreated)
	conflict := postJSON(t, srv.URL+"/standards", payload)
	requireStatus(t, conflict, http.StatusConflict)
	if conflict.errorMessage() == "" {
		t.Error("expected a non-empty error message on conflict")
	}
}

func TestUpdateStandard_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/standards/"+uuid.NewString(), map[string]any{
		"name": "Updated", "is_active": true,
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateStandard_BlankNameRejected(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	s := mustSeedStandard(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/standards/"+s.ID, map[string]any{
		"name": "   ", "is_active": true,
	}, "")
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateStandard_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	s := mustSeedStandard(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/standards/"+s.ID, map[string]any{
		"name": "Renamed Standard", "criteria": "Kriteria baru", "is_active": false,
	}, "")
	requireStatus(t, resp, http.StatusOK)

	var updated struct {
		Name     string `json:"name"`
		Criteria string `json:"criteria"`
		IsActive bool   `json:"is_active"`
	}
	resp.decode(t, &updated)
	if updated.Name != "Renamed Standard" || updated.Criteria != "Kriteria baru" {
		t.Errorf("unexpected update result: %+v", updated)
	}
	if updated.IsActive {
		t.Error("expected is_active to be false after update")
	}
}

func TestListStandards_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	mustSeedStandard(t, srv, companyA)
	mustSeedStandard(t, srv, companyB)

	resp := getJSON(t, srv.URL+"/standards?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 standard scoped to companyA, got %+v", list)
	}
}

// TestListStandards_FilteredByBranch confirms branch_id filtering is
// NULL-inclusive: a branch filter must still surface unassigned (NULL
// branch_id) rows alongside that branch's own rows.
func TestListStandards_FilteredByBranch(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	branchA := uuid.NewString()
	branchB := uuid.NewString()

	mkStandard := func(branchID *string) {
		code := "QS-" + uuid.NewString()[:8]
		requireStatus(t, postJSON(t, srv.URL+"/standards", map[string]any{
			"company_id": companyID, "branch_id": branchID, "standard_code": code, "name": "Test Standard " + code,
			"product_id": uuid.NewString(),
		}), http.StatusCreated)
	}
	mkStandard(&branchA)
	mkStandard(nil)
	mkStandard(&branchB)

	resp := getJSON(t, srv.URL+"/standards?company_id="+companyID+"&branch_id="+branchA)
	requireStatus(t, resp, http.StatusOK)
	var standards []struct {
		BranchID *string `json:"branch_id"`
	}
	resp.decode(t, &standards)
	if len(standards) != 2 {
		t.Fatalf("expected 2 standards (branchA + NULL), got %d: %+v", len(standards), standards)
	}
	for _, s := range standards {
		if s.BranchID != nil && *s.BranchID == branchB {
			t.Errorf("branchB standard leaked into branchA-filtered results: %+v", standards)
		}
	}
}

func TestListStandards_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/standards")
	requireStatus(t, resp, http.StatusBadRequest)
}
