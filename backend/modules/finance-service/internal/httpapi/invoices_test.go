package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func invoiceDate() string {
	return journalDate()
}

func TestCreateInvoice_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	controlAcc := mustSeedAccount(t, srv, companyID, "ASSET")
	revenueAcc := mustSeedAccount(t, srv, companyID, "REVENUE")

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{
			"missing partner_name",
			map[string]any{
				"company_id": companyID, "invoice_type": "AR", "invoice_date": invoiceDate(),
				"control_account_id": controlAcc.ID,
				"lines":              []map[string]any{{"account_id": revenueAcc.ID, "description": "Item", "quantity": 1, "unit_price": 100}},
			},
		},
		{
			"missing control_account_id",
			map[string]any{
				"company_id": companyID, "invoice_type": "AR", "invoice_date": invoiceDate(), "partner_name": "PT Uji",
				"lines": []map[string]any{{"account_id": revenueAcc.ID, "description": "Item", "quantity": 1, "unit_price": 100}},
			},
		},
		{
			"empty lines",
			map[string]any{
				"company_id": companyID, "invoice_type": "AR", "invoice_date": invoiceDate(), "partner_name": "PT Uji",
				"control_account_id": controlAcc.ID, "lines": []map[string]any{},
			},
		},
		{
			"invalid invoice_type",
			map[string]any{
				"company_id": companyID, "invoice_type": "XX", "invoice_date": invoiceDate(), "partner_name": "PT Uji",
				"control_account_id": controlAcc.ID,
				"lines":              []map[string]any{{"account_id": revenueAcc.ID, "description": "Item", "quantity": 1, "unit_price": 100}},
			},
		},
		{
			"bad invoice_date format",
			map[string]any{
				"company_id": companyID, "invoice_type": "AR", "invoice_date": "12-07-2026", "partner_name": "PT Uji",
				"control_account_id": controlAcc.ID,
				"lines":              []map[string]any{{"account_id": revenueAcc.ID, "description": "Item", "quantity": 1, "unit_price": 100}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/invoices", tc.payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

type journalLineView struct {
	AccountID    string  `json:"account_id"`
	DebitAmount  float64 `json:"debit_amount"`
	CreditAmount float64 `json:"credit_amount"`
}

type journalEntryView struct {
	ID          string            `json:"id"`
	Status      string            `json:"status"`
	TotalDebit  float64           `json:"total_debit"`
	TotalCredit float64           `json:"total_credit"`
	Lines       []journalLineView `json:"lines"`
}

func TestPostInvoice_ARCreatesBalancedJournal(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	arControlAcc := mustSeedAccount(t, srv, companyID, "ASSET") // Piutang Usaha
	revenueAcc := mustSeedAccount(t, srv, companyID, "REVENUE") // Pendapatan
	taxAcc := mustSeedAccount(t, srv, companyID, "LIABILITY")   // PPN Keluaran

	createResp := postJSON(t, srv.URL+"/invoices", map[string]any{
		"company_id": companyID, "invoice_type": "AR", "invoice_date": invoiceDate(),
		"partner_name": "PT Pelanggan Uji", "control_account_id": arControlAcc.ID,
		"tax_account_id": taxAcc.ID, "tax_amount": 11,
		"lines": []map[string]any{{"account_id": revenueAcc.ID, "description": "Jasa Konsultasi", "quantity": 1, "unit_price": 100}},
	})
	requireStatus(t, createResp, http.StatusCreated)
	var inv struct {
		ID          string  `json:"id"`
		Status      string  `json:"status"`
		SubtotalAmt float64 `json:"subtotal_amount"`
		TotalAmount float64 `json:"total_amount"`
	}
	createResp.decode(t, &inv)
	if inv.Status != "DRAFT" {
		t.Fatalf("status = %q, want DRAFT", inv.Status)
	}
	if inv.SubtotalAmt != 100 || inv.TotalAmount != 111 {
		t.Fatalf("subtotal=%.2f total=%.2f, want subtotal=100.00 total=111.00", inv.SubtotalAmt, inv.TotalAmount)
	}

	postResp := postJSON(t, srv.URL+"/invoices/"+inv.ID+"/post", nil)
	requireStatus(t, postResp, http.StatusOK)
	var posted struct {
		Status    string  `json:"status"`
		JournalID *string `json:"journal_id"`
	}
	postResp.decode(t, &posted)
	if posted.Status != "POSTED" {
		t.Fatalf("status = %q, want POSTED", posted.Status)
	}
	if posted.JournalID == nil {
		t.Fatal("expected journal_id to be set after posting")
	}

	journalResp := getJSON(t, srv.URL+"/journal-entries/"+*posted.JournalID)
	requireStatus(t, journalResp, http.StatusOK)
	var journal journalEntryView
	journalResp.decode(t, &journal)

	if journal.Status != "POSTED" {
		t.Errorf("auto-created journal status = %q, want POSTED", journal.Status)
	}
	if journal.TotalDebit != 111 || journal.TotalCredit != 111 {
		t.Errorf("journal totals = debit %.2f credit %.2f, want 111.00 both (unbalanced journal)", journal.TotalDebit, journal.TotalCredit)
	}
	if len(journal.Lines) != 3 {
		t.Fatalf("expected 3 journal lines (control + revenue + tax), got %d", len(journal.Lines))
	}

	var controlDebit, revenueCredit, taxCredit float64
	for _, l := range journal.Lines {
		switch l.AccountID {
		case arControlAcc.ID:
			controlDebit = l.DebitAmount
		case revenueAcc.ID:
			revenueCredit = l.CreditAmount
		case taxAcc.ID:
			taxCredit = l.CreditAmount
		}
	}
	if controlDebit != 111 {
		t.Errorf("AR control account debit = %.2f, want 111.00", controlDebit)
	}
	if revenueCredit != 100 {
		t.Errorf("revenue account credit = %.2f, want 100.00", revenueCredit)
	}
	if taxCredit != 11 {
		t.Errorf("tax account credit = %.2f, want 11.00", taxCredit)
	}

	// Posting an already-POSTED invoice must be rejected.
	repostResp := postJSON(t, srv.URL+"/invoices/"+inv.ID+"/post", nil)
	requireStatus(t, repostResp, http.StatusConflict)
}

func TestPostInvoice_APCreatesMirroredJournal(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	apControlAcc := mustSeedAccount(t, srv, companyID, "LIABILITY") // Hutang Usaha
	expenseAcc := mustSeedAccount(t, srv, companyID, "EXPENSE")

	createResp := postJSON(t, srv.URL+"/invoices", map[string]any{
		"company_id": companyID, "invoice_type": "AP", "invoice_date": invoiceDate(),
		"partner_name": "PT Pemasok Uji", "control_account_id": apControlAcc.ID,
		"lines": []map[string]any{{"account_id": expenseAcc.ID, "description": "Bahan Baku", "quantity": 2, "unit_price": 50}},
	})
	requireStatus(t, createResp, http.StatusCreated)
	var inv struct {
		ID string `json:"id"`
	}
	createResp.decode(t, &inv)

	postResp := postJSON(t, srv.URL+"/invoices/"+inv.ID+"/post", nil)
	requireStatus(t, postResp, http.StatusOK)
	var posted struct {
		JournalID *string `json:"journal_id"`
	}
	postResp.decode(t, &posted)

	journalResp := getJSON(t, srv.URL+"/journal-entries/"+*posted.JournalID)
	requireStatus(t, journalResp, http.StatusOK)
	var journal journalEntryView
	journalResp.decode(t, &journal)

	var controlCredit, expenseDebit float64
	for _, l := range journal.Lines {
		switch l.AccountID {
		case apControlAcc.ID:
			controlCredit = l.CreditAmount
		case expenseAcc.ID:
			expenseDebit = l.DebitAmount
		}
	}
	if controlCredit != 100 {
		t.Errorf("AP control account credit = %.2f, want 100.00", controlCredit)
	}
	if expenseDebit != 100 {
		t.Errorf("expense account debit = %.2f, want 100.00", expenseDebit)
	}
}

func TestArApSummary_ExcludesDraftInvoices(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	arControlAcc := mustSeedAccount(t, srv, companyID, "ASSET")
	revenueAcc := mustSeedAccount(t, srv, companyID, "REVENUE")
	apControlAcc := mustSeedAccount(t, srv, companyID, "LIABILITY")
	expenseAcc := mustSeedAccount(t, srv, companyID, "EXPENSE")

	// A posted AR invoice: should be included.
	arResp := postJSON(t, srv.URL+"/invoices", map[string]any{
		"company_id": companyID, "invoice_type": "AR", "invoice_date": invoiceDate(),
		"partner_name": "PT A", "control_account_id": arControlAcc.ID,
		"lines": []map[string]any{{"account_id": revenueAcc.ID, "description": "Item", "quantity": 1, "unit_price": 500}},
	})
	requireStatus(t, arResp, http.StatusCreated)
	var arInv struct {
		ID string `json:"id"`
	}
	arResp.decode(t, &arInv)
	requireStatus(t, postJSON(t, srv.URL+"/invoices/"+arInv.ID+"/post", nil), http.StatusOK)

	// A DRAFT AP invoice: should be excluded from the summary.
	apResp := postJSON(t, srv.URL+"/invoices", map[string]any{
		"company_id": companyID, "invoice_type": "AP", "invoice_date": invoiceDate(),
		"partner_name": "PT B", "control_account_id": apControlAcc.ID,
		"lines": []map[string]any{{"account_id": expenseAcc.ID, "description": "Item", "quantity": 1, "unit_price": 300}},
	})
	requireStatus(t, apResp, http.StatusCreated)

	summaryResp := getJSON(t, srv.URL+"/ar-ap-summary?company_id="+companyID)
	requireStatus(t, summaryResp, http.StatusOK)
	var summary []struct {
		InvoiceType       string  `json:"invoice_type"`
		Count             int     `json:"count"`
		TotalAmount       float64 `json:"total_amount"`
		OutstandingAmount float64 `json:"outstanding_amount"`
	}
	summaryResp.decode(t, &summary)

	if len(summary) != 1 {
		t.Fatalf("expected only the AR (posted) group in summary, got %d groups: %+v", len(summary), summary)
	}
	if summary[0].InvoiceType != "AR" || summary[0].Count != 1 || summary[0].TotalAmount != 500 {
		t.Errorf("unexpected AR summary: %+v", summary[0])
	}
	if summary[0].OutstandingAmount != 500 {
		t.Errorf("outstanding_amount = %.2f, want 500.00 (nothing paid yet)", summary[0].OutstandingAmount)
	}
}

func TestGetInvoice_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/invoices/"+uuid.NewString())
	requireStatus(t, resp, http.StatusNotFound)
}

func TestListInvoices_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/invoices")
	requireStatus(t, resp, http.StatusBadRequest)
}
