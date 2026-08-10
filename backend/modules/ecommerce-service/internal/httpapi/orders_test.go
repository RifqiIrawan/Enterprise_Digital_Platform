package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateOrder_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	validLine := map[string]any{"product_id": uuid.NewString(), "product_sku": "SKU-001", "product_name": "Produk", "unit_price": 10000, "quantity": 1}

	cases := map[string]map[string]any{
		"missing company_id":    {"customer_name": "Budi", "lines": []map[string]any{validLine}},
		"missing customer_name": {"company_id": companyID, "lines": []map[string]any{validLine}},
		"no lines":              {"company_id": companyID, "customer_name": "Budi", "lines": []map[string]any{}},
		"line missing product_id": {"company_id": companyID, "customer_name": "Budi", "lines": []map[string]any{
			{"product_sku": "SKU-001", "product_name": "Produk", "unit_price": 10000, "quantity": 1},
		}},
		"line quantity zero": {"company_id": companyID, "customer_name": "Budi", "lines": []map[string]any{
			{"product_id": uuid.NewString(), "product_sku": "SKU-001", "product_name": "Produk", "unit_price": 10000, "quantity": 0},
		}},
		"line negative unit_price": {"company_id": companyID, "customer_name": "Budi", "lines": []map[string]any{
			{"product_id": uuid.NewString(), "product_sku": "SKU-001", "product_name": "Produk", "unit_price": -1, "quantity": 1},
		}},
		"invalid order_date": {"company_id": companyID, "customer_name": "Budi", "order_date": "not-a-date", "lines": []map[string]any{validLine}},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/orders", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateOrder_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/orders", map[string]any{
		"company_id":     companyID,
		"customer_name":  "Budi Santoso",
		"customer_email": "budi@example.com",
		"lines": []map[string]any{
			{"product_id": uuid.NewString(), "product_sku": "SKU-001", "product_name": "Produk A", "unit_price": 50000, "quantity": 2},
			{"product_id": uuid.NewString(), "product_sku": "SKU-002", "product_name": "Produk B", "unit_price": 25000, "quantity": 4},
		},
	})
	requireStatus(t, resp, http.StatusCreated)

	var o struct {
		CompanyID   string  `json:"company_id"`
		Status      string  `json:"status"`
		OrderNumber string  `json:"order_number"`
		TotalAmount float64 `json:"total_amount"`
		Items       []struct {
			LineTotal float64 `json:"line_total"`
		} `json:"items"`
	}
	resp.decode(t, &o)
	if o.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", o.CompanyID, companyID)
	}
	if o.Status != "PENDING" {
		t.Errorf("status = %q, want default PENDING", o.Status)
	}
	if o.OrderNumber == "" {
		t.Error("expected a non-empty auto-generated order_number")
	}
	// 50000*2 + 25000*4 = 100000 + 100000 = 200000
	if o.TotalAmount != 200000 {
		t.Errorf("total_amount = %v, want 200000", o.TotalAmount)
	}
	if len(o.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(o.Items))
	}
	if o.Items[0].LineTotal != 100000 || o.Items[1].LineTotal != 100000 {
		t.Errorf("unexpected line totals: %+v", o.Items)
	}
}

func TestGetOrder_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/orders/"+uuid.NewString())
	requireStatus(t, resp, http.StatusNotFound)
}

func TestGetOrder_IncludesItems(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	o := mustSeedOrder(t, srv, companyID)

	resp := getJSON(t, srv.URL+"/orders/"+o.ID)
	requireStatus(t, resp, http.StatusOK)
	var full struct {
		Items []struct {
			ProductSKU string `json:"product_sku"`
		} `json:"items"`
	}
	resp.decode(t, &full)
	if len(full.Items) != 1 || full.Items[0].ProductSKU != "SKU-001" {
		t.Fatalf("unexpected items: %+v", full.Items)
	}
}

func TestUpdateOrder_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	o := mustSeedOrder(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/orders/"+o.ID, map[string]any{
		"customer_name": "Budi Direvisi", "shipping_address": "Alamat Baru",
	}, "")
	requireStatus(t, resp, http.StatusOK)
	var updated struct {
		CustomerName    string `json:"customer_name"`
		ShippingAddress string `json:"shipping_address"`
	}
	resp.decode(t, &updated)
	if updated.CustomerName != "Budi Direvisi" || updated.ShippingAddress != "Alamat Baru" {
		t.Errorf("unexpected update result: %+v", updated)
	}
}

