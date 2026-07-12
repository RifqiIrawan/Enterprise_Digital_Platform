package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func today() string {
	return time.Now().Format("2006-01-02")
}

type workOrderView struct {
	ID               string   `json:"id"`
	Status           string   `json:"status"`
	WONumber         string   `json:"wo_number"`
	QuantityPlanned  float64  `json:"quantity_planned"`
	QuantityProduced *float64 `json:"quantity_produced"`
}

func mustCreateWorkOrder(t *testing.T, srvURL, companyID, bomID, warehouseID string, quantityPlanned float64) workOrderView {
	t.Helper()
	resp := postJSON(t, srvURL+"/work-orders", map[string]any{
		"company_id": companyID, "bom_id": bomID, "warehouse_id": warehouseID,
		"quantity_planned": quantityPlanned, "planned_start_date": today(),
	})
	requireStatus(t, resp, http.StatusCreated)
	var wo workOrderView
	resp.decode(t, &wo)
	return wo
}

func mustStartWorkOrder(t *testing.T, srvURL, woID string) workOrderView {
	t.Helper()
	resp := postJSON(t, srvURL+"/work-orders/"+woID+"/start", nil)
	requireStatus(t, resp, http.StatusOK)
	var wo workOrderView
	resp.decode(t, &wo)
	return wo
}

func TestCreateWorkOrder_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	bom := mustSeedBOM(t, srv, companyID)
	warehouseID := uuid.NewString()

	cases := map[string]map[string]any{
		"missing bom_id": {
			"company_id": companyID, "warehouse_id": warehouseID, "quantity_planned": 5, "planned_start_date": today(),
		},
		"missing warehouse_id": {
			"company_id": companyID, "bom_id": bom.ID, "quantity_planned": 5, "planned_start_date": today(),
		},
		"zero quantity_planned": {
			"company_id": companyID, "bom_id": bom.ID, "warehouse_id": warehouseID, "quantity_planned": 0, "planned_start_date": today(),
		},
		"bad planned_start_date": {
			"company_id": companyID, "bom_id": bom.ID, "warehouse_id": warehouseID, "quantity_planned": 5, "planned_start_date": "01-07-2026",
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/work-orders", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateWorkOrder_BOMNotFound(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	resp := postJSON(t, srv.URL+"/work-orders", map[string]any{
		"company_id": companyID, "bom_id": uuid.NewString(), "warehouse_id": uuid.NewString(),
		"quantity_planned": 5, "planned_start_date": today(),
	})
	requireStatus(t, resp, http.StatusNotFound)
}

func TestCreateWorkOrder_InactiveBOMConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	bom := mustSeedBOM(t, srv, companyID)

	requireStatus(t, doRequest(t, http.MethodPut, srv.URL+"/boms/"+bom.ID, map[string]any{
		"name": "Resep", "is_active": false,
	}, ""), http.StatusOK)

	resp := postJSON(t, srv.URL+"/work-orders", map[string]any{
		"company_id": companyID, "bom_id": bom.ID, "warehouse_id": uuid.NewString(),
		"quantity_planned": 5, "planned_start_date": today(),
	})
	requireStatus(t, resp, http.StatusConflict)
}

func TestCreateWorkOrder_SnapshotsComponentRequirement(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	bom := mustSeedBOM(t, srv, companyID) // quantity_per_unit = 2
	warehouseID := uuid.NewString()

	resp := postJSON(t, srv.URL+"/work-orders", map[string]any{
		"company_id": companyID, "bom_id": bom.ID, "warehouse_id": warehouseID,
		"quantity_planned": 10, "planned_start_date": today(),
	})
	requireStatus(t, resp, http.StatusCreated)

	var wo struct {
		Status          string  `json:"status"`
		WONumber        string  `json:"wo_number"`
		QuantityPlanned float64 `json:"quantity_planned"`
		Lines           []struct {
			ComponentProductID string  `json:"component_product_id"`
			QuantityRequired   float64 `json:"quantity_required"`
		} `json:"lines"`
	}
	resp.decode(t, &wo)
	if wo.Status != "DRAFT" {
		t.Errorf("status = %q, want DRAFT", wo.Status)
	}
	if !strings.HasPrefix(wo.WONumber, "WO-") {
		t.Errorf("wo_number = %q, want WO- prefix", wo.WONumber)
	}
	if len(wo.Lines) != 1 {
		t.Fatalf("expected 1 snapshotted line, got %d", len(wo.Lines))
	}
	// quantity_required = bom.quantity_per_unit (2) * quantity_planned (10) = 20
	if wo.Lines[0].ComponentProductID != bom.ComponentProductID || wo.Lines[0].QuantityRequired != 20 {
		t.Errorf("unexpected snapshotted line: %+v, want component %q qty 20", wo.Lines[0], bom.ComponentProductID)
	}
}

