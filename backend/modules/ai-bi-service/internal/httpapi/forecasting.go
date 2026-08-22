package httpapi

import (
	"math"
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

// forecastPoint membawa rentang, bukan satu angka: proyeksi dari 6-24 titik
// data selalu punya ketidakpastian yang besar, dan menampilkan satu garis mulus
// membuatnya terbaca lebih pasti daripada yang sebenarnya. Lower/Upper adalah
// interval 95% dari sebaran residu model terhadap historinya sendiri.
type forecastPoint struct {
	Period string  `json:"period"`
	Value  float64 `json:"value"`
	Lower  float64 `json:"lower"`
	Upper  float64 `json:"upper"`
}

type forecastSeries struct {
	History  []periodValue   `json:"history"`
	Forecast []forecastPoint `json:"forecast"`
	// Method menjelaskan cara perhitungannya supaya pembaca dashboard tahu
	// seberapa jauh angkanya bisa dipercaya: "seasonal" (tren + pola bulanan),
	// "linear" (tren saja), atau "insufficient_data".
	Method string `json:"method"`
	// SeasonalPeriod = 12 saat pola bulanan dipakai, 0 kalau tidak.
	SeasonalPeriod int `json:"seasonal_period"`
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
// seasonalPeriod: pola tahunan pada data bulanan. Butuh minimal DUA siklus
// penuh sebelum dipakai -- dengan satu siklus, "pola musiman" tidak bisa
// dibedakan dari kejadian sekali lewat.
const seasonalPeriod = 12
const minSeasonalPoints = seasonalPeriod * 2

// zScore95 adalah pengali interval kepercayaan 95% untuk sebaran normal.
const zScore95 = 1.96

// projectSeries memproyeksikan histori bulanan ke depan.
//
// Dua lapis:
//  1. TREN -- regresi linear atas seluruh titik histori (seperti sebelumnya).
//  2. POLA MUSIMAN -- kalau histori mencakup >= 2 siklus penuh (24 bulan),
//     residu tren dirata-ratakan per BULAN KALENDER lalu dipusatkan ke nol
//     (dekomposisi aditif sederhana). Bulan Desember yang selalu tinggi tidak
//     lagi terbaca sebagai "tren naik" yang lalu diteruskan ke Januari.
//
// Kalau historinya lebih pendek dari itu, hasilnya persis seperti dulu (tren
// saja) -- ditandai method = "linear" supaya perbedaannya kelihatan di UI,
// bukan disembunyikan.
func projectSeries(history []periodValue, forecastMonths int) forecastSeries {
	result := forecastSeries{History: history, Forecast: []forecastPoint{}, Method: "insufficient_data"}
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
	result.Method = "linear"

	// Indeks musiman per bulan kalender (1-12), 0 = tidak dipakai.
	var seasonal map[int]float64
	if len(history) >= minSeasonalPoints {
		seasonal = seasonalIndices(history, slope, intercept)
		if seasonal != nil {
			result.Method = "seasonal"
			result.SeasonalPeriod = seasonalPeriod
		}
	}

	fitted := func(i int, period string) float64 {
		v := slope*float64(i) + intercept
		if seasonal != nil {
			if m, err := time.Parse("2006-01", period); err == nil {
				v += seasonal[int(m.Month())]
			}
		}
		return v
	}

	// Sebaran residu terhadap histori sendiri jadi dasar lebar interval.
	var sqSum float64
	for i, p := range history {
		d := p.Value - fitted(i, p.Period)
		sqSum += d * d
	}
	stdErr := math.Sqrt(sqSum / float64(len(history)))
	band := zScore95 * stdErr

	lastPeriod, err := time.Parse("2006-01", history[len(history)-1].Period)
	if err != nil {
		return result
	}
	for i := 1; i <= forecastMonths; i++ {
		period := lastPeriod.AddDate(0, i, 0).Format("2006-01")
		value := fitted(len(history)-1+i, period)
		if value < 0 {
			value = 0
		}
		lower := value - band
		if lower < 0 {
			// Penjualan & stok tidak bisa negatif; batas bawah dipotong di nol
			// alih-alih menampilkan angka yang mustahil.
			lower = 0
		}
		result.Forecast = append(result.Forecast, forecastPoint{
			Period: period, Value: value, Lower: lower, Upper: value + band,
		})
	}
	return result
}

// seasonalIndices menghitung rata-rata residu tren per bulan kalender, lalu
// memusatkannya ke nol supaya pola musiman hanya MENGGESER nilai, tidak ikut
// menaikkan/menurunkan levelnya (itu tugas tren).
//
// Mengembalikan nil kalau ada bulan kalender yang tidak punya satu pun titik
// data: indeks yang tidak lengkap akan membuat sebagian bulan proyeksi memakai
// pola dan sebagian tidak -- lebih jujur kembali ke tren saja.
func seasonalIndices(history []periodValue, slope, intercept float64) map[int]float64 {
	sums := map[int]float64{}
	counts := map[int]int{}
	for i, p := range history {
		m, err := time.Parse("2006-01", p.Period)
		if err != nil {
			return nil
		}
		residual := p.Value - (slope*float64(i) + intercept)
		sums[int(m.Month())] += residual
		counts[int(m.Month())]++
	}
	if len(counts) < seasonalPeriod {
		return nil
	}

	indices := make(map[int]float64, seasonalPeriod)
	var total float64
	for month, sum := range sums {
		indices[month] = sum / float64(counts[month])
		total += indices[month]
	}
	mean := total / float64(len(indices))
	for month := range indices {
		indices[month] -= mean
	}
	return indices
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
