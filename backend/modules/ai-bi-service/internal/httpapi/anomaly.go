package httpapi

import (
	"fmt"
	"math"
	"net/http"
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
	ZScore     float64 `json:"z_score"`
	Reason     string  `json:"reason"`
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
	mean, stddev := meanStdDev(values)
	if stddev == 0 || len(orders) < 3 {
		return nil, nil
	}

	var flagged []anomaly
	for _, o := range orders {
		z := (o.TotalAmount - mean) / stddev
		if math.Abs(z) >= threshold {
			flagged = append(flagged, anomaly{
				Source: "sales-service", EntityType: "sales_order", EntityID: o.ID,
				Label: o.SONumber, Value: o.TotalAmount, Mean: mean, StdDev: stddev, ZScore: z,
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
	mean, stddev := meanStdDev(values)
	if stddev == 0 || len(orders) < 3 {
		return nil, nil
	}

	var flagged []anomaly
	for _, o := range orders {
		z := (o.TotalAmount - mean) / stddev
		if math.Abs(z) >= threshold {
			flagged = append(flagged, anomaly{
				Source: "purchasing-service", EntityType: "purchase_order", EntityID: o.ID,
				Label: o.PONumber, Value: o.TotalAmount, Mean: mean, StdDev: stddev, ZScore: z,
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
	mean, stddev := meanStdDev(values)
	if stddev == 0 || len(movements) < 3 {
		return nil, nil
	}

	var flagged []anomaly
	for _, m := range movements {
		z := (m.Quantity - mean) / stddev
		if math.Abs(z) >= threshold {
			date := m.MovementDate
			if len(date) >= 10 {
				date = date[:10]
			}
			flagged = append(flagged, anomaly{
				Source: "warehouse-service", EntityType: "stock_movement", EntityID: m.ID,
				Label: fmt.Sprintf("%s (%s, %s)", m.ProductName, m.MovementType, date),
				Value: m.Quantity, Mean: mean, StdDev: stddev, ZScore: z,
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
