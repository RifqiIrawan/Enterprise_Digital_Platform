package httpapi

import (
	"net/http"
	"sort"
	"strconv"
	"time"
)

// Forecasting ini sengaja BUKAN model ML sungguhan -- proyeksi tren
// sederhana (regresi linear di atas agregasi bulanan histori yang sudah
// ada), dihitung on-the-fly tanpa training/database terpisah, konsisten
// dengan sifat ai-bi-service yang stateless. Cukup untuk sinyal arah tren,
// bukan prediksi presisi.

type periodValue struct {
	Period string  `json:"period"`
	Value  float64 `json:"value"`
}

type forecastSeries struct {
	History  []periodValue `json:"history"`
	Forecast []periodValue `json:"forecast"`
}

type forecastingResponse struct {
	CompanyID    string         `json:"company_id"`
	GeneratedAt  time.Time      `json:"generated_at"`
	SalesRevenue forecastSeries `json:"sales_revenue"`
	StockLevel   forecastSeries `json:"stock_level"`
	Errors       []sourceError  `json:"errors"`
}

func (h *Handler) forecastingSummary(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	historyMonths := intParam(r, "history_months", 6)
	forecastMonths := intParam(r, "forecast_months", 3)

	resp := forecastingResponse{
		CompanyID:   companyID,
		GeneratedAt: time.Now(),
		Errors:      []sourceError{},
	}

	salesHistory, err := h.monthlySalesRevenue(companyID, historyMonths)
	if err != nil {
		resp.Errors = append(resp.Errors, sourceError{Source: "sales-service", Message: err.Error()})
	} else {
		resp.SalesRevenue = projectSeries(salesHistory, forecastMonths)
	}

	stockHistory, err := h.monthlyStockLevel(companyID, historyMonths)
	if err != nil {
		resp.Errors = append(resp.Errors, sourceError{Source: "warehouse-service", Message: err.Error()})
	} else {
		resp.StockLevel = projectSeries(stockHistory, forecastMonths)
	}

	writeJSON(w, http.StatusOK, resp)
}

func intParam(r *http.Request, name string, fallback int) int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// monthlySalesRevenue mengelompokkan sales order jadi total revenue per
// bulan untuk `months` bulan terakhir (termasuk bulan berjalan).
func (h *Handler) monthlySalesRevenue(companyID string, months int) ([]periodValue, error) {
	var orders []struct {
		OrderDate   string  `json:"order_date"`
		TotalAmount float64 `json:"total_amount"`
	}
	if err := h.getJSON(h.cfg.SalesServiceURL, "/sales-orders", companyID, &orders); err != nil {
		return nil, err
	}

	byMonth := map[string]float64{}
	for _, o := range orders {
		if len(o.OrderDate) < 7 {
			continue
		}
		byMonth[o.OrderDate[:7]] += o.TotalAmount
	}
	return fillMonthlySeries(byMonth, months), nil
}

// monthlyStockLevel merekonstruksi total kuantitas stok (semua produk,
// semua gudang) di akhir tiap bulan dengan menjumlahkan running total dari
// seluruh histori stock_movements (IN positif, OUT negatif) secara
// kronologis. Catatan: endpoint GET /stock-movements di warehouse-service
// dibatasi 200 baris terbaru, jadi rekonstruksi ini hanya akurat kalau
// total mutasi belum melebihi itu (cukup untuk data demo/skala kecil).
func (h *Handler) monthlyStockLevel(companyID string, months int) ([]periodValue, error) {
	var movements []struct {
		MovementType string  `json:"movement_type"`
		Quantity     float64 `json:"quantity"`
		MovementDate string  `json:"movement_date"`
	}
	if err := h.getJSON(h.cfg.WarehouseServiceURL, "/stock-movements", companyID, &movements); err != nil {
		return nil, err
	}

	sort.Slice(movements, func(i, j int) bool { return movements[i].MovementDate < movements[j].MovementDate })

	runningTotal := 0.0
	levelAtMonthEnd := map[string]float64{}
	for _, m := range movements {
		if len(m.MovementDate) < 7 {
			continue
		}
		if m.MovementType == "IN" {
			runningTotal += m.Quantity
		} else {
			runningTotal -= m.Quantity
		}
		levelAtMonthEnd[m.MovementDate[:7]] = runningTotal
	}
	return fillMonthlySeries(levelAtMonthEnd, months), nil
}

// fillMonthlySeries mengisi `months` bulan terakhir secara berurutan; bulan
// tanpa data pakai nilai bulan sebelumnya (carry-forward) supaya cocok
// untuk seri level seperti stok, dan 0 kalau belum pernah ada data sama
// sekali (wajar untuk seri revenue yang belum ada transaksi).
func fillMonthlySeries(byMonth map[string]float64, months int) []periodValue {
	now := time.Now()
	series := make([]periodValue, 0, months)
	var lastValue float64
	var hasValue bool
	for i := months - 1; i >= 0; i-- {
		month := now.AddDate(0, -i, 0).Format("2006-01")
		value, ok := byMonth[month]
		if ok {
			lastValue = value
			hasValue = true
		} else if hasValue {
			value = lastValue
		}
		series = append(series, periodValue{Period: month, Value: value})
	}
	return series
}

// projectSeries menghitung regresi linear sederhana atas `history` (x =
// indeks bulan berurutan) lalu memproyeksikan `forecastMonths` bulan ke
// depan. Kalau titik data kurang dari 2, proyeksi dilewati (tidak cukup
// untuk menghitung tren).
func projectSeries(history []periodValue, forecastMonths int) forecastSeries {
	result := forecastSeries{History: history, Forecast: []periodValue{}}
	if len(history) < 2 {
		return result
	}

	xs := make([]float64, len(history))
	ys := make([]float64, len(history))
	for i, p := range history {
		xs[i] = float64(i)
		ys[i] = p.Value
	}
	slope, intercept := linearRegression(xs, ys)

	lastPeriod, err := time.Parse("2006-01", history[len(history)-1].Period)
	if err != nil {
		return result
	}
	for i := 1; i <= forecastMonths; i++ {
		x := float64(len(history) - 1 + i)
		value := slope*x + intercept
		if value < 0 {
			value = 0
		}
		period := lastPeriod.AddDate(0, i, 0).Format("2006-01")
		result.Forecast = append(result.Forecast, periodValue{Period: period, Value: value})
	}
	return result
}

func linearRegression(xs, ys []float64) (slope, intercept float64) {
	n := float64(len(xs))
	var sumX, sumY, sumXY, sumXX float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumXX += xs[i] * xs[i]
	}
	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return 0, sumY / n
	}
	slope = (n*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / n
	return slope, intercept
}
