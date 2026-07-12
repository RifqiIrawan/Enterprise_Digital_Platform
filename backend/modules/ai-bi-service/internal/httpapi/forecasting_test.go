package httpapi_test

import (
	"net/http"
	"testing"
	"time"
)

func TestForecastingSummary_MissingCompanyID(t *testing.T) {
	srv, _ := newServer(t)
	resp := getJSON(t, srv.URL+"/forecasting/summary")
	requireStatus(t, resp, http.StatusBadRequest)
}

// monthKey returns the "YYYY-MM" period the handler computes for
// `monthsAgo` months before now (0 = current month) -- mirrors
// fillMonthlySeries's own `time.Now().AddDate(0, -i, 0)` logic so test
// fixtures land in the exact bucket the handler expects.
func monthKey(monthsAgo int) string {
	return time.Now().AddDate(0, -monthsAgo, 0).Format("2006-01")
}

func monthDate(monthsAgo int, day string) string {
	return monthKey(monthsAgo) + "-" + day
}

type forecastSeriesView struct {
	History []struct {
		Period string  `json:"period"`
		Value  float64 `json:"value"`
	} `json:"history"`
	Forecast []struct {
		Period string  `json:"period"`
		Value  float64 `json:"value"`
	} `json:"forecast"`
}

type forecastingView struct {
	SalesRevenue forecastSeriesView `json:"sales_revenue"`
	StockLevel   forecastSeriesView `json:"stock_level"`
	Errors       []struct {
		Source  string `json:"source"`
		Message string `json:"message"`
	} `json:"errors"`
}

// TestForecastingSummary_ProjectsExactLinearTrend uses a dataset that is
// PERFECTLY linear (revenue = 100*month + 100) so the least-squares fit
// recovers the exact generating line -- this lets the test assert precise
// expected forecast values without re-deriving the regression formula.
func TestForecastingSummary_ProjectsExactLinearTrend(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)

	be.sales.json("/sales-orders", http.StatusOK, []map[string]any{
		{"order_date": monthDate(3, "10"), "total_amount": 100.0},
		{"order_date": monthDate(2, "10"), "total_amount": 200.0},
		{"order_date": monthDate(1, "10"), "total_amount": 300.0},
		{"order_date": monthDate(0, "10"), "total_amount": 400.0},
	})

	resp := getJSON(t, srv.URL+"/forecasting/summary?company_id="+companyID+"&history_months=4&forecast_months=2")
	requireStatus(t, resp, http.StatusOK)
	var f forecastingView
	resp.decode(t, &f)

	if len(f.SalesRevenue.History) != 4 {
		t.Fatalf("expected 4 history points, got %d: %+v", len(f.SalesRevenue.History), f.SalesRevenue.History)
	}
	wantHistory := []float64{100, 200, 300, 400}
	for i, want := range wantHistory {
		if f.SalesRevenue.History[i].Value != want {
			t.Errorf("history[%d] = %.2f, want %.2f", i, f.SalesRevenue.History[i].Value, want)
		}
	}
	if len(f.SalesRevenue.Forecast) != 2 {
		t.Fatalf("expected 2 forecast points, got %d", len(f.SalesRevenue.Forecast))
	}
	// y = 100x + 100 fit exactly reproduces the next two points: 500, 600.
	wantForecast := []float64{500, 600}
	for i, want := range wantForecast {
		if f.SalesRevenue.Forecast[i].Value != want {
			t.Errorf("forecast[%d] = %.2f, want %.2f", i, f.SalesRevenue.Forecast[i].Value, want)
		}
	}
}

func TestForecastingSummary_StockLevelCarriesForwardOverGapMonths(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)

	be.warehouse.json("/stock-movements", http.StatusOK, []map[string]any{
		{"movement_type": "IN", "quantity": 100.0, "movement_date": monthDate(3, "05")},
		// month -2: no movement at all -- should carry forward from month -3.
		{"movement_type": "OUT", "quantity": 20.0, "movement_date": monthDate(1, "05")},
		// month 0 (current): no movement -- should carry forward from month -1.
	})

	resp := getJSON(t, srv.URL+"/forecasting/summary?company_id="+companyID+"&history_months=4")
	requireStatus(t, resp, http.StatusOK)
	var f forecastingView
	resp.decode(t, &f)

	if len(f.StockLevel.History) != 4 {
		t.Fatalf("expected 4 history points, got %d", len(f.StockLevel.History))
	}
	want := []float64{100, 100, 80, 80} // [-3]=100, [-2] carried=100, [-1]=80, [0] carried=80
	for i, w := range want {
		if f.StockLevel.History[i].Value != w {
			t.Errorf("stock_level.history[%d] = %.2f, want %.2f (period %s)", i, f.StockLevel.History[i].Value, w, f.StockLevel.History[i].Period)
		}
	}
}

func TestForecastingSummary_DefaultParams(t *testing.T) {
	srv, _ := newServer(t)
	companyID := newCompanyID(t)

	resp := getJSON(t, srv.URL+"/forecasting/summary?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var f forecastingView
	resp.decode(t, &f)

	if len(f.SalesRevenue.History) != 6 {
		t.Errorf("default history_months: got %d history points, want 6", len(f.SalesRevenue.History))
	}
	// No data at all -> every bucket is 0 and forecast is skipped only if
	// len(history) < 2, which isn't the case here (always 6 by default), so
	// a flat-zero regression still produces a (zero) forecast of length 3.
	if len(f.SalesRevenue.Forecast) != 3 {
		t.Errorf("default forecast_months: got %d forecast points, want 3", len(f.SalesRevenue.Forecast))
	}
}

func TestForecastingSummary_PartialFailure(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)

	be.sales.fail("/sales-orders")
	be.warehouse.json("/stock-movements", http.StatusOK, []map[string]any{
		{"movement_type": "IN", "quantity": 42.0, "movement_date": monthDate(0, "05")},
	})

	resp := getJSON(t, srv.URL+"/forecasting/summary?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var f forecastingView
	resp.decode(t, &f)

	if len(f.Errors) != 1 || f.Errors[0].Source != "sales-service" {
		t.Fatalf("expected exactly 1 error from sales-service, got %+v", f.Errors)
	}
	if len(f.StockLevel.History) == 0 || f.StockLevel.History[len(f.StockLevel.History)-1].Value != 42 {
		t.Errorf("expected stock_level to still be populated despite sales-service failing: %+v", f.StockLevel.History)
	}
	if len(f.SalesRevenue.History) != 0 {
		t.Errorf("expected sales_revenue to be zero-valued (empty) after failure, got %+v", f.SalesRevenue.History)
	}
}
