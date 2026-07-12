package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateAccount_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"missing company_id", map[string]any{"account_code": "1000", "account_name": "Kas", "account_type": "ASSET"}},
		{"missing account_code", map[string]any{"company_id": companyID, "account_name": "Kas", "account_type": "ASSET"}},
		{"missing account_name", map[string]any{"company_id": companyID, "account_code": "1000", "account_type": "ASSET"}},
		{"invalid account_type", map[string]any{"company_id": companyID, "account_code": "1000", "account_name": "Kas", "account_type": "NOT_A_TYPE"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/accounts", tc.payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateAccount_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/accounts", map[string]any{
		"company_id":   companyID,
		"account_code": "1000",
		"account_name": "Kas",
		"account_type": "asset", // lowercase should be normalized to ASSET
	})
	requireStatus(t, resp, http.StatusCreated)

	var acc struct {
		ID          string `json:"id"`
		CompanyID   string `json:"company_id"`
		AccountCode string `json:"account_code"`
		AccountName string `json:"account_name"`
		AccountType string `json:"account_type"`
		IsPosting   bool   `json:"is_posting"`
		IsActive    bool   `json:"is_active"`
	}
	resp.decode(t, &acc)

	if acc.ID == "" {
		t.Fatal("expected a generated id")
	}
	if acc.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", acc.CompanyID, companyID)
	}
	if acc.AccountType != "ASSET" {
		t.Errorf("account_type = %q, want ASSET (normalized uppercase)", acc.AccountType)
	}
	if !acc.IsPosting {
		t.Error("expected is_posting to default true")
	}
	if !acc.IsActive {
		t.Error("expected is_active to default true")
	}
}

func TestCreateAccount_DuplicateCodeConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	payload := map[string]any{
		"company_id":   companyID,
		"account_code": "2000",
		"account_name": "Hutang Usaha",
		"account_type": "LIABILITY",
	}
	first := postJSON(t, srv.URL+"/accounts", payload)
	requireStatus(t, first, http.StatusCreated)

	second := postJSON(t, srv.URL+"/accounts", payload)
	requireStatus(t, second, http.StatusConflict)
	if msg := second.errorMessage(); msg == "" {
		t.Error("expected a non-empty error message on conflict")
	}
}

func TestUpdateAccount_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/accounts/"+uuid.NewString(), map[string]any{
		"account_name": "Updated Name",
		"is_posting":   true,
		"is_active":    true,
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateAccount_BlankNameRejected(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	acc := mustSeedAccount(t, srv, companyID, "EXPENSE")

	resp := doRequest(t, http.MethodPut, srv.URL+"/accounts/"+acc.ID, map[string]any{
		"account_name": "   ",
		"is_posting":   true,
		"is_active":    true,
	}, "")
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateAccount_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	acc := mustSeedAccount(t, srv, companyID, "EXPENSE")

	resp := doRequest(t, http.MethodPut, srv.URL+"/accounts/"+acc.ID, map[string]any{
		"account_name": "Renamed Expense Account",
		"is_posting":   false,
		"is_active":    false,
	}, "")
	requireStatus(t, resp, http.StatusOK)

	var updated struct {
		AccountName string `json:"account_name"`
		IsPosting   bool   `json:"is_posting"`
		IsActive    bool   `json:"is_active"`
	}
	resp.decode(t, &updated)
	if updated.AccountName != "Renamed Expense Account" {
		t.Errorf("account_name = %q, want %q", updated.AccountName, "Renamed Expense Account")
	}
	if updated.IsPosting {
		t.Error("expected is_posting to be false after update")
	}
	if updated.IsActive {
		t.Error("expected is_active to be false after update")
	}
}

func TestListAccounts_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	mustSeedAccount(t, srv, companyA, "ASSET")
	mustSeedAccount(t, srv, companyB, "ASSET")

	resp := getJSON(t, srv.URL+"/accounts?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)

	var accounts []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &accounts)
	if len(accounts) != 1 {
		t.Fatalf("expected exactly 1 account for companyA, got %d", len(accounts))
	}
	if accounts[0].CompanyID != companyA {
		t.Errorf("leaked account from another company: got company_id %q, want %q", accounts[0].CompanyID, companyA)
	}
}

func TestListAccounts_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/accounts")
	requireStatus(t, resp, http.StatusBadRequest)
}
