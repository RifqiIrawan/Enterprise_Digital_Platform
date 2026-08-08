package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateContact_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"missing company_id": {"first_name": "Siti"},
		"missing first_name": {"company_id": companyID},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/contacts", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateContact_AccountNotFound(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/contacts", map[string]any{
		"company_id": companyID, "account_id": uuid.NewString(), "first_name": "Siti",
	})
	requireStatus(t, resp, http.StatusNotFound)
}

// TestCreateContact_AccountFromOtherCompanyRejected confirms the account_id
// existence check is scoped by company_id, not just id -- a contact must
// not be linkable to another company's account by guessing its UUID.
func TestCreateContact_AccountFromOtherCompanyRejected(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)
	accountB := mustSeedAccount(t, srv, companyB)

	resp := postJSON(t, srv.URL+"/contacts", map[string]any{
		"company_id": companyA, "account_id": accountB.ID, "first_name": "Siti",
	})
	requireStatus(t, resp, http.StatusNotFound)
}

func TestCreateContact_SuccessWithoutAccount(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/contacts", map[string]any{
		"company_id": companyID, "first_name": "Siti", "last_name": "Aminah",
	})
	requireStatus(t, resp, http.StatusCreated)
	var c struct {
		CompanyID string  `json:"company_id"`
		AccountID *string `json:"account_id"`
	}
	resp.decode(t, &c)
	if c.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", c.CompanyID, companyID)
	}
	if c.AccountID != nil {
		t.Errorf("account_id = %v, want nil (contact without account)", c.AccountID)
	}
}

func TestCreateContact_SuccessWithAccount(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAccount(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/contacts", map[string]any{
		"company_id": companyID, "account_id": a.ID, "first_name": "Siti", "is_primary": true,
	})
	requireStatus(t, resp, http.StatusCreated)
	var c struct {
		AccountID *string `json:"account_id"`
		IsPrimary bool    `json:"is_primary"`
	}
	resp.decode(t, &c)
	if c.AccountID == nil || *c.AccountID != a.ID {
		t.Errorf("account_id = %v, want %q", c.AccountID, a.ID)
	}
	if !c.IsPrimary {
		t.Error("is_primary = false, want true")
	}
}

func TestUpdateContact_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/contacts/"+uuid.NewString(), map[string]any{
		"first_name": "Updated",
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestListContacts_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)
	accountA := mustSeedAccount(t, srv, companyA)
	accountB := mustSeedAccount(t, srv, companyB)

	mustSeedContact(t, srv, companyA, accountA.ID)
	mustSeedContact(t, srv, companyB, accountB.ID)

	resp := getJSON(t, srv.URL+"/contacts?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 contact scoped to companyA, got %+v", list)
	}
}

func TestListContacts_FilteredByAccountID(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	accountA := mustSeedAccount(t, srv, companyID)
	accountB := mustSeedAccount(t, srv, companyID)

	mustSeedContact(t, srv, companyID, accountA.ID)
	mustSeedContact(t, srv, companyID, accountB.ID)

	resp := getJSON(t, srv.URL+"/contacts?company_id="+companyID+"&account_id="+accountA.ID)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		AccountID *string `json:"account_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].AccountID == nil || *list[0].AccountID != accountA.ID {
		t.Fatalf("expected exactly 1 contact scoped to accountA, got %+v", list)
	}
}

func TestListContacts_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/contacts")
	requireStatus(t, resp, http.StatusBadRequest)
}
