package httpapi_test

import (
	"net/http"
	"testing"
)

func TestAnomalyScan_MissingCompanyID(t *testing.T) {
	srv, _ := newServer(t)
	resp := getJSON(t, srv.URL+"/anomaly-detection/scan")
	requireStatus(t, resp, http.StatusBadRequest)
}

type anomalyView struct {
	Source     string  `json:"source"`
	EntityType string  `json:"entity_type"`
	EntityID   string  `json:"entity_id"`
	Label      string  `json:"label"`
	Value      float64 `json:"value"`
	Mean       float64 `json:"mean"`
	StdDev     float64 `json:"stddev"`
	ZScore     float64 `json:"z_score"`
}

type anomalyScanView struct {
	ThresholdZ float64       `json:"threshold_z"`
	Anomalies  []anomalyView `json:"anomalies"`
	Errors     []struct {
		Source  string `json:"source"`
		Message string `json:"message"`
	} `json:"errors"`
}

// The dataset [10,10,10,10,100] gives clean, hand-verifiable statistics:
// mean=28, population stddev=36, so the outlier's z-score is EXACTLY 2.0
// (the default threshold) and the four normal values sit at exactly -0.5.
// This lets the test assert precise z-scores without re-deriving the
// mean/stddev formula in the test itself.
func skewedSalesOrders() []map[string]any {
	return []map[string]any{
		{"id": "so-1", "so_number": "SO-0001", "total_amount": 10.0},
		{"id": "so-2", "so_number": "SO-0002", "total_amount": 10.0},
		{"id": "so-3", "so_number": "SO-0003", "total_amount": 10.0},
		{"id": "so-4", "so_number": "SO-0004", "total_amount": 10.0},
		{"id": "so-5", "so_number": "SO-0005", "total_amount": 100.0},
	}
}

func TestAnomalyScan_FlagsOutlierAtDefaultThreshold(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)
	be.sales.json("/sales-orders", http.StatusOK, skewedSalesOrders())

	resp := getJSON(t, srv.URL+"/anomaly-detection/scan?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var scan anomalyScanView
	resp.decode(t, &scan)

	if scan.ThresholdZ != 2.0 {
		t.Errorf("threshold_z = %.2f, want default 2.0", scan.ThresholdZ)
	}
	if len(scan.Anomalies) != 1 {
		t.Fatalf("expected exactly 1 anomaly, got %d: %+v", len(scan.Anomalies), scan.Anomalies)
	}
	a := scan.Anomalies[0]
	if a.Source != "sales-service" || a.EntityType != "sales_order" || a.EntityID != "so-5" || a.Label != "SO-0005" {
		t.Errorf("unexpected anomaly identity: %+v", a)
	}
	if a.Mean != 28 || a.StdDev != 36 || a.ZScore != 2.0 {
		t.Errorf("mean=%.2f stddev=%.2f z_score=%.2f, want mean=28.00 stddev=36.00 z_score=2.00", a.Mean, a.StdDev, a.ZScore)
	}
}

func TestAnomalyScan_LowerThresholdFlagsMore(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)
	be.sales.json("/sales-orders", http.StatusOK, skewedSalesOrders())

	// The 4 "normal" values sit at z=-0.5, so a threshold of 0.4 should flag
	// all 5 orders, not just the outlier.
	resp := getJSON(t, srv.URL+"/anomaly-detection/scan?company_id="+companyID+"&threshold_z=0.4")
	requireStatus(t, resp, http.StatusOK)
	var scan anomalyScanView
	resp.decode(t, &scan)

	if scan.ThresholdZ != 0.4 {
		t.Errorf("threshold_z = %.2f, want 0.4", scan.ThresholdZ)
	}
	if len(scan.Anomalies) != 5 {
		t.Fatalf("expected all 5 orders flagged at threshold 0.4, got %d: %+v", len(scan.Anomalies), scan.Anomalies)
	}
}

func TestAnomalyScan_SkipsWhenFewerThanThreeDataPoints(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)
	be.sales.json("/sales-orders", http.StatusOK, []map[string]any{
		{"id": "so-1", "so_number": "SO-0001", "total_amount": 10.0},
		{"id": "so-2", "so_number": "SO-0002", "total_amount": 999999.0},
	})

	resp := getJSON(t, srv.URL+"/anomaly-detection/scan?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var scan anomalyScanView
	resp.decode(t, &scan)
	if len(scan.Anomalies) != 0 {
		t.Errorf("expected no anomalies with only 2 data points (need >= 3), got %+v", scan.Anomalies)
	}
}

