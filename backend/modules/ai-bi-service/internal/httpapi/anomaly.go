package httpapi

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"
)

// Anomaly Detection ini heuristik z-score sederhana (bukan model ML) --
// menandai transaksi yang nilainya jauh dari rata-rata historisnya sendiri
// per domain. Cukup untuk sinyal "ini beda dari biasanya, layak dicek
// manual", bukan klasifikasi fraud/error yang pasti benar.

type anomaly struct {
	Source     string  `json:"source"`
	EntityType string  `json:"entity_type"`
	EntityID   string  `json:"entity_id"`
	Label      string  `json:"label"`
	Value      float64 `json:"value"`
	Mean       float64 `json:"mean"`
	StdDev     float64 `json:"stddev"`
	Median     float64 `json:"median"`
	MAD        float64 `json:"mad"`
	ZScore     float64 `json:"z_score"`
	// Method menyebut dasar perhitungan z-nya: "modified" (median/MAD, tahan
	// outlier) atau "classic" (mean/stddev, dipakai saat MAD = 0).
	Method string `json:"method"`
	Reason string `json:"reason"`
}

type anomalyScanResponse struct {
	CompanyID   string        `json:"company_id"`
	GeneratedAt time.Time     `json:"generated_at"`
	ThresholdZ  float64       `json:"threshold_z"`
	Anomalies   []anomaly     `json:"anomalies"`
	Errors      []sourceError `json:"errors"`
}

func (h *Handler) anomalyScan(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	threshold := floatParam(r, "threshold_z", 2.0)

	resp := anomalyScanResponse{
		CompanyID:   companyID,
		GeneratedAt: time.Now(),
		ThresholdZ:  threshold,
		Anomalies:   []anomaly{},
		Errors:      []sourceError{},
	}

	if items, err := h.scanSalesOrders(companyID, threshold); err != nil {
		resp.Errors = append(resp.Errors, sourceError{Source: "sales-service", Message: err.Error()})
	} else {
		resp.Anomalies = append(resp.Anomalies, items...)
	}

	if items, err := h.scanPurchaseOrders(companyID, threshold); err != nil {
		resp.Errors = append(resp.Errors, sourceError{Source: "purchasing-service", Message: err.Error()})
	} else {
		resp.Anomalies = append(resp.Anomalies, items...)
	}

	if items, err := h.scanStockMovements(companyID, threshold); err != nil {
		resp.Errors = append(resp.Errors, sourceError{Source: "warehouse-service", Message: err.Error()})
	} else {
		resp.Anomalies = append(resp.Anomalies, items...)
	}

	writeJSON(w, http.StatusOK, resp)
}

func floatParam(r *http.Request, name string, fallback float64) float64 {
	v := r.URL.Query().Get(name)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		return fallback
	}
	return f
}

func meanStdDev(values []float64) (mean, stddev float64) {
	n := float64(len(values))
	if n == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean = sum / n
	var sqSum float64
	for _, v := range values {
		sqSum += (v - mean) * (v - mean)
	}
	stddev = math.Sqrt(sqSum / n)
	return mean, stddev
}

// madScale membuat MAD sebanding dengan simpangan baku pada sebaran normal
// (0.6745 adalah kuartil ke-0.75 dari normal baku), sehingga ambang batas yang
// sama bisa dipakai untuk kedua metode.
const madScale = 0.6745

// distribution merangkum satu kumpulan nilai dengan DUA cara sekaligus:
// mean/stddev yang lazim, dan median/MAD yang tahan outlier.
//
// Kenapa perlu keduanya: z-score klasik memakai mean & stddev yang keduanya
// ikut TERTARIK oleh outlier yang justru sedang dicari. Satu transaksi 100x
// lebih besar menaikkan stddev sedemikian rupa sehingga dirinya sendiri (dan
// outlier lain di sekitarnya) tidak lagi melewati ambang -- masking effect.
// Median/MAD tidak bergeser oleh beberapa nilai ekstrem, jadi yang aneh tetap
// terlihat aneh.
type distribution struct {
	Mean   float64
	StdDev float64
	Median float64
	MAD    float64
}

func describeValues(values []float64) distribution {
	mean, stddev := meanStdDev(values)
	median := medianOf(values)
	deviations := make([]float64, len(values))
	for i, v := range values {
		deviations[i] = math.Abs(v - median)
	}
	return distribution{Mean: mean, StdDev: stddev, Median: median, MAD: medianOf(deviations)}
}

// score mengembalikan z beserta metodenya. MAD = 0 terjadi kalau lebih dari
// separuh nilainya identik (mis. banyak order bernilai sama persis); di situ
// median/MAD tidak bisa membedakan apa pun dan perhitungan klasik yang dipakai.
func (d distribution) score(v float64) (z float64, method string) {
	if d.MAD > 0 {
		return madScale * (v - d.Median) / d.MAD, "modified"
	}
	if d.StdDev > 0 {
		return (v - d.Mean) / d.StdDev, "classic"
	}
	return 0, "classic"
}

