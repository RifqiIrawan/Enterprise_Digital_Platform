package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateAccount_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"missing company_id":   {"account_code": "ACC-001", "name": "PT Contoh"},
		"missing account_code": {"company_id": companyID, "name": "PT Contoh"},
		"missing name":         {"company_id": companyID, "account_code": "ACC-001"},
		"invalid account_type": {"company_id": companyID, "account_code": "ACC-001", "name": "PT Contoh", "account_type": "FRENEMY"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/accounts", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateAccount_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/accounts", map[string]any{
		"company_id": companyID, "account_code": "ACC-100", "name": "PT Contoh Sejahtera", "industry": "Manufaktur",
	})
	requireStatus(t, resp, http.StatusCreated)

	var a struct {
		CompanyID   string `json:"company_id"`
		AccountType string `json:"account_type"`
		Industry    string `json:"industry"`
	}
	resp.decode(t, &a)
	if a.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", a.CompanyID, companyID)
	}
	if a.AccountType != "PROSPECT" {
		t.Errorf("account_type = %q, want default PROSPECT", a.AccountType)
	}
	if a.Industry != "Manufaktur" {
		t.Errorf("industry = %q, want Manufaktur", a.Industry)
	}
}

func TestCreateAccount_DuplicateCodeConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	payload := map[string]any{"company_id": companyID, "account_code": "ACC-DUP", "name": "Account A"}
	requireStatus(t, postJSON(t, srv.URL+"/accounts", payload), http.StatusCreated)
	conflict := postJSON(t, srv.URL+"/accounts", payload)
	requireStatus(t, conflict, http.StatusConflict)
	if conflict.errorMessage() == "" {
		t.Error("expected a non-empty error message on conflict")
	}
}

func TestUpdateAccount_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/accounts/"+uuid.NewString(), map[string]any{
		"name": "Updated", "account_type": "CUSTOMER",
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateAccount_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAccount(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/accounts/"+a.ID, map[string]any{
		"name": "Renamed Account", "account_type": "CUSTOMER", "industry": "Retail",
	}, "")
	requireStatus(t, resp, http.StatusOK)

	var updated struct {
		Name        string `json:"name"`
		AccountType string `json:"account_type"`
		Industry    string `json:"industry"`
	}
	resp.decode(t, &updated)
	if updated.Name != "Renamed Account" || updated.AccountType != "CUSTOMER" || updated.Industry != "Retail" {
		t.Errorf("unexpected update result: %+v", updated)
	}
}

func TestListAccounts_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	mustSeedAccount(t, srv, companyA)
	mustSeedAccount(t, srv, companyB)

	resp := getJSON(t, srv.URL+"/accounts?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 account scoped to companyA, got %+v", list)
	}
}

func TestListAccounts_FilteredByBranch(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	branchA := uuid.NewString()
	branchB := uuid.NewString()

	mkAccount := func(branchID *string) {
		code := "ACC-" + uuid.NewString()[:8]
		requireStatus(t, postJSON(t, srv.URL+"/accounts", map[string]any{
			"company_id": companyID, "branch_id": branchID, "account_code": code, "name": "Test Account " + code,
		}), http.StatusCreated)
	}
	mkAccount(&branchA)
	mkAccount(nil)
	mkAccount(&branchB)

	resp := getJSON(t, srv.URL+"/accounts?company_id="+companyID+"&branch_id="+branchA)
	requireStatus(t, resp, http.StatusOK)
	var accounts []struct {
		BranchID *string `json:"branch_id"`
	}
	resp.decode(t, &accounts)
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts (branchA + NULL), got %d: %+v", len(accounts), accounts)
	}
	for _, a := range accounts {
		if a.BranchID != nil && *a.BranchID == branchB {
			t.Errorf("branchB account leaked into branchA-filtered results: %+v", accounts)
		}
	}
}

func TestListAccounts_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/accounts")
	requireStatus(t, resp, http.StatusBadRequest)
}