func TestAnomalyScan_SkipsWhenStdDevIsZero(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)
	be.sales.json("/sales-orders", http.StatusOK, []map[string]any{
		{"id": "so-1", "so_number": "SO-0001", "total_amount": 100.0},
		{"id": "so-2", "so_number": "SO-0002", "total_amount": 100.0},
		{"id": "so-3", "so_number": "SO-0003", "total_amount": 100.0},
	})

	resp := getJSON(t, srv.URL+"/anomaly-detection/scan?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var scan anomalyScanView
	resp.decode(t, &scan)
	if len(scan.Anomalies) != 0 {
		t.Errorf("expected no anomalies when all values are identical (stddev=0), got %+v", scan.Anomalies)
	}
}

func TestAnomalyScan_ScansPurchasingAndWarehouseDomains(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)

	be.purchasing.json("/purchase-orders", http.StatusOK, []map[string]any{
		{"id": "po-1", "po_number": "PO-0001", "total_amount": 10.0},
		{"id": "po-2", "po_number": "PO-0002", "total_amount": 10.0},
		{"id": "po-3", "po_number": "PO-0003", "total_amount": 10.0},
		{"id": "po-4", "po_number": "PO-0004", "total_amount": 10.0},
		{"id": "po-5", "po_number": "PO-0005", "total_amount": 100.0},
	})
	be.warehouse.json("/stock-movements", http.StatusOK, []map[string]any{
		{"id": "mv-1", "product_name": "Item A", "movement_type": "IN", "quantity": 10.0, "movement_date": "2026-07-01"},
		{"id": "mv-2", "product_name": "Item A", "movement_type": "IN", "quantity": 10.0, "movement_date": "2026-07-02"},
		{"id": "mv-3", "product_name": "Item A", "movement_type": "IN", "quantity": 10.0, "movement_date": "2026-07-03"},
		{"id": "mv-4", "product_name": "Item A", "movement_type": "IN", "quantity": 10.0, "movement_date": "2026-07-04"},
		{"id": "mv-5", "product_name": "Item A", "movement_type": "OUT", "quantity": 100.0, "movement_date": "2026-07-05"},
	})

	resp := getJSON(t, srv.URL+"/anomaly-detection/scan?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var scan anomalyScanView
	resp.decode(t, &scan)

	if len(scan.Anomalies) != 2 {
		t.Fatalf("expected 2 anomalies (1 purchasing + 1 warehouse), got %d: %+v", len(scan.Anomalies), scan.Anomalies)
	}
	bySource := map[string]anomalyView{}
	for _, a := range scan.Anomalies {
		bySource[a.Source] = a
	}
	po, ok := bySource["purchasing-service"]
	if !ok || po.EntityType != "purchase_order" || po.EntityID != "po-5" {
		t.Errorf("unexpected purchasing anomaly: %+v (all: %+v)", po, scan.Anomalies)
	}
	wh, ok := bySource["warehouse-service"]
	if !ok || wh.EntityType != "stock_movement" || wh.EntityID != "mv-5" {
		t.Errorf("unexpected warehouse anomaly: %+v (all: %+v)", wh, scan.Anomalies)
	}
}

func TestAnomalyScan_PartialFailure(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)
	be.sales.json("/sales-orders", http.StatusOK, skewedSalesOrders())
	be.purchasing.fail("/purchase-orders")

	resp := getJSON(t, srv.URL+"/anomaly-detection/scan?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var scan anomalyScanView
	resp.decode(t, &scan)

	if len(scan.Errors) != 1 || scan.Errors[0].Source != "purchasing-service" {
		t.Fatalf("expected exactly 1 error from purchasing-service, got %+v", scan.Errors)
	}
	if len(scan.Anomalies) != 1 || scan.Anomalies[0].Source != "sales-service" {
		t.Errorf("expected sales-service anomaly to still surface despite purchasing-service failing, got %+v", scan.Anomalies)
	}
}
