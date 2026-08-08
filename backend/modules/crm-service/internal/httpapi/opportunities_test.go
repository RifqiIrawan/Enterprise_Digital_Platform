package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateOpportunity_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAccount(t, srv, companyID)

	cases := map[string]map[string]any{
		"missing company_id": {"account_id": a.ID, "name": "Deal"},
		"missing account_id": {"company_id": companyID, "name": "Deal"},
		"missing name":       {"company_id": companyID, "account_id": a.ID},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/opportunities", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateOpportunity_AccountNotFound(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/opportunities", map[string]any{
		"company_id": companyID, "account_id": uuid.NewString(), "name": "Deal",
	})
	requireStatus(t, resp, http.StatusNotFound)
}

func TestCreateOpportunity_ContactNotFound(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAccount(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/opportunities", map[string]any{
		"company_id": companyID, "account_id": a.ID, "contact_id": uuid.NewString(), "name": "Deal",
	})
	requireStatus(t, resp, http.StatusNotFound)
}

func TestCreateOpportunity_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAccount(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/opportunities", map[string]any{
		"company_id": companyID, "account_id": a.ID, "name": "Deal Besar", "amount": 10_000_000,
	})
	requireStatus(t, resp, http.StatusCreated)

	var o struct {
		CompanyID         string  `json:"company_id"`
		OpportunityNumber string  `json:"opportunity_number"`
		Stage             string  `json:"stage"`
		Amount            float64 `json:"amount"`
	}
	resp.decode(t, &o)
	if o.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", o.CompanyID, companyID)
	}
	if o.Stage != "PROSPECTING" {
		t.Errorf("stage = %q, want default PROSPECTING", o.Stage)
	}
	if o.Amount != 10_000_000 {
		t.Errorf("amount = %v, want 10000000", o.Amount)
	}
	if o.OpportunityNumber == "" {
		t.Error("expected a non-empty auto-generated opportunity_number")
	}
}

func TestUpdateOpportunity_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/opportunities/"+uuid.NewString(), map[string]any{
		"name": "Updated", "stage": "PROPOSAL",
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateOpportunity_InvalidStageRejected(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAccount(t, srv, companyID)
	o := mustSeedOpportunity(t, srv, companyID, a.ID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/opportunities/"+o.ID, map[string]any{
		"name": "Deal", "stage": "WON",
	}, "")
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateOpportunity_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAccount(t, srv, companyID)
	o := mustSeedOpportunity(t, srv, companyID, a.ID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/opportunities/"+o.ID, map[string]any{
		"name": "Deal Direvisi", "stage": "PROPOSAL", "amount": 20_000_000, "probability": 50,
	}, "")
	requireStatus(t, resp, http.StatusOK)

	var updated struct {
		Name        string  `json:"name"`
		Stage       string  `json:"stage"`
		Amount      float64 `json:"amount"`
		Probability int16   `json:"probability"`
	}
	resp.decode(t, &updated)
	if updated.Name != "Deal Direvisi" || updated.Stage != "PROPOSAL" || updated.Amount != 20_000_000 || updated.Probability != 50 {
		t.Errorf("unexpected update result: %+v", updated)
	}
}

func TestWinOpportunity_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAccount(t, srv, companyID)
	o := mustSeedOpportunity(t, srv, companyID, a.ID)

	resp := postJSON(t, srv.URL+"/opportunities/"+o.ID+"/win", nil)
	requireStatus(t, resp, http.StatusOK)

	var won struct {
		Stage       string `json:"stage"`
		Probability int16  `json:"probability"`
	}
	resp.decode(t, &won)
	if won.Stage != "WON" || won.Probability != 100 {
		t.Errorf("unexpected win result: %+v", won)
	}
}

func TestLoseOpportunity_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAccount(t, srv, companyID)
	o := mustSeedOpportunity(t, srv, companyID, a.ID)

	resp := postJSON(t, srv.URL+"/opportunities/"+o.ID+"/lose", map[string]any{"lost_reason": "Harga kalah bersaing"})
	requireStatus(t, resp, http.StatusOK)

	var lost struct {
		Stage      string `json:"stage"`
		LostReason string `json:"lost_reason"`
	}
	resp.decode(t, &lost)
	if lost.Stage != "LOST" || lost.LostReason != "Harga kalah bersaing" {
		t.Errorf("unexpected lose result: %+v", lost)
	}
}

// TestUpdateOpportunity_RejectedOnceWon and TestWinOpportunity_RejectedIfAlreadyClosed
// confirm WON/LOST are truly terminal: no further PUT edits, and win/lose
// can't be re-applied to an already-closed opportunity.
func TestUpdateOpportunity_RejectedOnceWon(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAccount(t, srv, companyID)
	o := mustSeedOpportunity(t, srv, companyID, a.ID)
	requireStatus(t, postJSON(t, srv.URL+"/opportunities/"+o.ID+"/win", nil), http.StatusOK)

	resp := doRequest(t, http.MethodPut, srv.URL+"/opportunities/"+o.ID, map[string]any{
		"name": "Deal", "stage": "PROPOSAL",
	}, "")
	requireStatus(t, resp, http.StatusConflict)
}

func TestWinOpportunity_RejectedIfAlreadyClosed(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAccount(t, srv, companyID)
	o := mustSeedOpportunity(t, srv, companyID, a.ID)
	requireStatus(t, postJSON(t, srv.URL+"/opportunities/"+o.ID+"/lose", map[string]any{"lost_reason": "x"}), http.StatusOK)

	resp := postJSON(t, srv.URL+"/opportunities/"+o.ID+"/win", nil)
	requireStatus(t, resp, http.StatusConflict)
}

func TestListOpportunities_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)
	accountA := mustSeedAccount(t, srv, companyA)
	accountB := mustSeedAccount(t, srv, companyB)

	mustSeedOpportunity(t, srv, companyA, accountA.ID)
	mustSeedOpportunity(t, srv, companyB, accountB.ID)

	resp := getJSON(t, srv.URL+"/opportunities?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 opportunity scoped to companyA, got %+v", list)
	}
}

func TestListOpportunities_FilteredByStage(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAccount(t, srv, companyID)
	o1 := mustSeedOpportunity(t, srv, companyID, a.ID)
	mustSeedOpportunity(t, srv, companyID, a.ID)
	requireStatus(t, postJSON(t, srv.URL+"/opportunities/"+o1.ID+"/win", nil), http.StatusOK)

	resp := getJSON(t, srv.URL+"/opportunities?company_id="+companyID+"&stage=WON")
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		Stage string `json:"stage"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].Stage != "WON" {
		t.Fatalf("expected exactly 1 WON opportunity, got %+v", list)
	}
}

func TestListOpportunities_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/opportunities")
	requireStatus(t, resp, http.StatusBadRequest)
}