// usable: kumpulan nilai yang seluruhnya identik tidak punya "yang aneh".
func (d distribution) usable() bool {
	return d.MAD > 0 || d.StdDev > 0
}

func medianOf(values []float64) float64 {
	n := len(values)
	if n == 0 {
		return 0
	}
	sorted := make([]float64, n)
	copy(sorted, values)
	sort.Float64s(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func (h *Handler) scanSalesOrders(companyID string, threshold float64) ([]anomaly, error) {
	var orders []struct {
		ID          string  `json:"id"`
		SONumber    string  `json:"so_number"`
		TotalAmount float64 `json:"total_amount"`
	}
	if err := h.getJSON(h.cfg.SalesServiceURL, "/sales-orders", companyID, &orders); err != nil {
		return nil, err
	}
	values := make([]float64, len(orders))
	for i, o := range orders {
		values[i] = o.TotalAmount
	}
	dist := describeValues(values)
	if !dist.usable() || len(orders) < 3 {
		return nil, nil
	}

	var flagged []anomaly
	for _, o := range orders {
		z, method := dist.score(o.TotalAmount)
		if math.Abs(z) >= threshold {
			flagged = append(flagged, anomaly{
				Source: "sales-service", EntityType: "sales_order", EntityID: o.ID,
				Label: o.SONumber, Value: o.TotalAmount, Mean: dist.Mean, StdDev: dist.StdDev, Median: dist.Median, MAD: dist.MAD, ZScore: z, Method: method,
				Reason: zReason(z, "Nilai sales order"),
			})
		}
	}
	return flagged, nil
}

func (h *Handler) scanPurchaseOrders(companyID string, threshold float64) ([]anomaly, error) {
	var orders []struct {
		ID          string  `json:"id"`
		PONumber    string  `json:"po_number"`
		TotalAmount float64 `json:"total_amount"`
	}
	if err := h.getJSON(h.cfg.PurchasingServiceURL, "/purchase-orders", companyID, &orders); err != nil {
		return nil, err
	}
	values := make([]float64, len(orders))
	for i, o := range orders {
		values[i] = o.TotalAmount
	}
	dist := describeValues(values)
	if !dist.usable() || len(orders) < 3 {
		return nil, nil
	}

	var flagged []anomaly
	for _, o := range orders {
		z, method := dist.score(o.TotalAmount)
		if math.Abs(z) >= threshold {
			flagged = append(flagged, anomaly{
				Source: "purchasing-service", EntityType: "purchase_order", EntityID: o.ID,
				Label: o.PONumber, Value: o.TotalAmount, Mean: dist.Mean, StdDev: dist.StdDev, Median: dist.Median, MAD: dist.MAD, ZScore: z, Method: method,
				Reason: zReason(z, "Nilai purchase order"),
			})
		}
	}
	return flagged, nil
}

func (h *Handler) scanStockMovements(companyID string, threshold float64) ([]anomaly, error) {
	var movements []struct {
		ID           string  `json:"id"`
		ProductName  string  `json:"product_name"`
		MovementType string  `json:"movement_type"`
		Quantity     float64 `json:"quantity"`
		MovementDate string  `json:"movement_date"`
	}
	if err := h.getJSON(h.cfg.WarehouseServiceURL, "/stock-movements", companyID, &movements); err != nil {
		return nil, err
	}
	values := make([]float64, len(movements))
	for i, m := range movements {
		values[i] = m.Quantity
	}
	dist := describeValues(values)
	if !dist.usable() || len(movements) < 3 {
		return nil, nil
	}

	var flagged []anomaly
	for _, m := range movements {
		z, method := dist.score(m.Quantity)
		if math.Abs(z) >= threshold {
			date := m.MovementDate
			if len(date) >= 10 {
				date = date[:10]
			}
			flagged = append(flagged, anomaly{
				Source: "warehouse-service", EntityType: "stock_movement", EntityID: m.ID,
				Label: fmt.Sprintf("%s (%s, %s)", m.ProductName, m.MovementType, date),
				Value: m.Quantity, Mean: dist.Mean, StdDev: dist.StdDev, Median: dist.Median, MAD: dist.MAD, ZScore: z, Method: method,
				Reason: zReason(z, "Kuantitas mutasi stok"),
			})
		}
	}
	return flagged, nil
}

func zReason(z float64, subject string) string {
	if z > 0 {
		return fmt.Sprintf("%s jauh di atas rata-rata historisnya", subject)
	}
	return fmt.Sprintf("%s jauh di bawah rata-rata historisnya", subject)
}
