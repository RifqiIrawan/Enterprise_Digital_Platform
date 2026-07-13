package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateSupplier_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"missing company_id":    {"supplier_code": "SUP-001", "name": "PT Uji"},
		"missing supplier_code": {"company_id": companyID, "name": "PT Uji"},
		"missing name":          {"company_id": companyID, "supplier_code": "SUP-001"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/suppliers", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateSupplier_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/suppliers", map[string]any{
		"company_id": companyID, "supplier_code": "SUP-100", "name": "PT Pemasok Uji",
		"email": "supplier@example.test", "tax_id": "01.234.567.8-901.000",
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

func TestCreateSupplier_DuplicateCodeConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	payload := map[string]any{"company_id": companyID, "supplier_code": "SUP-DUP", "name": "PT A"}
	requireStatus(t, postJSON(t, srv.URL+"/suppliers", payload), http.StatusCreated)
	conflict := postJSON(t, srv.URL+"/suppliers", payload)
	requireStatus(t, conflict, http.StatusConflict)
	if conflict.errorMessage() == "" {
		t.Error("expected a non-empty error message on conflict")
	}
}

func TestUpdateSupplier_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/suppliers/"+uuid.NewString(), map[string]any{
		"name": "Updated", "is_active": true,
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateSupplier_BlankNameRejected(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	s := mustSeedSupplier(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/suppliers/"+s.ID, map[string]any{
		"name": "   ", "is_active": true,
	}, "")
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateSupplier_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	s := mustSeedSupplier(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/suppliers/"+s.ID, map[string]any{
		"name": "Renamed Supplier", "email": "new@example.test", "is_active": false,
	}, "")
	requireStatus(t, resp, http.StatusOK)

	var updated struct {
		Name     string `json:"name"`
		IsActive bool   `json:"is_active"`
	}
	resp.decode(t, &updated)
	if updated.Name != "Renamed Supplier" {
		t.Errorf("name = %q, want %q", updated.Name, "Renamed Supplier")
	}
	if updated.IsActive {
		t.Error("expected is_active to be false after update")
	}
}

func TestListSuppliers_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	mustSeedSupplier(t, srv, companyA)
	mustSeedSupplier(t, srv, companyB)

	resp := getJSON(t, srv.URL+"/suppliers?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 supplier scoped to companyA, got %+v", list)
	}
}

func TestListSuppliers_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/suppliers")
	requireStatus(t, resp, http.StatusBadRequest)
}

// TestListSuppliers_FilteredByBranch confirms branch_id filtering is
// NULL-inclusive: a branch filter must still surface unassigned (NULL
// branch_id) rows alongside that branch's own rows.
func TestListSuppliers_FilteredByBranch(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	branchA := uuid.NewString()
	branchB := uuid.NewString()

	mkSupplier := func(branchID *string) {
		code := "SUP-" + uuid.NewString()[:8]
		requireStatus(t, postJSON(t, srv.URL+"/suppliers", map[string]any{
			"company_id": companyID, "branch_id": branchID, "supplier_code": code, "name": "Test Supplier " + code,
		}), http.StatusCreated)
	}
	mkSupplier(&branchA)
	mkSupplier(nil)
	mkSupplier(&branchB)

	resp := getJSON(t, srv.URL+"/suppliers?company_id="+companyID+"&branch_id="+branchA)
	requireStatus(t, resp, http.StatusOK)
	var suppliers []struct {
		BranchID *string `json:"branch_id"`
	}
	resp.decode(t, &suppliers)
	if len(suppliers) != 2 {
		t.Fatalf("expected 2 suppliers (branchA + NULL), got %d: %+v", len(suppliers), suppliers)
	}
	for _, s := range suppliers {
		if s.BranchID != nil && *s.BranchID == branchB {
			t.Errorf("branchB supplier leaked into branchA-filtered results: %+v", suppliers)
		}
	}
}
