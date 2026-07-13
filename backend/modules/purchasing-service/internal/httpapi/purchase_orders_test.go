package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type purchaseOrderView struct {
	ID             string  `json:"id"`
	Status         string  `json:"status"`
	PONumber       string  `json:"po_number"`
	SubtotalAmount float64 `json:"subtotal_amount"`
	TotalAmount    float64 `json:"total_amount"`
	InvoiceID      *string `json:"invoice_id"`
}

func mustCreatePurchaseOrderOn(t *testing.T, srvURL, companyID, supplierID string) purchaseOrderView {
	t.Helper()
	resp := postJSON(t, srvURL+"/purchase-orders", map[string]any{
		"company_id": companyID, "supplier_id": supplierID, "order_date": today(),
		"lines": []map[string]any{{"product_name": "Barang Uji", "quantity": 3, "unit_price": 200}},
	})
	requireStatus(t, resp, http.StatusCreated)
	var po purchaseOrderView
	resp.decode(t, &po)
	return po
}

func mustConfirmPurchaseOrder(t *testing.T, srvURL, poID string) purchaseOrderView {
	t.Helper()
	resp := postJSON(t, srvURL+"/purchase-orders/"+poID+"/confirm", nil)
	requireStatus(t, resp, http.StatusOK)
	var po purchaseOrderView
	resp.decode(t, &po)
	return po
}

