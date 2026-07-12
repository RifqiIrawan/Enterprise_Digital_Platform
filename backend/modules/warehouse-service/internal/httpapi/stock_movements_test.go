package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCreateManualStockMovement_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)

	cases := map[string]map[string]any{
		"missing warehouse_id": {"company_id": companyID, "product_id": p.ID, "movement_type": "IN", "quantity": 1},
		"missing product_id":   {"company_id": companyID, "warehouse_id": wh.ID, "movement_type": "IN", "quantity": 1},
		"invalid movement_type": {
			"company_id": companyID, "warehouse_id": wh.ID, "product_id": p.ID, "movement_type": "ADJUST", "quantity": 1,
		},
		"zero quantity": {
			"company_id": companyID, "warehouse_id": wh.ID, "product_id": p.ID, "movement_type": "IN", "quantity": 0,
		},
		"negative quantity": {
			"company_id": companyID, "warehouse_id": wh.ID, "product_id": p.ID, "movement_type": "IN", "quantity": -5,
		},
		"bad movement_date": {
			"company_id": companyID, "warehouse_id": wh.ID, "product_id": p.ID, "movement_type": "IN", "quantity": 1,
			"movement_date": "01-07-2026",
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/stock-movements", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateManualStockMovement_InAndOutAdjustBalance(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)

	requireStatus(t, postJSON(t, srv.URL+"/stock-movements", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "product_id": p.ID, "movement_type": "IN", "quantity": 50,
	}), http.StatusCreated)
	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, p.ID); got != 50 {
		t.Fatalf("quantity after IN 50 = %.2f, want 50.00", got)
	}

	requireStatus(t, postJSON(t, srv.URL+"/stock-movements", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "product_id": p.ID, "movement_type": "OUT", "quantity": 20,
	}), http.StatusCreated)
	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, p.ID); got != 30 {
		t.Fatalf("quantity after OUT 20 = %.2f, want 30.00", got)
	}
}

func TestPostStockMovementBatch_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)

	cases := map[string]map[string]any{
		"missing warehouse_id": {
			"company_id": companyID, "movement_type": "IN", "reference_type": "MANUAL",
			"lines": []map[string]any{{"product_name": "Item A", "quantity": 1}},
		},
		"empty lines": {
			"company_id": companyID, "warehouse_id": wh.ID, "movement_type": "IN", "reference_type": "MANUAL",
			"lines": []map[string]any{},
		},
		"invalid movement_type": {
			"company_id": companyID, "warehouse_id": wh.ID, "movement_type": "ADJUST", "reference_type": "MANUAL",
			"lines": []map[string]any{{"product_name": "Item A", "quantity": 1}},
		},
		"invalid reference_type": {
			"company_id": companyID, "warehouse_id": wh.ID, "movement_type": "IN", "reference_type": "BOGUS",
			"lines": []map[string]any{{"product_name": "Item A", "quantity": 1}},
		},
		"line missing product identifier": {
			"company_id": companyID, "warehouse_id": wh.ID, "movement_type": "IN", "reference_type": "MANUAL",
			"lines": []map[string]any{{"quantity": 1}},
		},
		"line zero quantity": {
			"company_id": companyID, "warehouse_id": wh.ID, "movement_type": "IN", "reference_type": "MANUAL",
			"lines": []map[string]any{{"product_name": "Item A", "quantity": 0}},
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/stock-movements/batch", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

// TestPostStockMovementBatch_AutoCreatesProductByName exercises the path
// purchasing-service/sales-service actually use: they only know a free-text
// product_name (no product master of their own), so warehouse-service must
// match-or-create by (company_id, name) — see findOrCreateProduct.
func TestPostStockMovementBatch_AutoCreatesProductByName(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)
	referenceID := uuid.NewString()

	resp := postJSON(t, srv.URL+"/stock-movements/batch", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "movement_type": "IN",
		"reference_type": "PURCHASE_ORDER", "reference_id": referenceID, "notes": "Penerimaan PO-TEST",
		"lines": []map[string]any{{"product_name": "Bahan Baku Otomatis", "quantity": 15}},
	})
	requireStatus(t, resp, http.StatusCreated)

	var movements []struct {
		ProductID     string  `json:"product_id"`
		ProductSKU    string  `json:"product_sku"`
		ProductName   string  `json:"product_name"`
		ReferenceType string  `json:"reference_type"`
		ReferenceID   *string `json:"reference_id"`
	}
	resp.decode(t, &movements)
	if len(movements) != 1 {
		t.Fatalf("expected 1 movement, got %d", len(movements))
	}
	mv := movements[0]
	if mv.ProductName != "Bahan Baku Otomatis" {
		t.Errorf("product_name = %q, want %q", mv.ProductName, "Bahan Baku Otomatis")
	}
	if !strings.HasPrefix(mv.ProductSKU, "AUTO-") {
		t.Errorf("product_sku = %q, want AUTO- prefix (auto-generated)", mv.ProductSKU)
	}
	if mv.ReferenceType != "PURCHASE_ORDER" || mv.ReferenceID == nil || *mv.ReferenceID != referenceID {
		t.Errorf("reference = %s/%v, want PURCHASE_ORDER/%s", mv.ReferenceType, mv.ReferenceID, referenceID)
	}
	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, mv.ProductID); got != 15 {
		t.Errorf("quantity after batch IN 15 = %.2f, want 15.00", got)
	}

	// Posting a second batch with the SAME product_name must reuse the
	// auto-created product (matched by name), not create a duplicate.
	resp2 := postJSON(t, srv.URL+"/stock-movements/batch", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "movement_type": "IN",
		"reference_type": "PURCHASE_ORDER",
		"lines":          []map[string]any{{"product_name": "Bahan Baku Otomatis", "quantity": 5}},
	})
	requireStatus(t, resp2, http.StatusCreated)
	var movements2 []struct {
		ProductID string `json:"product_id"`
	}
	resp2.decode(t, &movements2)
	if movements2[0].ProductID != mv.ProductID {
		t.Errorf("second batch created a different product (%q) instead of reusing %q", movements2[0].ProductID, mv.ProductID)
	}
	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, mv.ProductID); got != 20 {
		t.Errorf("quantity after second batch IN 5 = %.2f, want 20.00 (15+5)", got)
	}
}

