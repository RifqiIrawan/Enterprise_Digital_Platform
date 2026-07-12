package httpapi_test

import (
	"net/http"
	"testing"
	"time"
)

func TestDashboardSummary_MissingCompanyID(t *testing.T) {
	srv, _ := newServer(t)
	resp := getJSON(t, srv.URL+"/dashboards/summary")
	requireStatus(t, resp, http.StatusBadRequest)
}

type dashboardView struct {
	CompanyID string `json:"company_id"`
	Sales     struct {
		TotalOrders  int            `json:"total_orders"`
		TotalRevenue float64        `json:"total_revenue"`
		ByStatus     map[string]int `json:"by_status"`
	} `json:"sales"`
	Purchasing struct {
		TotalOrders int            `json:"total_orders"`
		TotalSpend  float64        `json:"total_spend"`
		ByStatus    map[string]int `json:"by_status"`
	} `json:"purchasing"`
	Finance struct {
		ARTotal             float64 `json:"ar_total"`
		AROutstanding       float64 `json:"ar_outstanding"`
		APTotal             float64 `json:"ap_total"`
		APOutstanding       float64 `json:"ap_outstanding"`
		JournalEntriesCount int     `json:"journal_entries_count"`
	} `json:"finance"`
	Warehouse struct {
		TotalProducts   int `json:"total_products"`
		TotalWarehouses int `json:"total_warehouses"`
		TotalStockLines int `json:"total_stock_lines"`
		LowStockCount   int `json:"low_stock_count"`
	} `json:"warehouse"`
	Production struct {
		TotalWorkOrders int            `json:"total_work_orders"`
		ByStatus        map[string]int `json:"by_status"`
	} `json:"production"`
	QC struct {
		TotalInspections int     `json:"total_inspections"`
		PassCount        int     `json:"pass_count"`
		FailCount        int     `json:"fail_count"`
		PartialCount     int     `json:"partial_count"`
		PassRatePct      float64 `json:"pass_rate_pct"`
	} `json:"qc"`
	HR struct {
		TotalEmployees  int `json:"total_employees"`
		ActiveEmployees int `json:"active_employees"`
	} `json:"hr"`
	Asset struct {
		TotalAssets             int `json:"total_assets"`
		ActiveAssets            int `json:"active_assets"`
		OverdueMaintenanceCount int `json:"overdue_maintenance_count"`
	} `json:"asset"`
	Errors []struct {
		Source  string `json:"source"`
		Message string `json:"message"`
	} `json:"errors"`
}