func TestStartWorkOrder_Lifecycle(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	bom := mustSeedBOM(t, srv, companyID)
	wo := mustCreateWorkOrder(t, srv.URL, companyID, bom.ID, uuid.NewString(), 5)

	started := mustStartWorkOrder(t, srv.URL, wo.ID)
	if started.Status != "IN_PROGRESS" {
		t.Fatalf("status = %q, want IN_PROGRESS", started.Status)
	}

	// Starting twice must fail.
	requireStatus(t, postJSON(t, srv.URL+"/work-orders/"+wo.ID+"/start", nil), http.StatusConflict)
}

func TestCompleteWorkOrder_ValidationAndWrongStatus(t *testing.T) {
	srv, _ := newServerWithWarehouseStub(t, 0)
	companyID := newCompanyID(t)
	bom := mustSeedBOM(t, srv, companyID)
	wo := mustCreateWorkOrder(t, srv.URL, companyID, bom.ID, uuid.NewString(), 5) // still DRAFT

	zeroQty := postJSON(t, srv.URL+"/work-orders/"+wo.ID+"/complete", map[string]any{"quantity_produced": 0})
	requireStatus(t, zeroQty, http.StatusBadRequest)

	wrongStatus := postJSON(t, srv.URL+"/work-orders/"+wo.ID+"/complete", map[string]any{"quantity_produced": 5})
	requireStatus(t, wrongStatus, http.StatusConflict)
}

func TestCompleteWorkOrder_Success(t *testing.T) {
	srv, calls := newServerWithWarehouseStub(t, 0)
	companyID := newCompanyID(t)
	bom := mustSeedBOM(t, srv, companyID) // quantity_per_unit = 2
	warehouseID := uuid.NewString()
	wo := mustCreateWorkOrder(t, srv.URL, companyID, bom.ID, warehouseID, 10)
	mustStartWorkOrder(t, srv.URL, wo.ID)

	resp := postJSON(t, srv.URL+"/work-orders/"+wo.ID+"/complete", map[string]any{"quantity_produced": 9})
	requireStatus(t, resp, http.StatusOK)
	var completed workOrderView
	resp.decode(t, &completed)
	if completed.Status != "COMPLETED" {
		t.Errorf("status = %q, want COMPLETED", completed.Status)
	}
	if completed.QuantityProduced == nil || *completed.QuantityProduced != 9 {
		t.Errorf("quantity_produced = %v, want 9", completed.QuantityProduced)
	}

	if len(*calls) != 2 {
		t.Fatalf("expected 2 calls to warehouse-service (OUT components, IN finished goods), got %d", len(*calls))
	}
	outCall, inCall := (*calls)[0], (*calls)[1]
	if outCall.movementType != "OUT" {
		t.Errorf("first call movement_type = %q, want OUT (component consumption)", outCall.movementType)
	}
	if inCall.movementType != "IN" {
		t.Errorf("second call movement_type = %q, want IN (finished goods)", inCall.movementType)
	}

	var outSent struct {
		WarehouseID   string `json:"warehouse_id"`
		ReferenceType string `json:"reference_type"`
		ReferenceID   string `json:"reference_id"`
		Lines         []struct {
			ProductID string  `json:"product_id"`
			Quantity  float64 `json:"quantity"`
		} `json:"lines"`
	}
	if err := json.Unmarshal(outCall.body, &outSent); err != nil {
		t.Fatalf("decode OUT call body: %v", err)
	}
	if outSent.WarehouseID != warehouseID || outSent.ReferenceType != "WORK_ORDER" || outSent.ReferenceID != wo.ID {
		t.Errorf("unexpected OUT call metadata: %+v", outSent)
	}
	// quantity_required snapshotted as quantity_per_unit(2) * quantity_planned(10) = 20
	if len(outSent.Lines) != 1 || outSent.Lines[0].ProductID != bom.ComponentProductID || outSent.Lines[0].Quantity != 20 {
		t.Errorf("unexpected OUT lines: %+v", outSent.Lines)
	}

	var inSent struct {
		Lines []struct {
			ProductID string  `json:"product_id"`
			Quantity  float64 `json:"quantity"`
		} `json:"lines"`
	}
	if err := json.Unmarshal(inCall.body, &inSent); err != nil {
		t.Fatalf("decode IN call body: %v", err)
	}
	if len(inSent.Lines) != 1 || inSent.Lines[0].ProductID != bom.ProductID || inSent.Lines[0].Quantity != 9 {
		t.Errorf("unexpected IN lines: %+v, want finished good %q qty 9", inSent.Lines, bom.ProductID)
	}

	// Completing an already-COMPLETED work order must be rejected.
	requireStatus(t, postJSON(t, srv.URL+"/work-orders/"+wo.ID+"/complete", map[string]any{"quantity_produced": 1}), http.StatusConflict)
}