func TestUpdateOrder_RejectsAfterNotPending(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	o := mustSeedOrder(t, srv, companyID)
	mustPayOrder(t, srv.URL, o.ID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/orders/"+o.ID, map[string]any{"customer_name": "Budi"}, "")
	requireStatus(t, resp, http.StatusConflict)
}

func TestPayOrder_ThenRejectsSecondPay(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	o := mustSeedOrder(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/orders/"+o.ID+"/pay", nil)
	requireStatus(t, resp, http.StatusOK)
	var paid struct {
		Status string `json:"status"`
		PaidAt string `json:"paid_at"`
	}
	resp.decode(t, &paid)
	if paid.Status != "PAID" || paid.PaidAt == "" {
		t.Errorf("unexpected pay result: %+v", paid)
	}

	requireStatus(t, postJSON(t, srv.URL+"/orders/"+o.ID+"/pay", nil), http.StatusConflict)
}

func TestShipOrder_ValidationAndWrongStatus(t *testing.T) {
	srv, _ := newServerWithWarehouseStub(t, false)
	companyID := newCompanyID(t)
	o := mustSeedOrder(t, srv, companyID) // still PENDING

	missingWarehouse := postJSON(t, srv.URL+"/orders/"+o.ID+"/ship", map[string]any{})
	requireStatus(t, missingWarehouse, http.StatusBadRequest)

	wrongStatus := postJSON(t, srv.URL+"/orders/"+o.ID+"/ship", map[string]any{"warehouse_id": uuid.NewString()})
	requireStatus(t, wrongStatus, http.StatusConflict)
}

func TestShipOrder_Success(t *testing.T) {
	srv, warehouseCalls := newServerWithWarehouseStub(t, false)
	companyID := newCompanyID(t)
	o := mustSeedOrder(t, srv, companyID)
	mustPayOrder(t, srv.URL, o.ID)

	warehouseID := uuid.NewString()
	resp := postJSON(t, srv.URL+"/orders/"+o.ID+"/ship", map[string]any{"warehouse_id": warehouseID})
	requireStatus(t, resp, http.StatusOK)
	var shipped struct {
		Status    string `json:"status"`
		ShippedAt string `json:"shipped_at"`
	}
	resp.decode(t, &shipped)
	if shipped.Status != "SHIPPED" || shipped.ShippedAt == "" {
		t.Errorf("unexpected ship result: %+v", shipped)
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
			ProductID string  `json:"product_id"`
			Quantity  float64 `json:"quantity"`
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
	if sent.ReferenceType != "ECOMMERCE_ORDER" || sent.ReferenceID != o.ID {
		t.Errorf("sent reference = %s/%s, want ECOMMERCE_ORDER/%s", sent.ReferenceType, sent.ReferenceID, o.ID)
	}
	if len(sent.Lines) != 1 || sent.Lines[0].ProductID == "" || sent.Lines[0].Quantity != 2 {
		t.Errorf("unexpected lines sent to warehouse-service: %+v", sent.Lines)
	}
}

func TestShipOrder_WarehouseFailureLeavesPaid(t *testing.T) {
	srv, warehouseCalls := newServerWithWarehouseStub(t, true) // warehouse stub always fails
	companyID := newCompanyID(t)
	o := mustSeedOrder(t, srv, companyID)
	mustPayOrder(t, srv.URL, o.ID)

	resp := postJSON(t, srv.URL+"/orders/"+o.ID+"/ship", map[string]any{"warehouse_id": uuid.NewString()})
	requireStatus(t, resp, http.StatusBadGateway)
	if len(*warehouseCalls) != 1 {
		t.Fatalf("expected exactly 1 attempted warehouse-service call, got %d", len(*warehouseCalls))
	}

	getResp := getJSON(t, srv.URL+"/orders/"+o.ID)
	requireStatus(t, getResp, http.StatusOK)
	var reloaded struct {
		Status string `json:"status"`
	}
	getResp.decode(t, &reloaded)
	if reloaded.Status != "PAID" {
		t.Errorf("status = %q, want PAID after warehouse-service failure", reloaded.Status)
	}
}

func TestDeliverOrder_FullLifecycle(t *testing.T) {
	srv, _ := newServerWithWarehouseStub(t, false)
	companyID := newCompanyID(t)
	o := mustSeedOrder(t, srv, companyID)

	// Deliver before SHIPPED must fail.
	requireStatus(t, postJSON(t, srv.URL+"/orders/"+o.ID+"/deliver", nil), http.StatusConflict)

	mustPayOrder(t, srv.URL, o.ID)
	requireStatus(t, postJSON(t, srv.URL+"/orders/"+o.ID+"/ship", map[string]any{"warehouse_id": uuid.NewString()}), http.StatusOK)

	resp := postJSON(t, srv.URL+"/orders/"+o.ID+"/deliver", nil)
	requireStatus(t, resp, http.StatusOK)
	var delivered struct {
		Status      string `json:"status"`
		DeliveredAt string `json:"delivered_at"`
	}
	resp.decode(t, &delivered)
	if delivered.Status != "DELIVERED" || delivered.DeliveredAt == "" {
		t.Errorf("unexpected deliver result: %+v", delivered)
	}

	// Delivering again must fail -- DELIVERED is terminal.
	requireStatus(t, postJSON(t, srv.URL+"/orders/"+o.ID+"/deliver", nil), http.StatusConflict)
}

func TestCancelOrder_FromPending(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	o := mustSeedOrder(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/orders/"+o.ID+"/cancel", nil)
	requireStatus(t, resp, http.StatusOK)
	var cancelled struct {
		Status      string `json:"status"`
		CancelledAt string `json:"cancelled_at"`
	}
	resp.decode(t, &cancelled)
	if cancelled.Status != "CANCELLED" || cancelled.CancelledAt == "" {
		t.Errorf("unexpected cancel result: %+v", cancelled)
	}
}

func TestCancelOrder_FromPaid(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	o := mustSeedOrder(t, srv, companyID)
	mustPayOrder(t, srv.URL, o.ID)

	resp := postJSON(t, srv.URL+"/orders/"+o.ID+"/cancel", nil)
	requireStatus(t, resp, http.StatusOK)
}

func TestCancelOrder_RejectedAfterShipped(t *testing.T) {
	srv, _ := newServerWithWarehouseStub(t, false)
	companyID := newCompanyID(t)
	o := mustSeedOrder(t, srv, companyID)
	mustPayOrder(t, srv.URL, o.ID)
	requireStatus(t, postJSON(t, srv.URL+"/orders/"+o.ID+"/ship", map[string]any{"warehouse_id": uuid.NewString()}), http.StatusOK)

	requireStatus(t, postJSON(t, srv.URL+"/orders/"+o.ID+"/cancel", nil), http.StatusConflict)
}

func TestListOrders_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)
	mustSeedOrder(t, srv, companyA)
	mustSeedOrder(t, srv, companyB)

	resp := getJSON(t, srv.URL+"/orders?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 order scoped to companyA, got %+v", list)
	}
}

func TestListOrders_FilteredByStatus(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	o1 := mustSeedOrder(t, srv, companyID)
	mustSeedOrder(t, srv, companyID)
	mustPayOrder(t, srv.URL, o1.ID)

	resp := getJSON(t, srv.URL+"/orders?company_id="+companyID+"&status=PAID")
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		Status string `json:"status"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].Status != "PAID" {
		t.Fatalf("expected exactly 1 PAID order, got %+v", list)
	}
}

func TestListOrders_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/orders")
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestListOrders_BranchNullInclusive(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	branchID := uuid.NewString()

	// Order with no branch_id (NULL) must show up regardless of which
	// branch_id is queried -- same NULL-inclusive convention as every other
	// module's list endpoint.
	mustSeedOrder(t, srv, companyID)

	resp := getJSON(t, srv.URL+"/orders?company_id="+companyID+"&branch_id="+branchID)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		BranchID *string `json:"branch_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 {
		t.Fatalf("expected the NULL-branch order to still be included, got %+v", list)
	}
}
