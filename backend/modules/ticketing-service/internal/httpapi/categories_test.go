package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateCategory_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"missing company_id":    {"category_code": "CAT-001", "name": "Billing"},
		"missing category_code": {"company_id": companyID, "name": "Billing"},
		"missing name":          {"company_id": companyID, "category_code": "CAT-001"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/categories", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateCategory_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/categories", map[string]any{
		"company_id": companyID, "category_code": "CAT-100", "name": "Billing", "description": "Pertanyaan tagihan",
	})
	requireStatus(t, resp, http.StatusCreated)

	var c struct {
		CompanyID   string `json:"company_id"`
		IsActive    bool   `json:"is_active"`
		Description string `json:"description"`
	}
	resp.decode(t, &c)
	if c.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", c.CompanyID, companyID)
	}
	if !c.IsActive {
		t.Error("is_active = false, want default true")
	}
	if c.Description != "Pertanyaan tagihan" {
		t.Errorf("description = %q, want Pertanyaan tagihan", c.Description)
	}
}

func TestCreateCategory_DuplicateCodeConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	payload := map[string]any{"company_id": companyID, "category_code": "CAT-DUP", "name": "Category A"}
	requireStatus(t, postJSON(t, srv.URL+"/categories", payload), http.StatusCreated)
	conflict := postJSON(t, srv.URL+"/categories", payload)
	requireStatus(t, conflict, http.StatusConflict)
	if conflict.errorMessage() == "" {
		t.Error("expected a non-empty error message on conflict")
	}
}

func TestUpdateCategory_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/categories/"+uuid.NewString(), map[string]any{
		"name": "Updated", "is_active": true,
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateCategory_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	c := mustSeedCategory(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/categories/"+c.ID, map[string]any{
		"name": "Renamed Category", "description": "Updated desc", "is_active": false,
	}, "")
	requireStatus(t, resp, http.StatusOK)

	var updated struct {
		Name     string `json:"name"`
		IsActive bool   `json:"is_active"`
	}
	resp.decode(t, &updated)
	if updated.Name != "Renamed Category" || updated.IsActive {
		t.Errorf("unexpected update result: %+v", updated)
	}
}

func TestListCategories_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	mustSeedCategory(t, srv, companyA)
	mustSeedCategory(t, srv, companyB)

	resp := getJSON(t, srv.URL+"/categories?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 category scoped to companyA, got %+v", list)
	}
}

func TestListCategories_FilteredByBranch(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	branchA := uuid.NewString()
	branchB := uuid.NewString()

	mkCategory := func(branchID *string) {
		code := "CAT-" + uuid.NewString()[:8]
		requireStatus(t, postJSON(t, srv.URL+"/categories", map[string]any{
			"company_id": companyID, "branch_id": branchID, "category_code": code, "name": "Test " + code,
		}), http.StatusCreated)
	}
	mkCategory(&branchA)
	mkCategory(nil)
	mkCategory(&branchB)

	resp := getJSON(t, srv.URL+"/categories?company_id="+companyID+"&branch_id="+branchA)
	requireStatus(t, resp, http.StatusOK)
	var categories []struct {
		BranchID *string `json:"branch_id"`
	}
	resp.decode(t, &categories)
	if len(categories) != 2 {
		t.Fatalf("expected 2 categories (branchA + NULL), got %d: %+v", len(categories), categories)
	}
	for _, c := range categories {
		if c.BranchID != nil && *c.BranchID == branchB {
			t.Errorf("branchB category leaked into branchA-filtered results: %+v", categories)
		}
	}
}

func TestListCategories_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/categories")
	requireStatus(t, resp, http.StatusBadRequest)
}