func TestCompleteWorkOrder_ComponentConsumptionFailureLeavesInProgress(t *testing.T) {
	srv, calls := newServerWithWarehouseStub(t, 1) // fail the 1st call (OUT)
	companyID := newCompanyID(t)
	bom := mustSeedBOM(t, srv, companyID)
	wo := mustCreateWorkOrder(t, srv.URL, companyID, bom.ID, uuid.NewString(), 10)
	mustStartWorkOrder(t, srv.URL, wo.ID)

	resp := postJSON(t, srv.URL+"/work-orders/"+wo.ID+"/complete", map[string]any{"quantity_produced": 9})
	requireStatus(t, resp, http.StatusBadGateway)
	if len(*calls) != 1 {
		t.Fatalf("expected exactly 1 attempted warehouse-service call (OUT), got %d", len(*calls))
	}

	getResp := getJSON(t, srv.URL+"/work-orders/"+wo.ID)
	requireStatus(t, getResp, http.StatusOK)
	var reloaded workOrderView
	getResp.decode(t, &reloaded)
	if reloaded.Status != "IN_PROGRESS" {
		t.Errorf("status = %q, want IN_PROGRESS after component consumption failure", reloaded.Status)
	}
	if reloaded.QuantityProduced != nil {
		t.Errorf("quantity_produced = %v, want nil after failure", reloaded.QuantityProduced)
	}
}

// TestCompleteWorkOrder_FinishedGoodsFailureAfterComponentsConsumed documents
// a real partial-failure gap: if the SECOND warehouse-service call (finished
// goods IN) fails, the FIRST call (component OUT) already succeeded and
// warehouse-service has genuinely consumed the components -- but the work
// order's local status still shows IN_PROGRESS with quantity_produced unset,
// because production-service only updates its own row after BOTH calls
// succeed. Re-completing would consume the components a second time. This is
// documented as existing behavior (same two-call-no-compensation pattern
// used everywhere else in this codebase), not fixed in this hardening pass.
func TestCompleteWorkOrder_FinishedGoodsFailureAfterComponentsConsumed(t *testing.T) {
	srv, calls := newServerWithWarehouseStub(t, 2) // fail the 2nd call (IN)
	companyID := newCompanyID(t)
	bom := mustSeedBOM(t, srv, companyID)
	wo := mustCreateWorkOrder(t, srv.URL, companyID, bom.ID, uuid.NewString(), 10)
	mustStartWorkOrder(t, srv.URL, wo.ID)

	resp := postJSON(t, srv.URL+"/work-orders/"+wo.ID+"/complete", map[string]any{"quantity_produced": 9})
	requireStatus(t, resp, http.StatusBadGateway)
	if len(*calls) != 2 {
		t.Fatalf("expected 2 attempted warehouse-service calls (OUT succeeded, IN failed), got %d", len(*calls))
	}
	if (*calls)[0].movementType != "OUT" || (*calls)[1].movementType != "IN" {
		t.Fatalf("unexpected call order: %+v", *calls)
	}

	getResp := getJSON(t, srv.URL+"/work-orders/"+wo.ID)
	requireStatus(t, getResp, http.StatusOK)
	var reloaded workOrderView
	getResp.decode(t, &reloaded)
	if reloaded.Status != "IN_PROGRESS" {
		t.Errorf("status = %q, want IN_PROGRESS (local status unchanged despite components already consumed upstream)", reloaded.Status)
	}
}

func TestCompleteWorkOrder_NotFound(t *testing.T) {
	srv, _ := newServerWithWarehouseStub(t, 0)
	resp := postJSON(t, srv.URL+"/work-orders/"+uuid.NewString()+"/complete", map[string]any{"quantity_produced": 5})
	requireStatus(t, resp, http.StatusNotFound)
}

func TestGetWorkOrder_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/work-orders/"+uuid.NewString())
	requireStatus(t, resp, http.StatusNotFound)
}

func TestListWorkOrders_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/work-orders")
	requireStatus(t, resp, http.StatusBadRequest)
}