func TestPostStockMovementBatch_ByProductID(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/stock-movements/batch", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "movement_type": "OUT", "reference_type": "SALES_ORDER",
		"lines": []map[string]any{{"product_id": p.ID, "quantity": 3}},
	})
	// OUT below zero is still allowed by this handler (no negative-stock guard
	// in applyStockMovement) -- document actual behavior rather than assume.
	requireStatus(t, resp, http.StatusCreated)
	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, p.ID); got != -3 {
		t.Errorf("quantity after OUT 3 from zero balance = %.2f, want -3.00 (no negative-stock guard)", got)
	}
}

func TestPostStockMovementBatch_UnknownProductID(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/stock-movements/batch", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "movement_type": "IN", "reference_type": "MANUAL",
		"lines": []map[string]any{{"product_id": uuid.NewString(), "quantity": 1}},
	})
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestListStockMovements_FilteredByWarehouse(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	whA := mustSeedWarehouse(t, srv, companyID)
	whB := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)

	requireStatus(t, postJSON(t, srv.URL+"/stock-movements", map[string]any{
		"company_id": companyID, "warehouse_id": whA.ID, "product_id": p.ID, "movement_type": "IN", "quantity": 10,
	}), http.StatusCreated)
	requireStatus(t, postJSON(t, srv.URL+"/stock-movements", map[string]any{
		"company_id": companyID, "warehouse_id": whB.ID, "product_id": p.ID, "movement_type": "IN", "quantity": 10,
	}), http.StatusCreated)

	resp := getJSON(t, srv.URL+"/stock-movements?company_id="+companyID+"&warehouse_id="+whA.ID)
	requireStatus(t, resp, http.StatusOK)
	var movements []struct {
		WarehouseID string `json:"warehouse_id"`
	}
	resp.decode(t, &movements)
	if len(movements) != 1 || movements[0].WarehouseID != whA.ID {
		t.Fatalf("expected exactly 1 movement scoped to whA, got %+v", movements)
	}
}

func TestListStockBalances_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/stock")
	requireStatus(t, resp, http.StatusBadRequest)
}