func TestCreatePurchaseOrder_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	supplier := mustSeedSupplier(t, srv, companyID)

	cases := map[string]map[string]any{
		"missing supplier_id": {
			"company_id": companyID, "order_date": today(),
			"lines": []map[string]any{{"product_name": "Item A", "quantity": 1, "unit_price": 100}},
		},
		"empty lines": {
			"company_id": companyID, "supplier_id": supplier.ID, "order_date": today(), "lines": []map[string]any{},
		},
		"line missing product_name": {
			"company_id": companyID, "supplier_id": supplier.ID, "order_date": today(),
			"lines": []map[string]any{{"product_name": "", "quantity": 1, "unit_price": 100}},
		},
		"bad order_date format": {
			"company_id": companyID, "supplier_id": supplier.ID, "order_date": "01-07-2026",
			"lines": []map[string]any{{"product_name": "Item A", "quantity": 1, "unit_price": 100}},
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/purchase-orders", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreatePurchaseOrder_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	supplier := mustSeedSupplier(t, srv, companyID)

	po := mustCreatePurchaseOrderOn(t, srv.URL, companyID, supplier.ID)
	if po.Status != "DRAFT" {
		t.Errorf("status = %q, want DRAFT", po.Status)
	}
	if !strings.HasPrefix(po.PONumber, "PO-") {
		t.Errorf("po_number = %q, want PO- prefix", po.PONumber)
	}
	if po.TotalAmount != 600 {
		t.Errorf("total_amount = %.2f, want 600.00", po.TotalAmount)
	}
}

func TestConfirmPurchaseOrder_Lifecycle(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	supplier := mustSeedSupplier(t, srv, companyID)
	po := mustCreatePurchaseOrderOn(t, srv.URL, companyID, supplier.ID)

	confirmed := mustConfirmPurchaseOrder(t, srv.URL, po.ID)
	if confirmed.Status != "CONFIRMED" {
		t.Fatalf("status = %q, want CONFIRMED", confirmed.Status)
	}

	// Confirming twice must fail.
	requireStatus(t, postJSON(t, srv.URL+"/purchase-orders/"+po.ID+"/confirm", nil), http.StatusConflict)
}

func TestReceivePurchaseOrder_ValidationAndWrongStatus(t *testing.T) {
	srv, _, _ := newServerWithStubs(t, false, false)
	companyID := newCompanyID(t)
	supplier := mustSeedSupplier(t, srv, companyID)
	po := mustCreatePurchaseOrderOn(t, srv.URL, companyID, supplier.ID) // still DRAFT

	missingWarehouse := postJSON(t, srv.URL+"/purchase-orders/"+po.ID+"/receive", map[string]any{})
	requireStatus(t, missingWarehouse, http.StatusBadRequest)

	wrongStatus := postJSON(t, srv.URL+"/purchase-orders/"+po.ID+"/receive", map[string]any{"warehouse_id": uuid.NewString()})
	requireStatus(t, wrongStatus, http.StatusConflict)
}

func TestReceivePurchaseOrder_Success(t *testing.T) {
	srv, _, warehouseCalls := newServerWithStubs(t, false, false)
	companyID := newCompanyID(t)
	supplier := mustSeedSupplier(t, srv, companyID)
	po := mustCreatePurchaseOrderOn(t, srv.URL, companyID, supplier.ID)
	mustConfirmPurchaseOrder(t, srv.URL, po.ID)

	warehouseID := uuid.NewString()
	resp := postJSON(t, srv.URL+"/purchase-orders/"+po.ID+"/receive", map[string]any{"warehouse_id": warehouseID})
	requireStatus(t, resp, http.StatusOK)
	var received purchaseOrderView
	resp.decode(t, &received)
	if received.Status != "RECEIVED" {
		t.Errorf("status = %q, want RECEIVED", received.Status)
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
	if sent.MovementType != "IN" {
		t.Errorf("sent movement_type = %q, want IN", sent.MovementType)
	}
	if sent.ReferenceType != "PURCHASE_ORDER" || sent.ReferenceID != po.ID {
		t.Errorf("sent reference = %s/%s, want PURCHASE_ORDER/%s", sent.ReferenceType, sent.ReferenceID, po.ID)
	}
	if len(sent.Lines) != 1 || sent.Lines[0].ProductName != "Barang Uji" || sent.Lines[0].Quantity != 3 {
		t.Errorf("unexpected lines sent to warehouse-service: %+v", sent.Lines)
	}
}

func TestReceivePurchaseOrder_WarehouseFailureLeavesConfirmed(t *testing.T) {
	srv, _, warehouseCalls := newServerWithStubs(t, false, true) // warehouse stub always fails
	companyID := newCompanyID(t)
	supplier := mustSeedSupplier(t, srv, companyID)
	po := mustCreatePurchaseOrderOn(t, srv.URL, companyID, supplier.ID)
	mustConfirmPurchaseOrder(t, srv.URL, po.ID)

	resp := postJSON(t, srv.URL+"/purchase-orders/"+po.ID+"/receive", map[string]any{"warehouse_id": uuid.NewString()})
	requireStatus(t, resp, http.StatusBadGateway)
	if len(*warehouseCalls) != 1 {
		t.Fatalf("expected exactly 1 attempted warehouse-service call, got %d", len(*warehouseCalls))
	}

	getResp := getJSON(t, srv.URL+"/purchase-orders/"+po.ID)
	requireStatus(t, getResp, http.StatusOK)
	var reloaded purchaseOrderView
	getResp.decode(t, &reloaded)
	if reloaded.Status != "CONFIRMED" {
		t.Errorf("status = %q, want CONFIRMED after warehouse-service failure", reloaded.Status)
	}
}

func TestInvoicePurchaseOrder_ValidationAndWrongStatus(t *testing.T) {
	srv, _, _ := newServerWithStubs(t, false, false)
	companyID := newCompanyID(t)
	supplier := mustSeedSupplier(t, srv, companyID)
	po := mustCreatePurchaseOrderOn(t, srv.URL, companyID, supplier.ID) // still DRAFT

	missingFields := postJSON(t, srv.URL+"/purchase-orders/"+po.ID+"/invoice", map[string]any{})
	requireStatus(t, missingFields, http.StatusBadRequest)

	wrongStatus := postJSON(t, srv.URL+"/purchase-orders/"+po.ID+"/invoice", map[string]any{
		"expense_account_id": uuid.NewString(), "control_account_id": uuid.NewString(),
	})
	requireStatus(t, wrongStatus, http.StatusConflict)
}

func TestInvoicePurchaseOrder_SuccessFromConfirmed(t *testing.T) {
	srv, financeCalls, _ := newServerWithStubs(t, false, false)
	companyID := newCompanyID(t)
	supplier := mustSeedSupplier(t, srv, companyID)
	po := mustCreatePurchaseOrderOn(t, srv.URL, companyID, supplier.ID)
	mustConfirmPurchaseOrder(t, srv.URL, po.ID)

	expenseAcc := uuid.NewString()
	controlAcc := uuid.NewString()
	taxAcc := uuid.NewString()

	resp := postJSON(t, srv.URL+"/purchase-orders/"+po.ID+"/invoice", map[string]any{
		"expense_account_id": expenseAcc, "control_account_id": controlAcc, "tax_account_id": taxAcc,
	})
	requireStatus(t, resp, http.StatusOK)
	var invoiced purchaseOrderView
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
	if sent.InvoiceType != "AP" {
		t.Errorf("sent invoice_type = %q, want AP", sent.InvoiceType)
	}
	if sent.ControlAccountID != controlAcc {
		t.Errorf("sent control_account_id = %q, want %q", sent.ControlAccountID, controlAcc)
	}
	if sent.TaxAccountID == nil || *sent.TaxAccountID != taxAcc {
		t.Errorf("sent tax_account_id = %v, want %q", sent.TaxAccountID, taxAcc)
	}
	if len(sent.Lines) != 1 || sent.Lines[0].AccountID != expenseAcc || sent.Lines[0].Quantity != 3 {
		t.Errorf("unexpected lines sent to finance-service: %+v", sent.Lines)
	}

	// Invoicing an already-invoiced purchase order must be rejected.
	requireStatus(t, postJSON(t, srv.URL+"/purchase-orders/"+po.ID+"/invoice", map[string]any{
		"expense_account_id": expenseAcc, "control_account_id": controlAcc,
	}), http.StatusConflict)
}

func TestInvoicePurchaseOrder_SuccessFromReceived(t *testing.T) {
	srv, financeCalls, _ := newServerWithStubs(t, false, false)
	companyID := newCompanyID(t)
	supplier := mustSeedSupplier(t, srv, companyID)
	po := mustCreatePurchaseOrderOn(t, srv.URL, companyID, supplier.ID)
	mustConfirmPurchaseOrder(t, srv.URL, po.ID)
	requireStatus(t, postJSON(t, srv.URL+"/purchase-orders/"+po.ID+"/receive", map[string]any{
		"warehouse_id": uuid.NewString(),
	}), http.StatusOK)

	resp := postJSON(t, srv.URL+"/purchase-orders/"+po.ID+"/invoice", map[string]any{
		"expense_account_id": uuid.NewString(), "control_account_id": uuid.NewString(),
	})
	requireStatus(t, resp, http.StatusOK)
	var invoiced purchaseOrderView
	resp.decode(t, &invoiced)
	if invoiced.Status != "INVOICED" {
		t.Errorf("status = %q, want INVOICED (from RECEIVED)", invoiced.Status)
	}
	if len(*financeCalls) != 2 {
		t.Fatalf("expected 2 calls to finance-service, got %d", len(*financeCalls))
	}
}

func TestInvoicePurchaseOrder_FinanceFailureLeavesStatus(t *testing.T) {
	srv, financeCalls, _ := newServerWithStubs(t, true, false) // finance stub always fails
	companyID := newCompanyID(t)
	supplier := mustSeedSupplier(t, srv, companyID)
	po := mustCreatePurchaseOrderOn(t, srv.URL, companyID, supplier.ID)
	mustConfirmPurchaseOrder(t, srv.URL, po.ID)

	resp := postJSON(t, srv.URL+"/purchase-orders/"+po.ID+"/invoice", map[string]any{
		"expense_account_id": uuid.NewString(), "control_account_id": uuid.NewString(),
	})
	requireStatus(t, resp, http.StatusBadGateway)
	if len(*financeCalls) != 1 {
		t.Fatalf("expected exactly 1 attempted finance-service call, got %d", len(*financeCalls))
	}

	getResp := getJSON(t, srv.URL+"/purchase-orders/"+po.ID)
	requireStatus(t, getResp, http.StatusOK)
	var reloaded purchaseOrderView
	getResp.decode(t, &reloaded)
	if reloaded.Status != "CONFIRMED" {
		t.Errorf("status = %q, want CONFIRMED after finance-service failure", reloaded.Status)
	}
	if reloaded.InvoiceID != nil {
		t.Errorf("invoice_id = %v, want nil after finance-service failure", reloaded.InvoiceID)
	}
}

func TestGetPurchaseOrder_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/purchase-orders/"+uuid.NewString())
	requireStatus(t, resp, http.StatusNotFound)
}

func TestListPurchaseOrders_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/purchase-orders")
	requireStatus(t, resp, http.StatusBadRequest)
}

