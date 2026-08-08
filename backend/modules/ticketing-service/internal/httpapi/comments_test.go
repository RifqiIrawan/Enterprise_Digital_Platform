package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateComment_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	c := mustSeedCategory(t, srv, companyID)
	ti := mustSeedTicket(t, srv, companyID, c.ID)

	cases := map[string]map[string]any{
		"missing company_id":   {"ticket_id": ti.ID, "comment_text": "Halo"},
		"missing ticket_id":    {"company_id": companyID, "comment_text": "Halo"},
		"missing comment_text": {"company_id": companyID, "ticket_id": ti.ID},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/comments", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateComment_RejectsUnknownTicket(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/comments", map[string]any{
		"company_id": companyID, "ticket_id": uuid.NewString(), "comment_text": "Halo",
	})
	requireStatus(t, resp, http.StatusNotFound)
}

func TestCreateComment_RejectsTicketFromAnotherCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)
	catA := mustSeedCategory(t, srv, companyA)
	tiA := mustSeedTicket(t, srv, companyA, catA.ID)

	// Guessing a real ticket ID that belongs to a different company must
	// still 404 -- the company_id filter in the EXISTS-equivalent lookup is
	// mandatory, same guard as crm-service's activities.referenceExists.
	resp := postJSON(t, srv.URL+"/comments", map[string]any{
		"company_id": companyB, "ticket_id": tiA.ID, "comment_text": "Halo",
	})
	requireStatus(t, resp, http.StatusNotFound)
}

func TestCreateComment_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	c := mustSeedCategory(t, srv, companyID)
	ti := mustSeedTicket(t, srv, companyID, c.ID)

	resp := postJSON(t, srv.URL+"/comments", map[string]any{
		"company_id": companyID, "ticket_id": ti.ID, "comment_text": "Sudah kami cek", "is_internal": true,
	})
	requireStatus(t, resp, http.StatusCreated)

	var cm struct {
		CompanyID   string `json:"company_id"`
		TicketID    string `json:"ticket_id"`
		CommentText string `json:"comment_text"`
		IsInternal  bool   `json:"is_internal"`
	}
	resp.decode(t, &cm)
	if cm.CompanyID != companyID || cm.TicketID != ti.ID {
		t.Errorf("unexpected scoping: %+v", cm)
	}
	if cm.CommentText != "Sudah kami cek" || !cm.IsInternal {
		t.Errorf("unexpected create result: %+v", cm)
	}
}

func TestListComments_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)
	catA := mustSeedCategory(t, srv, companyA)
	catB := mustSeedCategory(t, srv, companyB)
	tiA := mustSeedTicket(t, srv, companyA, catA.ID)
	tiB := mustSeedTicket(t, srv, companyB, catB.ID)

	requireStatus(t, postJSON(t, srv.URL+"/comments", map[string]any{
		"company_id": companyA, "ticket_id": tiA.ID, "comment_text": "A",
	}), http.StatusCreated)
	requireStatus(t, postJSON(t, srv.URL+"/comments", map[string]any{
		"company_id": companyB, "ticket_id": tiB.ID, "comment_text": "B",
	}), http.StatusCreated)

	resp := getJSON(t, srv.URL+"/comments?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 comment scoped to companyA, got %+v", list)
	}
}

func TestListComments_FilteredByTicket(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	c := mustSeedCategory(t, srv, companyID)
	ti1 := mustSeedTicket(t, srv, companyID, c.ID)
	ti2 := mustSeedTicket(t, srv, companyID, c.ID)

	requireStatus(t, postJSON(t, srv.URL+"/comments", map[string]any{
		"company_id": companyID, "ticket_id": ti1.ID, "comment_text": "For ticket 1",
	}), http.StatusCreated)
	requireStatus(t, postJSON(t, srv.URL+"/comments", map[string]any{
		"company_id": companyID, "ticket_id": ti2.ID, "comment_text": "For ticket 2",
	}), http.StatusCreated)

	resp := getJSON(t, srv.URL+"/comments?company_id="+companyID+"&ticket_id="+ti1.ID)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		TicketID string `json:"ticket_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].TicketID != ti1.ID {
		t.Fatalf("expected exactly 1 comment scoped to ticket 1, got %+v", list)
	}
}

func TestListComments_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/comments")
	requireStatus(t, resp, http.StatusBadRequest)
}
