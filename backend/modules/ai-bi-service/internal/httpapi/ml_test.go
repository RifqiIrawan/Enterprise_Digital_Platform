package httpapi_test

import (
	"math"
	"net/http"
	"testing"
)

// Dua peningkatan yang diuji di file ini:
//  1. Forecasting sadar pola musiman + interval kepercayaan.
//  2. Anomali memakai median/MAD yang tahan outlier, bukan mean/stddev yang
//     justru tertarik oleh outlier yang sedang dicari.

type forecastPointView struct {
	Period string  `json:"period"`
	Value  float64 `json:"value"`
	Lower  float64 `json:"lower"`
	Upper  float64 `json:"upper"`
}

type forecastSeriesFull struct {
	History []struct {
		Period string  `json:"period"`
		Value  float64 `json:"value"`
	} `json:"history"`
	Forecast       []forecastPointView `json:"forecast"`
	Method         string              `json:"method"`
	SeasonalPeriod int                 `json:"seasonal_period"`
}

type forecastingFullView struct {
	SalesRevenue forecastSeriesFull `json:"sales_revenue"`
	StockLevel   forecastSeriesFull `json:"stock_level"`
}

// Histori pendek (< 2 siklus) tetap memakai tren saja -- ditandai eksplisit,
// bukan diam-diam mengaku musiman.
func TestForecasting_ShortHistoryStaysLinear(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)

	orders := []map[string]any{}
	for m := 5; m >= 0; m-- {
		orders = append(orders, map[string]any{"order_date": monthDate(m, "10"), "total_amount": float64(100 * (6 - m))})
	}
	be.sales.json("/sales-orders", http.StatusOK, orders)

	resp := getJSON(t, srv.URL+"/forecasting/summary?company_id="+companyID+"&history_months=6&forecast_months=2")
	requireStatus(t, resp, http.StatusOK)
	var f forecastingFullView
	resp.decode(t, &f)

	if f.SalesRevenue.Method != "linear" {
		t.Fatalf("method = %q, want linear", f.SalesRevenue.Method)
	}
	if f.SalesRevenue.SeasonalPeriod != 0 {
		t.Errorf("seasonal_period = %d, want 0", f.SalesRevenue.SeasonalPeriod)
	}
}

// Interval kepercayaan: data yang persis linear tidak punya residu, jadi
// rentangnya nol. Itu sekaligus membuktikan lebarnya memang berasal dari
// sebaran residu, bukan angka tetap yang ditempel.
func TestForecasting_PerfectFitHasZeroBand(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)

	be.sales.json("/sales-orders", http.StatusOK, []map[string]any{
		{"order_date": monthDate(3, "10"), "total_amount": 100.0},
		{"order_date": monthDate(2, "10"), "total_amount": 200.0},
		{"order_date": monthDate(1, "10"), "total_amount": 300.0},
		{"order_date": monthDate(0, "10"), "total_amount": 400.0},
	})

	resp := getJSON(t, srv.URL+"/forecasting/summary?company_id="+companyID+"&history_months=4&forecast_months=1")
	requireStatus(t, resp, http.StatusOK)
	var f forecastingFullView
	resp.decode(t, &f)

	p := f.SalesRevenue.Forecast[0]
	if p.Value != 500 {
		t.Fatalf("value = %v, want 500", p.Value)
	}
	if p.Lower != 500 || p.Upper != 500 {
		t.Errorf("rentang = %v..%v, want 500..500 untuk data yang persis linear", p.Lower, p.Upper)
	}
}

// Data berisik: rentangnya harus melebar, dan batas bawah tidak boleh negatif
// (penjualan tidak bisa minus).
func TestForecasting_NoisyDataWidensBandAndClampsAtZero(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)

	// Nilai berayun jauh di sekitar level yang rendah.
	values := []float64{10, 400, 5, 380, 15, 420}
	orders := []map[string]any{}
	for i, v := range values {
		orders = append(orders, map[string]any{"order_date": monthDate(5-i, "10"), "total_amount": v})
	}
	be.sales.json("/sales-orders", http.StatusOK, orders)

	resp := getJSON(t, srv.URL+"/forecasting/summary?company_id="+companyID+"&history_months=6&forecast_months=1")
	requireStatus(t, resp, http.StatusOK)
	var f forecastingFullView
	resp.decode(t, &f)

	p := f.SalesRevenue.Forecast[0]
	if p.Upper <= p.Value {
		t.Errorf("upper (%v) seharusnya di atas value (%v) untuk data berisik", p.Upper, p.Value)
	}
	if p.Lower < 0 {
		t.Errorf("lower = %v, tidak boleh negatif", p.Lower)
	}
}

