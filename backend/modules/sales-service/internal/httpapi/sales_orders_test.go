package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type salesOrderView struct {
	ID             string  `json:"id"`
	Status         string  `json:"status"`
	SONumber       string  `json:"so_number"`
	SubtotalAmount float64 `json:"subtotal_amount"`
	TotalAmount    float64 `json:"total_amount"`
	InvoiceID      *string `json:"invoice_id"`
}

func mustCreateSalesOrderOn(t *testing.T, srvURL, companyID, customerID string) salesOrderView {
	t.Helper()
	resp := postJSON(t, srvURL+"/sales-orders", map[string]any{
		"company_id": companyID, "customer_id": customerID, "order_date": today(),
		"lines": []map[string]any{{"product_name": "Barang Uji", "quantity": 3, "unit_price": 200}},
	})
	requireStatus(t, resp, http.StatusCreated)
	var so salesOrderView
	resp.decode(t, &so)
	return so
}

func mustConfirmSalesOrder(t *testing.T, srvURL, soID string) salesOrderView {
	t.Helper()
	resp := postJSON(t, srvURL+"/sales-orders/"+soID+"/confirm", nil)
	requireStatus(t, resp, http.StatusOK)
	var so salesOrderView
	resp.decode(t, &so)
	return so
}

func TestCreateSalesOrder_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	cust := mustSeedCustomer(t, srv, companyID)

	cases := map[string]map[string]any{
		"missing customer_id": {
			"company_id": companyID, "order_date": today(),
			"lines": []map[string]any{{"product_name": "Item A", "quantity": 1, "unit_price": 100}},
		},
		"empty lines": {
			"company_id": companyID, "customer_id": cust.ID, "order_date": today(), "lines": []map[string]any{},
		},
		"line missing product_name": {
			"company_id": companyID, "customer_id": cust.ID, "order_date": today(),
			"lines": []map[string]any{{"product_name": "", "quantity": 1, "unit_price": 100}},
		},
		"bad order_date format": {
			"company_id": companyID, "customer_id": cust.ID, "order_date": "01-07-2026",
			"lines": []map[string]any{{"product_name": "Item A", "quantity": 1, "unit_price": 100}},
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/sales-orders", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateSalesOrder_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	cust := mustSeedCustomer(t, srv, companyID)

	so := mustCreateSalesOrderOn(t, srv.URL, companyID, cust.ID)
	if so.Status != "DRAFT" {
		t.Errorf("status = %q, want DRAFT", so.Status)
	}
	if !strings.HasPrefix(so.SONumber, "SO-") {
		t.Errorf("so_number = %q, want SO- prefix", so.SONumber)
	}
	if so.TotalAmount != 600 {
		t.Errorf("total_amount = %.2f, want 600.00", so.TotalAmount)
	}
}

func TestConfirmSalesOrder_Lifecycle(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	cust := mustSeedCustomer(t, srv, companyID)
	so := mustCreateSalesOrderOn(t, srv.URL, companyID, cust.ID)

	confirmed := mustConfirmSalesOrder(t, srv.URL, so.ID)
	if confirmed.Status != "CONFIRMED" {
		t.Fatalf("status = %q, want CONFIRMED", confirmed.Status)
	}

	// Confirming twice must fail.
	requireStatus(t, postJSON(t, srv.URL+"/sales-orders/"+so.ID+"/confirm", nil), http.StatusConflict)
}

func TestFulfillSalesOrder_ValidationAndWrongStatus(t *testing.T) {
	srv, _, _ := newServerWithStubs(t, false, false)
	companyID := newCompanyID(t)
	cust := mustSeedCustomer(t, srv, companyID)
	so := mustCreateSalesOrderOn(t, srv.URL, companyID, cust.ID) // still DRAFT

	missingWarehouse := postJSON(t, srv.URL+"/sales-orders/"+so.ID+"/fulfill", map[string]any{})
	requireStatus(t, missingWarehouse, http.StatusBadRequest)

	wrongStatus := postJSON(t, srv.URL+"/sales-orders/"+so.ID+"/fulfill", map[string]any{"warehouse_id": uuid.NewString()})
	requireStatus(t, wrongStatus, http.StatusConflict)
}

func TestFulfillSalesOrder_Success(t *testing.T) {
	srv, _, warehouseCalls := newServerWithStubs(t, false, false)
	companyID := newCompanyID(t)
	cust := mustSeedCustomer(t, srv, companyID)
	so := mustCreateSalesOrderOn(t, srv.URL, companyID, cust.ID)
	mustConfirmSalesOrder(t, srv.URL, so.ID)

	warehouseID := uuid.NewString()
	resp := postJSON(t, srv.URL+"/sales-orders/"+so.ID+"/fulfill", map[string]any{"warehouse_id": warehouseID})
	requireStatus(t, resp, http.StatusOK)
	var fulfilled salesOrderView
	resp.decode(t, &fulfilled)
	if fulfilled.Status != "FULFILLED" {
		t.Errorf("status = %q, want FULFILLED", fulfilled.Status)
	}

	if len(*warehouseCalls) != 1 {
		t.Fatalf("expected 1 call to warehouse-service, got %d", len(*warehouseCalls))
	}
	var sent struct {
		CompanyID     string `json:"company_id"`
		WarehouseID   string `json:"warehouse_id"`
		MovementType  string `json:"movement_type"`
		ReferenceType string `json:"reference_type"`
		ReferenceID   string `json:"reference_id"`
		Lines         []struct {
			ProductName string  `json:"product_name"`
			Quantity    float64 `json:"quantity"`
		} `json:"lines"`
	}
	if err := json.Unmarshal((*warehouseCalls)[0].body, &sent); err != nil {
		t.Fatalf("decode stock movement batch sent to warehouse-service: %v", err)
	}
	if sent.CompanyID != companyID {
		t.Errorf("sent company_id = %q, want %q", sent.CompanyID, companyID)
	}
	if sent.WarehouseID != warehouseID {
		t.Errorf("sent warehouse_id = %q, want %q", sent.WarehouseID, warehouseID)
	}
	if sent.MovementType != "OUT" {
		t.Errorf("sent movement_type = %q, want OUT", sent.MovementType)
	}
	if sent.ReferenceType != "SALES_ORDER" || sent.ReferenceID != so.ID {
		t.Errorf("sent reference = %s/%s, want SALES_ORDER/%s", sent.ReferenceType, sent.ReferenceID, so.ID)
	}
	if len(sent.Lines) != 1 || sent.Lines[0].ProductName != "Barang Uji" || sent.Lines[0].Quantity != 3 {
		t.Errorf("unexpected lines sent to warehouse-service: %+v", sent.Lines)
	}
}

func TestFulfillSalesOrder_WarehouseFailureLeavesConfirmed(t *testing.T) {
	srv, _, warehouseCalls := newServerWithStubs(t, false, true) // warehouse stub always fails
	companyID := newCompanyID(t)
	cust := mustSeedCustomer(t, srv, companyID)
	so := mustCreateSalesOrderOn(t, srv.URL, companyID, cust.ID)
	mustConfirmSalesOrder(t, srv.URL, so.ID)

	resp := postJSON(t, srv.URL+"/sales-orders/"+so.ID+"/fulfill", map[string]any{"warehouse_id": uuid.NewString()})
	requireStatus(t, resp, http.StatusBadGateway)
	if len(*warehouseCalls) != 1 {
		t.Fatalf("expected exactly 1 attempted warehouse-service call, got %d", len(*warehouseCalls))
	}

	getResp := getJSON(t, srv.URL+"/sales-orders/"+so.ID)
	requireStatus(t, getResp, http.StatusOK)
	var reloaded salesOrderView
	getResp.decode(t, &reloaded)
	if reloaded.Status != "CONFIRMED" {
		t.Errorf("status = %q, want CONFIRMED after warehouse-service failure", reloaded.Status)
	}
}

func TestInvoiceSalesOrder_ValidationAndWrongStatus(t *testing.T) {
	srv, _, _ := newServerWithStubs(t, false, false)
	companyID := newCompanyID(t)
	cust := mustSeedCustomer(t, srv, companyID)
	so := mustCreateSalesOrderOn(t, srv.URL, companyID, cust.ID) // still DRAFT

	missingFields := postJSON(t, srv.URL+"/sales-orders/"+so.ID+"/invoice", map[string]any{})
	requireStatus(t, missingFields, http.StatusBadRequest)

	wrongStatus := postJSON(t, srv.URL+"/sales-orders/"+so.ID+"/invoice", map[string]any{
		"revenue_account_id": uuid.NewString(), "control_account_id": uuid.NewString(),
	})
	requireStatus(t, wrongStatus, http.StatusConflict)
}

func TestInvoiceSalesOrder_SuccessFromConfirmed(t *testing.T) {
	srv, financeCalls, _ := newServerWithStubs(t, false, false)
	companyID := newCompanyID(t)
	cust := mustSeedCustomer(t, srv, companyID)
	so := mustCreateSalesOrderOn(t, srv.URL, companyID, cust.ID)
	mustConfirmSalesOrder(t, srv.URL, so.ID)

	revenueAcc := uuid.NewString()
	controlAcc := uuid.NewString()
	taxAcc := uuid.NewString()

	resp := postJSON(t, srv.URL+"/sales-orders/"+so.ID+"/invoice", map[string]any{
		"revenue_account_id": revenueAcc, "control_account_id": controlAcc, "tax_account_id": taxAcc,
	})
	requireStatus(t, resp, http.StatusOK)
	var invoiced salesOrderView
	resp.decode(t, &invoiced)
	if invoiced.Status != "INVOICED" {
		t.Errorf("status = %q, want INVOICED", invoiced.Status)
	}
	if invoiced.InvoiceID == nil {
		t.Fatal("expected invoice_id to be set")
	}

	if len(*financeCalls) != 2 {
		t.Fatalf("expected 2 calls to finance-service (create + post), got %d", len(*financeCalls))
	}
	var sent struct {
		CompanyID        string  `json:"company_id"`
		InvoiceType      string  `json:"invoice_type"`
		PartnerName      string  `json:"partner_name"`
		ControlAccountID string  `json:"control_account_id"`
		TaxAccountID     *string `json:"tax_account_id"`
		Lines            []struct {
			AccountID string  `json:"account_id"`
			Quantity  float64 `json:"quantity"`
			UnitPrice float64 `json:"unit_price"`
		} `json:"lines"`
	}
	if err := json.Unmarshal((*financeCalls)[0].body, &sent); err != nil {
		t.Fatalf("decode invoice sent to finance-service: %v", err)
	}
	if sent.CompanyID != companyID {
		t.Errorf("sent company_id = %q, want %q", sent.CompanyID, companyID)
	}
	if sent.InvoiceType != "AR" {
		t.Errorf("sent invoice_type = %q, want AR", sent.InvoiceType)
	}
	if sent.ControlAccountID != controlAcc {
		t.Errorf("sent control_account_id = %q, want %q", sent.ControlAccountID, controlAcc)
	}
	if sent.TaxAccountID == nil || *sent.TaxAccountID != taxAcc {
		t.Errorf("sent tax_account_id = %v, want %q", sent.TaxAccountID, taxAcc)
	}
	if len(sent.Lines) != 1 || sent.Lines[0].AccountID != revenueAcc || sent.Lines[0].Quantity != 3 {
		t.Errorf("unexpected lines sent to finance-service: %+v", sent.Lines)
	}

	// Invoicing an already-invoiced sales order must be rejected.
	requireStatus(t, postJSON(t, srv.URL+"/sales-orders/"+so.ID+"/invoice", map[string]any{
		"revenue_account_id": revenueAcc, "control_account_id": controlAcc,
	}), http.StatusConflict)
}

func TestInvoiceSalesOrder_SuccessFromFulfilled(t *testing.T) {
	srv, financeCalls, _ := newServerWithStubs(t, false, false)
	companyID := newCompanyID(t)
	cust := mustSeedCustomer(t, srv, companyID)
	so := mustCreateSalesOrderOn(t, srv.URL, companyID, cust.ID)
	mustConfirmSalesOrder(t, srv.URL, so.ID)
	requireStatus(t, postJSON(t, srv.URL+"/sales-orders/"+so.ID+"/fulfill", map[string]any{
		"warehouse_id": uuid.NewString(),
	}), http.StatusOK)

	resp := postJSON(t, srv.URL+"/sales-orders/"+so.ID+"/invoice", map[string]any{
		"revenue_account_id": uuid.NewString(), "control_account_id": uuid.NewString(),
	})
	requireStatus(t, resp, http.StatusOK)
	var invoiced salesOrderView
	resp.decode(t, &invoiced)
	if invoiced.Status != "INVOICED" {
		t.Errorf("status = %q, want INVOICED (from FULFILLED)", invoiced.Status)
	}
	if len(*financeCalls) != 2 {
		t.Fatalf("expected 2 calls to finance-service, got %d", len(*financeCalls))
	}
}

func TestInvoiceSalesOrder_FinanceFailureLeavesStatus(t *testing.T) {
	srv, financeCalls, _ := newServerWithStubs(t, true, false) // finance stub always fails
	companyID := newCompanyID(t)
	cust := mustSeedCustomer(t, srv, companyID)
	so := mustCreateSalesOrderOn(t, srv.URL, companyID, cust.ID)
	mustConfirmSalesOrder(t, srv.URL, so.ID)

	resp := postJSON(t, srv.URL+"/sales-orders/"+so.ID+"/invoice", map[string]any{
		"revenue_account_id": uuid.NewString(), "control_account_id": uuid.NewString(),
	})
	requireStatus(t, resp, http.StatusBadGateway)
	if len(*financeCalls) != 1 {
		t.Fatalf("expected exactly 1 attempted finance-service call, got %d", len(*financeCalls))
	}

	getResp := getJSON(t, srv.URL+"/sales-orders/"+so.ID)
	requireStatus(t, getResp, http.StatusOK)
	var reloaded salesOrderView
	getResp.decode(t, &reloaded)
	if reloaded.Status != "CONFIRMED" {
		t.Errorf("status = %q, want CONFIRMED after finance-service failure", reloaded.Status)
	}
	if reloaded.InvoiceID != nil {
		t.Errorf("invoice_id = %v, want nil after finance-service failure", reloaded.InvoiceID)
	}
}

func TestGetSalesOrder_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/sales-orders/"+uuid.NewString())
	requireStatus(t, resp, http.StatusNotFound)
}

func TestListSalesOrders_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/sales-orders")
	requireStatus(t, resp, http.StatusBadRequest)
}
