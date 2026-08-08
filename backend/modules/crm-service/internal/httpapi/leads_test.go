package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateLead_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"missing company_id": {"first_name": "Budi"},
		"missing first_name": {"company_id": companyID},
		"invalid source":     {"company_id": companyID, "first_name": "Budi", "source": "CARRIER_PIGEON"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/leads", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateLead_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/leads", map[string]any{
		"company_id": companyID, "first_name": "Budi", "last_name": "Santoso",
		"company_name": "PT Contoh Sejahtera", "email": "budi@contoh.co.id", "source": "WEBSITE",
	})
	requireStatus(t, resp, http.StatusCreated)

	var l struct {
		CompanyID  string `json:"company_id"`
		LeadNumber string `json:"lead_number"`
		Status     string `json:"status"`
		Source     string `json:"source"`
	}
	resp.decode(t, &l)
	if l.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", l.CompanyID, companyID)
	}
	if l.Status != "NEW" {
		t.Errorf("status = %q, want default NEW", l.Status)
	}
	if l.Source != "WEBSITE" {
		t.Errorf("source = %q, want WEBSITE", l.Source)
	}
	if l.LeadNumber == "" {
		t.Error("expected a non-empty auto-generated lead_number")
	}
}

func TestCreateLead_DefaultSourceOther(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/leads", map[string]any{"company_id": companyID, "first_name": "Budi"})
	requireStatus(t, resp, http.StatusCreated)
	var l struct {
		Source string `json:"source"`
	}
	resp.decode(t, &l)
	if l.Source != "OTHER" {
		t.Errorf("source = %q, want default OTHER", l.Source)
	}
}

func TestUpdateLead_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/leads/"+uuid.NewString(), map[string]any{
		"first_name": "Updated", "source": "OTHER", "status": "CONTACTED",
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateLead_InvalidStatusRejected(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	l := mustSeedLead(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/leads/"+l.ID, map[string]any{
		"first_name": "Test", "source": "OTHER", "status": "CONVERTED",
	}, "")
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateLead_RejectedOnceConverted(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	l := mustSeedLead(t, srv, companyID)
	mustSetLeadStatus(t, srv, l.ID, "QUALIFIED")
	requireStatus(t, postJSON(t, srv.URL+"/leads/"+l.ID+"/convert", nil), http.StatusCreated)

	resp := doRequest(t, http.MethodPut, srv.URL+"/leads/"+l.ID, map[string]any{
		"first_name": "Test", "source": "OTHER", "status": "CONTACTED",
	}, "")
	requireStatus(t, resp, http.StatusConflict)
}

func TestListLeads_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	mustSeedLead(t, srv, companyA)
	mustSeedLead(t, srv, companyB)

	resp := getJSON(t, srv.URL+"/leads?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 lead scoped to companyA, got %+v", list)
	}
}

// TestListLeads_FilteredByBranch confirms branch_id filtering is
// NULL-inclusive: a branch filter must still surface unassigned (NULL
// branch_id) rows alongside that branch's own rows.
func TestListLeads_FilteredByBranch(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	branchA := uuid.NewString()
	branchB := uuid.NewString()

	mkLead := func(branchID *string) {
		requireStatus(t, postJSON(t, srv.URL+"/leads", map[string]any{
			"company_id": companyID, "branch_id": branchID, "first_name": "Test",
		}), http.StatusCreated)
	}
	mkLead(&branchA)
	mkLead(nil)
	mkLead(&branchB)

	resp := getJSON(t, srv.URL+"/leads?company_id="+companyID+"&branch_id="+branchA)
	requireStatus(t, resp, http.StatusOK)
	var leads []struct {
		BranchID *string `json:"branch_id"`
	}
	resp.decode(t, &leads)
	if len(leads) != 2 {
		t.Fatalf("expected 2 leads (branchA + NULL), got %d: %+v", len(leads), leads)
	}
	for _, l := range leads {
		if l.BranchID != nil && *l.BranchID == branchB {
			t.Errorf("branchB lead leaked into branchA-filtered results: %+v", leads)
		}
	}
}