// Inti peningkatan forecasting: dengan 24 bulan histori dan lonjakan yang
// selalu jatuh di bulan yang sama, proyeksinya harus MENGIKUTI pola itu --
// bukan meratakannya seperti regresi linear polos.
func TestForecasting_SeasonalPatternIsFollowed(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)

	// 24 bulan: level datar 100, kecuali dua bulan kalender yang sama (18 dan 6
	// bulan lalu) yang melonjak ke 1000. Posisinya sengaja SIMETRIS di tengah
	// histori supaya tren keseluruhan tetap datar -- kalau lonjakannya
	// ditaruh di awal, regresi membacanya sebagai tren menurun dan proyeksinya
	// terpotong di nol sebelum pola musimannya sempat kelihatan.
	orders := []map[string]any{}
	for m := 23; m >= 0; m-- {
		amount := 100.0
		if m == 18 || m == 6 {
			amount = 1000.0
		}
		orders = append(orders, map[string]any{"order_date": monthDate(m, "10"), "total_amount": amount})
	}
	be.sales.json("/sales-orders", http.StatusOK, orders)

	resp := getJSON(t, srv.URL+"/forecasting/summary?company_id="+companyID+"&history_months=24&forecast_months=12")
	requireStatus(t, resp, http.StatusOK)
	var f forecastingFullView
	resp.decode(t, &f)

	if f.SalesRevenue.Method != "seasonal" || f.SalesRevenue.SeasonalPeriod != 12 {
		t.Fatalf("method/period = %q/%d, want seasonal/12", f.SalesRevenue.Method, f.SalesRevenue.SeasonalPeriod)
	}
	if len(f.SalesRevenue.Forecast) != 12 {
		t.Fatalf("expected 12 titik proyeksi, got %d", len(f.SalesRevenue.Forecast))
	}

	// Lonjakan terakhir 6 bulan lalu, jadi lonjakan berikutnya jatuh 6 bulan ke
	// depan = titik proyeksi ke-6 (indeks 5).
	var puncak, biasa float64
	for i, p := range f.SalesRevenue.Forecast {
		if i == 5 {
			puncak = p.Value
		} else if biasa == 0 {
			biasa = p.Value
		}
	}
	if puncak <= biasa*2 {
		t.Errorf("proyeksi bulan lonjakan (%v) seharusnya jauh di atas bulan biasa (%v)", puncak, biasa)
	}
}

type anomalyFullView struct {
	Anomalies []struct {
		Label  string  `json:"label"`
		Value  float64 `json:"value"`
		Mean   float64 `json:"mean"`
		StdDev float64 `json:"stddev"`
		Median float64 `json:"median"`
		MAD    float64 `json:"mad"`
		ZScore float64 `json:"z_score"`
		Method string  `json:"method"`
	} `json:"anomalies"`
}

// Masking effect: beberapa outlier ekstrem menaikkan stddev sedemikian rupa
// sehingga z klasik mereka sendiri turun di bawah ambang -- outlier menutupi
// dirinya sendiri. Median/MAD tidak tertarik oleh nilai ekstrem, jadi semuanya
// tetap tertandai.
func TestAnomaly_ModifiedZResistsMasking(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)

	// Nilai dasar sengaja BERVARIASI sedikit: kalau semuanya identik, MAD = 0
	// dan jalur median/MAD memang tidak berlaku (ada test terpisah untuk itu).
	orders := []map[string]any{}
	for _, v := range []float64{100, 104, 96, 108, 92, 102, 98, 106, 94, 101} {
		orders = append(orders, map[string]any{
			"id": newCompanyID(t), "so_number": "SO-N", "total_amount": v,
		})
	}
	// Lima outlier besar sekaligus -- cukup banyak untuk menggelembungkan
	// stddev sehingga z klasik mereka sendiri turun di bawah ambang.
	for range 5 {
		orders = append(orders, map[string]any{
			"id": newCompanyID(t), "so_number": "SO-OUTLIER", "total_amount": 5000.0,
		})
	}
	be.sales.json("/sales-orders", http.StatusOK, orders)

	resp := getJSON(t, srv.URL+"/anomaly-detection/scan?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var a anomalyFullView
	resp.decode(t, &a)

	outliers := 0
	for _, item := range a.Anomalies {
		if item.Label == "SO-OUTLIER" {
			outliers++
			if item.Method != "modified" {
				t.Errorf("method = %q, want modified", item.Method)
			}
			if item.MAD <= 0 {
				t.Errorf("mad = %v, seharusnya > 0", item.MAD)
			}
		}
	}
	if outliers != 5 {
		t.Fatalf("expected kelima outlier tertandai, got %d", outliers)
	}

	// Bukti masking-nya nyata: z KLASIK untuk outlier yang sama tidak sampai
	// ambang 2.0, jadi cara lama akan melewatkan ketiganya.
	first := a.Anomalies[0]
	classicZ := math.Abs((5000 - first.Mean) / first.StdDev)
	if classicZ >= 2.0 {
		t.Errorf("prasyarat test tidak terpenuhi: z klasik %v sudah >= 2.0, masking tidak terbukti", classicZ)
	}
	if math.Abs(first.ZScore) < 2.0 {
		t.Errorf("z modified = %v, seharusnya >= 2.0", first.ZScore)
	}
}

// Kalau lebih dari separuh nilainya identik, MAD = 0 dan median/MAD tidak bisa
// membedakan apa pun -- perhitungan klasik yang dipakai, bukan menyerah.
func TestAnomaly_FallsBackToClassicWhenMADIsZero(t *testing.T) {
	srv, be := newServer(t)
	companyID := newCompanyID(t)

	orders := []map[string]any{}
	for range 9 {
		orders = append(orders, map[string]any{
			"id": newCompanyID(t), "so_number": "SO-SAMA", "total_amount": 100.0,
		})
	}
	orders = append(orders, map[string]any{
		"id": newCompanyID(t), "so_number": "SO-BEDA", "total_amount": 900.0,
	})
	be.sales.json("/sales-orders", http.StatusOK, orders)

	resp := getJSON(t, srv.URL+"/anomaly-detection/scan?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var a anomalyFullView
	resp.decode(t, &a)

	if len(a.Anomalies) != 1 || a.Anomalies[0].Label != "SO-BEDA" {
		t.Fatalf("expected satu anomali SO-BEDA, got %+v", a.Anomalies)
	}
	if a.Anomalies[0].Method != "classic" {
		t.Errorf("method = %q, want classic (MAD = 0)", a.Anomalies[0].Method)
	}
	if a.Anomalies[0].MAD != 0 {
		t.Errorf("mad = %v, want 0", a.Anomalies[0].MAD)
	}
}
