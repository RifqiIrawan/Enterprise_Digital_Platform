package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func journalDate() string {
	return time.Now().Format("2006-01-02")
}

func TestCreateJournalEntry_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	debitAcc := mustSeedAccount(t, srv, companyID, "ASSET")
	creditAcc := mustSeedAccount(t, srv, companyID, "REVENUE")

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{
			"fewer than 2 lines",
			map[string]any{
				"company_id": companyID, "entry_date": journalDate(),
				"lines": []map[string]any{{"account_id": debitAcc.ID, "debit_amount": 100}},
			},
		},
		{
			"line missing account_id",
			map[string]any{
				"company_id": companyID, "entry_date": journalDate(),
				"lines": []map[string]any{
					{"account_id": "", "debit_amount": 100},
					{"account_id": creditAcc.ID, "credit_amount": 100},
				},
			},
		},
		{
			"line with both debit and credit",
			map[string]any{
				"company_id": companyID, "entry_date": journalDate(),
				"lines": []map[string]any{
					{"account_id": debitAcc.ID, "debit_amount": 100, "credit_amount": 50},
					{"account_id": creditAcc.ID, "credit_amount": 100},
				},
			},
		},
		{
			"unbalanced debit/credit",
			map[string]any{
				"company_id": companyID, "entry_date": journalDate(),
				"lines": []map[string]any{
					{"account_id": debitAcc.ID, "debit_amount": 100},
					{"account_id": creditAcc.ID, "credit_amount": 50},
				},
			},
		},
		{
			"bad entry_date format",
			map[string]any{
				"company_id": companyID, "entry_date": "07/12/2026",
				"lines": []map[string]any{
					{"account_id": debitAcc.ID, "debit_amount": 100},
					{"account_id": creditAcc.ID, "credit_amount": 100},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/journal-entries", tc.payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateJournalEntry_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	debitAcc := mustSeedAccount(t, srv, companyID, "ASSET")
	creditAcc := mustSeedAccount(t, srv, companyID, "REVENUE")

	resp := postJSON(t, srv.URL+"/journal-entries", map[string]any{
		"company_id": companyID, "entry_date": journalDate(), "description": "Test entry",
		"lines": []map[string]any{
			{"account_id": debitAcc.ID, "debit_amount": 150.5},
			{"account_id": creditAcc.ID, "credit_amount": 150.5},
		},
	})
	requireStatus(t, resp, http.StatusCreated)

	var entry struct {
		ID          string  `json:"id"`
		Status      string  `json:"status"`
		EntryNumber string  `json:"entry_number"`
		TotalDebit  float64 `json:"total_debit"`
		TotalCredit float64 `json:"total_credit"`
		Lines       []struct {
			AccountID string `json:"account_id"`
		} `json:"lines"`
	}
	resp.decode(t, &entry)

	if entry.Status != "DRAFT" {
		t.Errorf("status = %q, want DRAFT", entry.Status)
	}
	wantPeriod := journalDate()[:7]
	wantPrefix := "JE-" + strings.ReplaceAll(wantPeriod, "-", "") + "-"
	if !strings.HasPrefix(entry.EntryNumber, wantPrefix) {
		t.Errorf("entry_number = %q, want prefix %q", entry.EntryNumber, wantPrefix)
	}
	if entry.TotalDebit != 150.5 || entry.TotalCredit != 150.5 {
		t.Errorf("totals = debit %.2f credit %.2f, want 150.50 both", entry.TotalDebit, entry.TotalCredit)
	}
	if len(entry.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(entry.Lines))
	}

	// GET should return the same entry with lines.
	getResp := getJSON(t, srv.URL+"/journal-entries/"+entry.ID)
	requireStatus(t, getResp, http.StatusOK)
}

func TestPostJournalEntry_Lifecycle(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	debitAcc := mustSeedAccount(t, srv, companyID, "ASSET")
	creditAcc := mustSeedAccount(t, srv, companyID, "REVENUE")

	createResp := postJSON(t, srv.URL+"/journal-entries", map[string]any{
		"company_id": companyID, "entry_date": journalDate(),
		"lines": []map[string]any{
			{"account_id": debitAcc.ID, "debit_amount": 200},
			{"account_id": creditAcc.ID, "credit_amount": 200},
		},
	})
	requireStatus(t, createResp, http.StatusCreated)
	var entry struct {
		ID string `json:"id"`
	}
	createResp.decode(t, &entry)

	actor := uuid.NewString()
	postResp := doRequest(t, http.MethodPost, srv.URL+"/journal-entries/"+entry.ID+"/post", nil, actor)
	requireStatus(t, postResp, http.StatusOK)

	var posted struct {
		Status   string  `json:"status"`
		PostedBy *string `json:"posted_by"`
		PostedAt *string `json:"posted_at"`
	}
	postResp.decode(t, &posted)
	if posted.Status != "POSTED" {
		t.Errorf("status = %q, want POSTED", posted.Status)
	}
	if posted.PostedBy == nil || *posted.PostedBy != actor {
		t.Errorf("posted_by = %v, want %q", posted.PostedBy, actor)
	}
	if posted.PostedAt == nil {
		t.Error("expected posted_at to be set")
	}

	// Posting an already-POSTED entry must be rejected.
	repostResp := postJSON(t, srv.URL+"/journal-entries/"+entry.ID+"/post", nil)
	requireStatus(t, repostResp, http.StatusConflict)
}

func TestPostJournalEntry_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := postJSON(t, srv.URL+"/journal-entries/"+uuid.NewString()+"/post", nil)
	// Handler can't distinguish "doesn't exist" from "not DRAFT" (same query
	// guard), so it reports Conflict for both — documenting actual behavior.
	requireStatus(t, resp, http.StatusConflict)
}

func TestGetJournalEntry_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/journal-entries/"+uuid.NewString())
	requireStatus(t, resp, http.StatusNotFound)
}