// TestListPurchaseOrders_FilteredByBranch confirms branch_id filtering is
// NULL-inclusive (see TestListSuppliers_FilteredByBranch for rationale).
func TestListPurchaseOrders_FilteredByBranch(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	supplier := mustSeedSupplier(t, srv, companyID)
	branchA := uuid.NewString()
	branchB := uuid.NewString()

	mkOrder := func(branchID *string) {
		requireStatus(t, postJSON(t, srv.URL+"/purchase-orders", map[string]any{
			"company_id": companyID, "branch_id": branchID, "supplier_id": supplier.ID, "order_date": today(),
			"lines": []map[string]any{{"product_name": "Barang Uji", "quantity": 1, "unit_price": 100}},
		}), http.StatusCreated)
	}
	mkOrder(&branchA)
	mkOrder(nil)
	mkOrder(&branchB)

	resp := getJSON(t, srv.URL+"/purchase-orders?company_id="+companyID+"&branch_id="+branchA)
	requireStatus(t, resp, http.StatusOK)
	var orders []struct {
		BranchID *string `json:"branch_id"`
	}
	resp.decode(t, &orders)
	if len(orders) != 2 {
		t.Fatalf("expected 2 purchase orders (branchA + NULL), got %d: %+v", len(orders), orders)
	}
	for _, o := range orders {
		if o.BranchID != nil && *o.BranchID == branchB {
			t.Errorf("branchB purchase order leaked into branchA-filtered results: %+v", orders)
		}
	}
}
