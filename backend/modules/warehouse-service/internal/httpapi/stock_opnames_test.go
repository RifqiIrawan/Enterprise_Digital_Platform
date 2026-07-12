package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestCreateStockOpname_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)

	cases := map[string]map[string]any{
		"missing warehouse_id": {
			"company_id": companyID, "opname_date": today(),
			"lines": []map[string]any{{"product_id": p.ID, "counted_quantity": 5}},
		},
		"empty lines": {
			"company_id": companyID, "warehouse_id": wh.ID, "opname_date": today(), "lines": []map[string]any{},
		},
		"bad opname_date": {
			"company_id": companyID, "warehouse_id": wh.ID, "opname_date": "01-07-2026",
			"lines": []map[string]any{{"product_id": p.ID, "counted_quantity": 5}},
		},
		"line missing product_id": {
			"company_id": companyID, "warehouse_id": wh.ID, "opname_date": today(),
			"lines": []map[string]any{{"counted_quantity": 5}},
		},
		"line negative counted_quantity": {
			"company_id": companyID, "warehouse_id": wh.ID, "opname_date": today(),
			"lines": []map[string]any{{"product_id": p.ID, "counted_quantity": -1}},
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/stock-opnames", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

type stockOpnameLineView struct {
	ProductID       string  `json:"product_id"`
	SystemQuantity  float64 `json:"system_quantity"`
	CountedQuantity float64 `json:"counted_quantity"`
}

type stockOpnameView struct {
	ID           string                `json:"id"`
	Status       string                `json:"status"`
	OpnameNumber string                `json:"opname_number"`
	Lines        []stockOpnameLineView `json:"lines"`
}

func TestCreateStockOpname_SnapshotsSystemQuantity(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)
	pWithStock := mustSeedProduct(t, srv, companyID)
	pNoStock := mustSeedProduct(t, srv, companyID)
	mustGiveStock(t, srv, companyID, wh.ID, pWithStock.ID, 40)

	resp := postJSON(t, srv.URL+"/stock-opnames", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "opname_date": today(),
		"lines": []map[string]any{
			{"product_id": pWithStock.ID, "counted_quantity": 45},
			{"product_id": pNoStock.ID, "counted_quantity": 3},
		},
	})
	requireStatus(t, resp, http.StatusCreated)
	var o stockOpnameView
	resp.decode(t, &o)

	if o.Status != "DRAFT" {
		t.Errorf("status = %q, want DRAFT", o.Status)
	}
	if len(o.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(o.Lines))
	}
	for _, l := range o.Lines {
		switch l.ProductID {
		case pWithStock.ID:
			if l.SystemQuantity != 40 {
				t.Errorf("system_quantity for pre-stocked product = %.2f, want 40.00 (snapshot of existing balance)", l.SystemQuantity)
			}
			if l.CountedQuantity != 45 {
				t.Errorf("counted_quantity = %.2f, want 45.00", l.CountedQuantity)
			}
		case pNoStock.ID:
			if l.SystemQuantity != 0 {
				t.Errorf("system_quantity for never-moved product = %.2f, want 0.00 (no stock_balances row yet)", l.SystemQuantity)
			}
		}
	}

	// Posting must not have touched the balance yet (that only happens on POST).
	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, pWithStock.ID); got != 40 {
		t.Errorf("quantity before posting opname = %.2f, want unchanged 40.00", got)
	}
}

func TestPostStockOpname_AppliesAdjustmentAndSkipsZeroDiff(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)
	pIncrease := mustSeedProduct(t, srv, companyID)
	pDecrease := mustSeedProduct(t, srv, companyID)
	pUnchanged := mustSeedProduct(t, srv, companyID)
	mustGiveStock(t, srv, companyID, wh.ID, pIncrease.ID, 40)
	mustGiveStock(t, srv, companyID, wh.ID, pDecrease.ID, 40)
	mustGiveStock(t, srv, companyID, wh.ID, pUnchanged.ID, 40)

	beforeMovements := stockMovementCount(t, srv, companyID, wh.ID)

	createResp := postJSON(t, srv.URL+"/stock-opnames", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "opname_date": today(),
		"lines": []map[string]any{
			{"product_id": pIncrease.ID, "counted_quantity": 45},  // +5
			{"product_id": pDecrease.ID, "counted_quantity": 30},  // -10
			{"product_id": pUnchanged.ID, "counted_quantity": 40}, // no diff
		},
	})
	requireStatus(t, createResp, http.StatusCreated)
	var o stockOpnameView
	createResp.decode(t, &o)

	postResp := postJSON(t, srv.URL+"/stock-opnames/"+o.ID+"/post", nil)
	requireStatus(t, postResp, http.StatusOK)
	var posted stockOpnameView
	postResp.decode(t, &posted)
	if posted.Status != "POSTED" {
		t.Fatalf("status = %q, want POSTED", posted.Status)
	}

	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, pIncrease.ID); got != 45 {
		t.Errorf("pIncrease quantity after post = %.2f, want 45.00", got)
	}
	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, pDecrease.ID); got != 30 {
		t.Errorf("pDecrease quantity after post = %.2f, want 30.00", got)
	}
	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, pUnchanged.ID); got != 40 {
		t.Errorf("pUnchanged quantity after post = %.2f, want unchanged 40.00", got)
	}

	// Only 2 new movements (increase + decrease) should have been recorded;
	// the zero-diff line must be skipped entirely (see postStockOpname doc-comment).
	afterMovements := stockMovementCount(t, srv, companyID, wh.ID)
	if afterMovements-beforeMovements != 2 {
		t.Errorf("new movements recorded = %d, want 2 (zero-diff line must be skipped)", afterMovements-beforeMovements)
	}

	// Posting an already-POSTED opname must be rejected.
	requireStatus(t, postJSON(t, srv.URL+"/stock-opnames/"+o.ID+"/post", nil), http.StatusConflict)
}

func stockMovementCount(t *testing.T, srv *httptest.Server, companyID, warehouseID string) int {
	t.Helper()
	resp := getJSON(t, srv.URL+"/stock-movements?company_id="+companyID+"&warehouse_id="+warehouseID)
	requireStatus(t, resp, http.StatusOK)
	var movements []struct{}
	resp.decode(t, &movements)
	return len(movements)
}

func TestPostStockOpname_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := postJSON(t, srv.URL+"/stock-opnames/"+uuid.NewString()+"/post", nil)
	requireStatus(t, resp, http.StatusNotFound)
}

func TestGetStockOpname_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/stock-opnames/"+uuid.NewString())
	requireStatus(t, resp, http.StatusNotFound)
}

func TestListStockOpnames_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/stock-opnames")
	requireStatus(t, resp, http.StatusBadRequest)
}
