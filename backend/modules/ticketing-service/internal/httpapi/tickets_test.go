package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateTicket_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	c := mustSeedCategory(t, srv, companyID)

	cases := map[string]map[string]any{
		"missing company_id":     {"category_id": c.ID, "subject": "Help", "requester_name": "Budi"},
		"missing category_id":    {"company_id": companyID, "subject": "Help", "requester_name": "Budi"},
		"missing subject":        {"company_id": companyID, "category_id": c.ID, "requester_name": "Budi"},
		"missing requester_name": {"company_id": companyID, "category_id": c.ID, "subject": "Help"},
		"invalid priority":       {"company_id": companyID, "category_id": c.ID, "subject": "Help", "requester_name": "Budi", "priority": "WHENEVER"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/tickets", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateTicket_RejectsUnknownCategory(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/tickets", map[string]any{
		"company_id": companyID, "category_id": uuid.NewString(), "subject": "Help", "requester_name": "Budi",
	})
	requireStatus(t, resp, http.StatusNotFound)
}

func TestCreateTicket_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	c := mustSeedCategory(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/tickets", map[string]any{
		"company_id": companyID, "category_id": c.ID, "subject": "Login gagal", "requester_name": "Budi", "requester_email": "budi@example.com",
	})
	requireStatus(t, resp, http.StatusCreated)

	var ti struct {
		CompanyID    string `json:"company_id"`
		TicketNumber string `json:"ticket_number"`
		Status       string `json:"status"`
		Priority     string `json:"priority"`
	}
	resp.decode(t, &ti)
	if ti.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", ti.CompanyID, companyID)
	}
	if ti.Status != "OPEN" {
		t.Errorf("status = %q, want default OPEN", ti.Status)
	}
	if ti.Priority != "MEDIUM" {
		t.Errorf("priority = %q, want default MEDIUM", ti.Priority)
	}
	if ti.TicketNumber == "" {
		t.Error("expected a non-empty auto-generated ticket_number")
	}
}

func TestUpdateTicket_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/tickets/"+uuid.NewString(), map[string]any{
		"subject": "Updated", "priority": "HIGH", "status": "OPEN", "requester_name": "Budi",
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateTicket_InvalidStatusRejected(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	c := mustSeedCategory(t, srv, companyID)
	ti := mustSeedTicket(t, srv, companyID, c.ID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/tickets/"+ti.ID, map[string]any{
		"subject": "Ticket", "priority": "MEDIUM", "status": "CLOSED", "requester_name": "Budi",
	}, "")
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateTicket_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	c := mustSeedCategory(t, srv, companyID)
	ti := mustSeedTicket(t, srv, companyID, c.ID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/tickets/"+ti.ID, map[string]any{
		"subject": "Ticket Direvisi", "priority": "HIGH", "status": "RESOLVED", "requester_name": "Budi",
	}, "")
	requireStatus(t, resp, http.StatusOK)

	var updated struct {
		Subject    string  `json:"subject"`
		Priority   string  `json:"priority"`
		Status     string  `json:"status"`
		ResolvedAt *string `json:"resolved_at"`
	}
	resp.decode(t, &updated)
	if updated.Subject != "Ticket Direvisi" || updated.Priority != "HIGH" || updated.Status != "RESOLVED" {
		t.Errorf("unexpected update result: %+v", updated)
	}
	if updated.ResolvedAt == nil {
		t.Error("expected resolved_at to be set when status becomes RESOLVED")
	}
}

func TestUpdateTicket_RejectsAfterClosed(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	c := mustSeedCategory(t, srv, companyID)
	ti := mustSeedTicket(t, srv, companyID, c.ID)
	requireStatus(t, postJSON(t, srv.URL+"/tickets/"+ti.ID+"/close", nil), http.StatusOK)

	resp := doRequest(t, http.MethodPut, srv.URL+"/tickets/"+ti.ID, map[string]any{
		"subject": "Ticket", "priority": "MEDIUM", "status": "OPEN", "requester_name": "Budi",
	}, "")
	requireStatus(t, resp, http.StatusConflict)
}

func TestCloseTicket_ThenReopen(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	c := mustSeedCategory(t, srv, companyID)
	ti := mustSeedTicket(t, srv, companyID, c.ID)

	closeResp := postJSON(t, srv.URL+"/tickets/"+ti.ID+"/close", nil)
	requireStatus(t, closeResp, http.StatusOK)
	var closed struct {
		Status   string `json:"status"`
		ClosedAt string `json:"closed_at"`
	}
	closeResp.decode(t, &closed)
	if closed.Status != "CLOSED" || closed.ClosedAt == "" {
		t.Errorf("unexpected close result: %+v", closed)
	}

	// Closing again must fail -- CLOSED is a genuine terminal state until reopened.
	requireStatus(t, postJSON(t, srv.URL+"/tickets/"+ti.ID+"/close", nil), http.StatusConflict)

	reopenResp := postJSON(t, srv.URL+"/tickets/"+ti.ID+"/reopen", nil)
	requireStatus(t, reopenResp, http.StatusOK)
	var reopened struct {
		Status   string  `json:"status"`
		ClosedAt *string `json:"closed_at"`
	}
	reopenResp.decode(t, &reopened)
	if reopened.Status != "OPEN" || reopened.ClosedAt != nil {
		t.Errorf("unexpected reopen result: %+v", reopened)
	}

	// Reopening a ticket that isn't CLOSED must fail.
	requireStatus(t, postJSON(t, srv.URL+"/tickets/"+ti.ID+"/reopen", nil), http.StatusConflict)
}

func TestListTickets_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)
	catA := mustSeedCategory(t, srv, companyA)
	catB := mustSeedCategory(t, srv, companyB)

	mustSeedTicket(t, srv, companyA, catA.ID)
	mustSeedTicket(t, srv, companyB, catB.ID)

	resp := getJSON(t, srv.URL+"/tickets?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 ticket scoped to companyA, got %+v", list)
	}
}

func TestListTickets_FilteredByStatus(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	c := mustSeedCategory(t, srv, companyID)
	ti1 := mustSeedTicket(t, srv, companyID, c.ID)
	mustSeedTicket(t, srv, companyID, c.ID)
	requireStatus(t, postJSON(t, srv.URL+"/tickets/"+ti1.ID+"/close", nil), http.StatusOK)

	resp := getJSON(t, srv.URL+"/tickets?company_id="+companyID+"&status=CLOSED")
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		Status string `json:"status"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].Status != "CLOSED" {
		t.Fatalf("expected exactly 1 CLOSED ticket, got %+v", list)
	}
}

func TestListTickets_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/tickets")
	requireStatus(t, resp, http.StatusBadRequest)
}