func TestDashboardSummary_AggregatesAllDomains(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)

	be.sales.json("/sales-orders", http.StatusOK, []map[string]any{
		{"status": "CONFIRMED", "total_amount": 100.0},
		{"status": "CONFIRMED", "total_amount": 200.0},
		{"status": "INVOICED", "total_amount": 300.0},
	})
	be.purchasing.json("/purchase-orders", http.StatusOK, []map[string]any{
		{"status": "RECEIVED", "total_amount": 50.0},
	})
	be.finance.json("/ar-ap-summary", http.StatusOK, []map[string]any{
		{"invoice_type": "AR", "total_amount": 500.0, "outstanding_amount": 200.0},
		{"invoice_type": "AP", "total_amount": 300.0, "outstanding_amount": 100.0},
	})
	be.finance.json("/journal-entries", http.StatusOK, []map[string]any{
		{"id": "je-1"}, {"id": "je-2"}, {"id": "je-3"},
	})
	be.warehouse.json("/products", http.StatusOK, []map[string]any{{"id": "p1"}, {"id": "p2"}})
	be.warehouse.json("/warehouses", http.StatusOK, []map[string]any{{"id": "w1"}})
	be.warehouse.json("/stock", http.StatusOK, []map[string]any{
		{"quantity": 5.0},  // below threshold (10) -> low stock
		{"quantity": 10.0}, // exactly at threshold -> NOT low stock (strict <)
		{"quantity": 50.0},
	})
	be.production.json("/work-orders", http.StatusOK, []map[string]any{
		{"status": "IN_PROGRESS"}, {"status": "COMPLETED"}, {"status": "COMPLETED"},
	})
	be.qc.json("/inspections", http.StatusOK, []map[string]any{
		{"result": "PASS"}, {"result": "PASS"}, {"result": "PASS"}, {"result": "FAIL"},
	})
	be.hr.json("/employees", http.StatusOK, []map[string]any{
		{"is_active": true}, {"is_active": true}, {"is_active": false},
	})
	be.asset.json("/assets", http.StatusOK, []map[string]any{
		{"status": "ACTIVE"}, {"status": "ACTIVE"}, {"status": "DISPOSED"},
	})
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	be.asset.json("/maintenance-schedules", http.StatusOK, []map[string]any{
		{"status": "SCHEDULED", "scheduled_date": yesterday}, // overdue
		{"status": "SCHEDULED", "scheduled_date": tomorrow},  // not yet due
		{"status": "COMPLETED", "scheduled_date": yesterday}, // done, doesn't count even though date passed
	})

	resp := getJSON(t, srv.URL+"/dashboards/summary?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var d dashboardView
	resp.decode(t, &d)

	if len(d.Errors) != 0 {
		t.Fatalf("expected no errors when all 8 services succeed, got %+v", d.Errors)
	}
	if d.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", d.CompanyID, companyID)
	}

	if d.Sales.TotalOrders != 3 || d.Sales.TotalRevenue != 600 || d.Sales.ByStatus["CONFIRMED"] != 2 || d.Sales.ByStatus["INVOICED"] != 1 {
		t.Errorf("unexpected sales summary: %+v", d.Sales)
	}
	if d.Purchasing.TotalOrders != 1 || d.Purchasing.TotalSpend != 50 {
		t.Errorf("unexpected purchasing summary: %+v", d.Purchasing)
	}
	if d.Finance.ARTotal != 500 || d.Finance.AROutstanding != 200 || d.Finance.APTotal != 300 || d.Finance.APOutstanding != 100 || d.Finance.JournalEntriesCount != 3 {
		t.Errorf("unexpected finance summary: %+v", d.Finance)
	}
	if d.Warehouse.TotalProducts != 2 || d.Warehouse.TotalWarehouses != 1 || d.Warehouse.TotalStockLines != 3 || d.Warehouse.LowStockCount != 1 {
		t.Errorf("unexpected warehouse summary: %+v (want low_stock_count=1: only qty=5 is strictly below threshold 10)", d.Warehouse)
	}
	if d.Production.TotalWorkOrders != 3 || d.Production.ByStatus["COMPLETED"] != 2 || d.Production.ByStatus["IN_PROGRESS"] != 1 {
		t.Errorf("unexpected production summary: %+v", d.Production)
	}
	if d.QC.TotalInspections != 4 || d.QC.PassCount != 3 || d.QC.FailCount != 1 || d.QC.PassRatePct != 75 {
		t.Errorf("unexpected qc summary: %+v (want pass_rate_pct=75)", d.QC)
	}
	if d.HR.TotalEmployees != 3 || d.HR.ActiveEmployees != 2 {
		t.Errorf("unexpected hr summary: %+v", d.HR)
	}
	if d.Asset.TotalAssets != 3 || d.Asset.ActiveAssets != 2 {
		t.Errorf("unexpected asset summary: %+v", d.Asset)
	}
	if d.Asset.OverdueMaintenanceCount != 1 {
		t.Errorf("overdue_maintenance_count = %d, want 1 (only the SCHEDULED+past-date one; COMPLETED-but-past and SCHEDULED-but-future don't count)", d.Asset.OverdueMaintenanceCount)
	}
}

func TestDashboardSummary_TogglesPartialFailure(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)

	be.sales.json("/sales-orders", http.StatusOK, []map[string]any{
		{"status": "CONFIRMED", "total_amount": 100.0},
	})
	be.qc.fail("/inspections")

	resp := getJSON(t, srv.URL+"/dashboards/summary?company_id="+companyID)
	// The endpoint itself must still return 200 even though one of the 8
	// upstream calls failed -- that's the whole point of the "tolerant of
	// partial failure" design documented in summary.go.
	requireStatus(t, resp, http.StatusOK)
	var d dashboardView
	resp.decode(t, &d)

	if d.Sales.TotalOrders != 1 || d.Sales.TotalRevenue != 100 {
		t.Errorf("expected sales summary to still be populated despite qc-service failing, got %+v", d.Sales)
	}
	if d.QC.TotalInspections != 0 {
		t.Errorf("expected qc summary to be zero-valued after failure, got %+v", d.QC)
	}
	if len(d.Errors) != 1 {
		t.Fatalf("expected exactly 1 recorded error, got %+v", d.Errors)
	}
	if d.Errors[0].Source != "qc-service" || d.Errors[0].Message == "" {
		t.Errorf("unexpected error entry: %+v", d.Errors[0])
	}
}

func TestDashboardSummary_AllServicesDown(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)

	for _, path := range []struct {
		b    *fakeService
		path string
	}{
		{be.sales, "/sales-orders"}, {be.purchasing, "/purchase-orders"},
		{be.finance, "/ar-ap-summary"}, {be.finance, "/journal-entries"},
		{be.warehouse, "/products"}, {be.warehouse, "/warehouses"}, {be.warehouse, "/stock"},
		{be.production, "/work-orders"}, {be.qc, "/inspections"},
		{be.hr, "/employees"}, {be.asset, "/assets"}, {be.asset, "/maintenance-schedules"},
	} {
		path.b.fail(path.path)
	}

	resp := getJSON(t, srv.URL+"/dashboards/summary?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var d dashboardView
	resp.decode(t, &d)

	if len(d.Errors) != 8 {
		t.Errorf("expected 8 recorded errors (one per downstream service), got %d: %+v", len(d.Errors), d.Errors)
	}
}
