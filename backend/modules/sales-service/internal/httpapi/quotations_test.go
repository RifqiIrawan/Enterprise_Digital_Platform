package httpapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func today() string {
	return time.Now().Format("2006-01-02")
}

func TestCreateQuotation_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	cust := mustSeedCustomer(t, srv, companyID)

	cases := map[string]map[string]any{
		"missing customer_id": {
			"company_id": companyID, "quotation_date": today(),
			"lines": []map[string]any{{"product_name": "Item A", "quantity": 1, "unit_price": 100}},
		},
		"empty lines": {
			"company_id": companyID, "customer_id": cust.ID, "quotation_date": today(), "lines": []map[string]any{},
		},
		"line missing product_name": {
			"company_id": companyID, "customer_id": cust.ID, "quotation_date": today(),
			"lines": []map[string]any{{"product_name": "", "quantity": 1, "unit_price": 100}},
		},
		"bad quotation_date format": {
			"company_id": companyID, "customer_id": cust.ID, "quotation_date": "01-07-2026",
			"lines": []map[string]any{{"product_name": "Item A", "quantity": 1, "unit_price": 100}},
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/quotations", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

type quotationView struct {
	ID              string  `json:"id"`
	Status          string  `json:"status"`
	QuotationNumber string  `json:"quotation_number"`
	SubtotalAmount  float64 `json:"subtotal_amount"`
	TotalAmount     float64 `json:"total_amount"`
}

func mustCreateQuotationOn(t *testing.T, srvURL, companyID, customerID string, taxAmount float64) quotationView {
	t.Helper()
	resp := postJSON(t, srvURL+"/quotations", map[string]any{
		"company_id": companyID, "customer_id": customerID, "quotation_date": today(), "tax_amount": taxAmount,
		"lines": []map[string]any{{"product_name": "Jasa Konsultasi", "quantity": 2, "unit_price": 500}},
	})
	requireStatus(t, resp, http.StatusCreated)
	var q quotationView
	resp.decode(t, &q)
	return q
}

func TestCreateQuotation_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	cust := mustSeedCustomer(t, srv, companyID)

	q := mustCreateQuotationOn(t, srv.URL, companyID, cust.ID, 100)
	if q.Status != "DRAFT" {
		t.Errorf("status = %q, want DRAFT", q.Status)
	}
	if q.SubtotalAmount != 1000 || q.TotalAmount != 1100 {
		t.Errorf("subtotal=%.2f total=%.2f, want subtotal=1000.00 total=1100.00", q.SubtotalAmount, q.TotalAmount)
	}

	getResp := getJSON(t, srv.URL+"/quotations/"+q.ID)
	requireStatus(t, getResp, http.StatusOK)
}

func TestQuotationLifecycle_SendAcceptConvert(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	cust := mustSeedCustomer(t, srv, companyID)
	q := mustCreateQuotationOn(t, srv.URL, companyID, cust.ID, 0)

	sendResp := postJSON(t, srv.URL+"/quotations/"+q.ID+"/send", nil)
	requireStatus(t, sendResp, http.StatusOK)
	var sent quotationView
	sendResp.decode(t, &sent)
	if sent.Status != "SENT" {
		t.Fatalf("status = %q, want SENT", sent.Status)
	}

	// Can't accept from DRAFT again / send twice.
	requireStatus(t, postJSON(t, srv.URL+"/quotations/"+q.ID+"/send", nil), http.StatusConflict)

	acceptResp := postJSON(t, srv.URL+"/quotations/"+q.ID+"/accept", nil)
	requireStatus(t, acceptResp, http.StatusOK)
	var accepted quotationView
	acceptResp.decode(t, &accepted)
	if accepted.Status != "ACCEPTED" {
		t.Fatalf("status = %q, want ACCEPTED", accepted.Status)
	}

	convertResp := postJSON(t, srv.URL+"/quotations/"+q.ID+"/convert", nil)
	requireStatus(t, convertResp, http.StatusCreated)
	var so struct {
		ID          string  `json:"id"`
		Status      string  `json:"status"`
		QuotationID *string `json:"quotation_id"`
		TotalAmount float64 `json:"total_amount"`
	}
	convertResp.decode(t, &so)
	if so.Status != "DRAFT" {
		t.Errorf("converted sales order status = %q, want DRAFT", so.Status)
	}
	if so.QuotationID == nil || *so.QuotationID != q.ID {
		t.Errorf("converted sales order quotation_id = %v, want %q", so.QuotationID, q.ID)
	}
	if so.TotalAmount != accepted.TotalAmount {
		t.Errorf("converted sales order total = %.2f, want quotation total %.2f", so.TotalAmount, accepted.TotalAmount)
	}

	// Quotation must now be CONVERTED and can't be converted again.
	getResp := getJSON(t, srv.URL+"/quotations/"+q.ID)
	requireStatus(t, getResp, http.StatusOK)
	var reloaded quotationView
	getResp.decode(t, &reloaded)
	if reloaded.Status != "CONVERTED" {
		t.Errorf("status = %q, want CONVERTED", reloaded.Status)
	}
	requireStatus(t, postJSON(t, srv.URL+"/quotations/"+q.ID+"/convert", nil), http.StatusConflict)
}

func TestRejectQuotation(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	cust := mustSeedCustomer(t, srv, companyID)
	q := mustCreateQuotationOn(t, srv.URL, companyID, cust.ID, 0)

	requireStatus(t, postJSON(t, srv.URL+"/quotations/"+q.ID+"/send", nil), http.StatusOK)
	rejectResp := postJSON(t, srv.URL+"/quotations/"+q.ID+"/reject", nil)
	requireStatus(t, rejectResp, http.StatusOK)
	var rejected quotationView
	rejectResp.decode(t, &rejected)
	if rejected.Status != "REJECTED" {
		t.Errorf("status = %q, want REJECTED", rejected.Status)
	}

	// A REJECTED quotation can't be converted.
	requireStatus(t, postJSON(t, srv.URL+"/quotations/"+q.ID+"/convert", nil), http.StatusConflict)
}

func TestConvertQuotation_RequiresAccepted(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	cust := mustSeedCustomer(t, srv, companyID)
	q := mustCreateQuotationOn(t, srv.URL, companyID, cust.ID, 0) // still DRAFT

	resp := postJSON(t, srv.URL+"/quotations/"+q.ID+"/convert", nil)
	requireStatus(t, resp, http.StatusConflict)
}

func TestGetQuotation_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/quotations/"+uuid.NewString())
	requireStatus(t, resp, http.StatusNotFound)
}
