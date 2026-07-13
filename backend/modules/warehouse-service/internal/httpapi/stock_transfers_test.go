package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// mustGiveStock seeds a warehouse+product with an initial IN movement so
// tests have a known non-zero starting balance to transfer/count from.
func mustGiveStock(t *testing.T, srv *httptest.Server, companyID, warehouseID, productID string, quantity float64) {
	t.Helper()
	requireStatus(t, postJSON(t, srv.URL+"/stock-movements", map[string]any{
		"company_id": companyID, "warehouse_id": warehouseID, "product_id": productID,
		"movement_type": "IN", "quantity": quantity,
	}), http.StatusCreated)
}

func TestCreateStockTransfer_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	whA := mustSeedWarehouse(t, srv, companyID)
	whB := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)

	cases := map[string]map[string]any{
		"missing to_warehouse_id": {
			"company_id": companyID, "from_warehouse_id": whA.ID, "transfer_date": today(),
			"lines": []map[string]any{{"product_id": p.ID, "quantity": 1}},
		},
		"same from and to warehouse": {
			"company_id": companyID, "from_warehouse_id": whA.ID, "to_warehouse_id": whA.ID, "transfer_date": today(),
			"lines": []map[string]any{{"product_id": p.ID, "quantity": 1}},
		},
		"empty lines": {
			"company_id": companyID, "from_warehouse_id": whA.ID, "to_warehouse_id": whB.ID, "transfer_date": today(),
			"lines": []map[string]any{},
		},
		"bad transfer_date": {
			"company_id": companyID, "from_warehouse_id": whA.ID, "to_warehouse_id": whB.ID, "transfer_date": "01-07-2026",
			"lines": []map[string]any{{"product_id": p.ID, "quantity": 1}},
		},
		"line zero quantity": {
			"company_id": companyID, "from_warehouse_id": whA.ID, "to_warehouse_id": whB.ID, "transfer_date": today(),
			"lines": []map[string]any{{"product_id": p.ID, "quantity": 0}},
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/stock-transfers", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func today() string {
	return time.Now().Format("2006-01-02")
}

type stockTransferView struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	TransferNumber string `json:"transfer_number"`
}

func mustCreateStockTransfer(t *testing.T, srvURL, companyID, fromWH, toWH, productID string, quantity float64) stockTransferView {
	t.Helper()
	resp := postJSON(t, srvURL+"/stock-transfers", map[string]any{
		"company_id": companyID, "from_warehouse_id": fromWH, "to_warehouse_id": toWH, "transfer_date": today(),
		"lines": []map[string]any{{"product_id": productID, "quantity": quantity}},
	})
	requireStatus(t, resp, http.StatusCreated)
	var tr stockTransferView
	resp.decode(t, &tr)
	return tr
}

func TestCreateStockTransfer_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	whA := mustSeedWarehouse(t, srv, companyID)
	whB := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)

	tr := mustCreateStockTransfer(t, srv.URL, companyID, whA.ID, whB.ID, p.ID, 10)
	if tr.Status != "DRAFT" {
		t.Errorf("status = %q, want DRAFT", tr.Status)
	}
	if !strings.HasPrefix(tr.TransferNumber, "TRF-") {
		t.Errorf("transfer_number = %q, want TRF- prefix", tr.TransferNumber)
	}

	getResp := getJSON(t, srv.URL+"/stock-transfers/"+tr.ID)
	requireStatus(t, getResp, http.StatusOK)
}

func TestConfirmStockTransfer_MovesStockBetweenWarehouses(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	whA := mustSeedWarehouse(t, srv, companyID)
	whB := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)
	mustGiveStock(t, srv, companyID, whA.ID, p.ID, 100)

	tr := mustCreateStockTransfer(t, srv.URL, companyID, whA.ID, whB.ID, p.ID, 30)

	confirmResp := postJSON(t, srv.URL+"/stock-transfers/"+tr.ID+"/confirm", nil)
	requireStatus(t, confirmResp, http.StatusOK)
	var confirmed stockTransferView
	confirmResp.decode(t, &confirmed)
	if confirmed.Status != "CONFIRMED" {
		t.Fatalf("status = %q, want CONFIRMED", confirmed.Status)
	}

	if got := mustGetStockQuantity(t, srv, companyID, whA.ID, p.ID); got != 70 {
		t.Errorf("source warehouse quantity = %.2f, want 70.00 (100-30)", got)
	}
	if got := mustGetStockQuantity(t, srv, companyID, whB.ID, p.ID); got != 30 {
		t.Errorf("destination warehouse quantity = %.2f, want 30.00", got)
	}

	// Confirming twice must fail.
	requireStatus(t, postJSON(t, srv.URL+"/stock-transfers/"+tr.ID+"/confirm", nil), http.StatusConflict)
}

func TestConfirmStockTransfer_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := postJSON(t, srv.URL+"/stock-transfers/"+uuid.NewString()+"/confirm", nil)
	requireStatus(t, resp, http.StatusNotFound)
}

func TestGetStockTransfer_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/stock-transfers/"+uuid.NewString())
	requireStatus(t, resp, http.StatusNotFound)
}

func TestListStockTransfers_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/stock-transfers")
	requireStatus(t, resp, http.StatusBadRequest)
}

// TestListStockTransfers_FilteredByBranch confirms branch_id filtering is
// NULL-inclusive: a branch filter must still surface unassigned (NULL
// branch_id) rows alongside that branch's own rows.
func TestListStockTransfers_FilteredByBranch(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	whA := mustSeedWarehouse(t, srv, companyID)
	whB := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)
	branchA := uuid.NewString()
	branchB := uuid.NewString()

	mkTransfer := func(branchID *string) {
		resp := postJSON(t, srv.URL+"/stock-transfers", map[string]any{
			"company_id": companyID, "branch_id": branchID, "from_warehouse_id": whA.ID, "to_warehouse_id": whB.ID, "transfer_date": today(),
			"lines": []map[string]any{{"product_id": p.ID, "quantity": 1}},
		})
		requireStatus(t, resp, http.StatusCreated)
	}
	mkTransfer(&branchA)
	mkTransfer(nil)
	mkTransfer(&branchB)

	resp := getJSON(t, srv.URL+"/stock-transfers?company_id="+companyID+"&branch_id="+branchA)
	requireStatus(t, resp, http.StatusOK)
	var transfers []struct {
		BranchID *string `json:"branch_id"`
	}
	resp.decode(t, &transfers)
	if len(transfers) != 2 {
		t.Fatalf("expected 2 transfers (branchA + NULL), got %d: %+v", len(transfers), transfers)
	}
	for _, tr := range transfers {
		if tr.BranchID != nil && *tr.BranchID == branchB {
			t.Errorf("branchB transfer leaked into branchA-filtered results: %+v", transfers)
		}
	}
}