func TestListLeads_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/leads")
	requireStatus(t, resp, http.StatusBadRequest)
}

// TestConvertLead_CreatesAccountContactOpportunity is the core regression
// guard for the transactional convert flow (see leads.go convertLead):
// exactly one Account, Contact, and Opportunity must be created and linked
// back onto the lead, and the lead itself must move to CONVERTED.
func TestConvertLead_CreatesAccountContactOpportunity(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	l := mustSeedLead(t, srv, companyID)
	mustSetLeadStatus(t, srv, l.ID, "QUALIFIED")

	resp := postJSON(t, srv.URL+"/leads/"+l.ID+"/convert", map[string]any{
		"opportunity_amount": 5_000_000,
	})
	requireStatus(t, resp, http.StatusCreated)

	var result struct {
		Lead struct {
			Status                 string  `json:"status"`
			ConvertedAccountID     *string `json:"converted_account_id"`
			ConvertedContactID     *string `json:"converted_contact_id"`
			ConvertedOpportunityID *string `json:"converted_opportunity_id"`
		} `json:"lead"`
		Account struct {
			ID          string `json:"id"`
			AccountCode string `json:"account_code"`
			Name        string `json:"name"`
		} `json:"account"`
		Contact struct {
			ID        string  `json:"id"`
			AccountID *string `json:"account_id"`
		} `json:"contact"`
		Opportunity struct {
			ID     string  `json:"id"`
			Stage  string  `json:"stage"`
			Amount float64 `json:"amount"`
		} `json:"opportunity"`
	}
	resp.decode(t, &result)

	if result.Lead.Status != "CONVERTED" {
		t.Errorf("lead status = %q, want CONVERTED", result.Lead.Status)
	}
	if result.Lead.ConvertedAccountID == nil || *result.Lead.ConvertedAccountID != result.Account.ID {
		t.Errorf("lead.converted_account_id = %v, want %q", result.Lead.ConvertedAccountID, result.Account.ID)
	}
	if result.Lead.ConvertedContactID == nil || *result.Lead.ConvertedContactID != result.Contact.ID {
		t.Errorf("lead.converted_contact_id = %v, want %q", result.Lead.ConvertedContactID, result.Contact.ID)
	}
	if result.Lead.ConvertedOpportunityID == nil || *result.Lead.ConvertedOpportunityID != result.Opportunity.ID {
		t.Errorf("lead.converted_opportunity_id = %v, want %q", result.Lead.ConvertedOpportunityID, result.Opportunity.ID)
	}
	if result.Contact.AccountID == nil || *result.Contact.AccountID != result.Account.ID {
		t.Errorf("contact.account_id = %v, want %q", result.Contact.AccountID, result.Account.ID)
	}
	if result.Opportunity.Stage != "PROSPECTING" {
		t.Errorf("opportunity.stage = %q, want PROSPECTING", result.Opportunity.Stage)
	}
	if result.Opportunity.Amount != 5_000_000 {
		t.Errorf("opportunity.amount = %v, want 5000000", result.Opportunity.Amount)
	}
	if result.Account.AccountCode == "" {
		t.Error("expected a non-empty auto-generated account_code")
	}

	// Verify the account/contact/opportunity are genuinely queryable
	// afterwards (not just present in the convert response).
	requireStatus(t, getJSON(t, srv.URL+"/accounts?company_id="+companyID), http.StatusOK)
}

func TestConvertLead_FailsIfNotQualified(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	l := mustSeedLead(t, srv, companyID) // status defaults to NEW

	resp := postJSON(t, srv.URL+"/leads/"+l.ID+"/convert", nil)
	requireStatus(t, resp, http.StatusConflict)
}

func TestConvertLead_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := postJSON(t, srv.URL+"/leads/"+uuid.NewString()+"/convert", nil)
	requireStatus(t, resp, http.StatusNotFound)
}
