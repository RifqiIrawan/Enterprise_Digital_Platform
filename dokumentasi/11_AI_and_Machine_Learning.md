# 11 — AI & Machine Learning
## Enterprise Digital Platform (EDP)

---

## Overview

`ai-bi-service` (port 8093) menyediakan **tiga kapabilitas analitik** yang diimplementasikan dalam Go polos — tanpa Python, tanpa MLflow, tanpa JupyterHub, tanpa BentoML:

1. **BI Dashboards** — agregasi metrik bisnis dari 8 service
2. **Forecasting** — proyeksi tren linear (regresi linear sederhana)
3. **Anomaly Detection** — deteksi nilai outlier (z-score heuristik)

---

## Arsitektur ai-bi-service

```
ai-bi-service (port 8093)
    │
    ├── GET /dashboards/summary
    │   └── Agregasi HTTP paralel ke 8 service:
    │       finance-service, hr-service, sales-service, purchasing-service,
    │       warehouse-service, production-service, qc-service, asset-service
    │
    ├── GET /forecasting/{metric}
    │   └── Ambil data historis → regresi linear → return proyeksi N periode ke depan
    │
    └── GET /anomaly-detection/{domain}
        └── Ambil data → hitung mean & stddev → flag z-score > threshold
```

**Tidak punya database sendiri** — murni agregasi HTTP dari service lain. Tidak ada Kafka consumer, tidak ada Postgres connection.

---

## BI Dashboards

Endpoint: `GET /dashboards/summary?company_id=<uuid>`

Menjalankan beberapa HTTP request secara paralel ke service lain, mengumpulkan metrik kunci:

| Metrik | Source Service | Data yang diambil |
|--------|---------------|-------------------|
| `total_revenue` | sales-service | SUM(amount) SO yang FULFILLED/INVOICED |
| `total_expenses` | purchasing-service | SUM(amount) PO yang RECEIVED/INVOICED |
| `total_employees` | hr-service | COUNT(employees) ACTIVE |
| `total_assets` | asset-service | COUNT(assets) ACTIVE |
| `open_alerts` | iot-service | COUNT(alerts) status=OPEN |
| `pending_wo` | production-service | COUNT(work_orders) IN_PROGRESS |
| `pending_inspections` | qc-service | COUNT(inspections) baru |
| `stock_value` | warehouse-service | SUM(quantity * estimated_value) |

Response juga menyertakan `errors[]` kalau salah satu service tidak tersedia — partial result lebih baik dari no result.

---

## Forecasting

Endpoint: `GET /forecasting/{metric}?company_id=<uuid>&periods=6`

**Algoritma**: Regresi linear sederhana (Ordinary Least Squares)

```go
// Input: series []DataPoint{Period, Value}
// Output: []ForecastPoint{Period, Projected, LowerBound, UpperBound}

func linearRegression(series []DataPoint) (slope, intercept float64) {
    n := float64(len(series))
    sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0
    for i, p := range series {
        x := float64(i)
        sumX += x; sumY += p.Value
        sumXY += x * p.Value; sumX2 += x * x
    }
    slope = (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
    intercept = (sumY - slope*sumX) / n
    return
}
```

Metric yang didukung: `sales_revenue`, `purchase_cost`, `payroll_cost`, `production_output`

---

## Anomaly Detection

Endpoint: `GET /anomaly-detection/{domain}?company_id=<uuid>`

**Algoritma**: Z-score heuristik

```go
// Untuk setiap data point:
// zscore = |value - mean| / stddev
// Kalau zscore > threshold (default 2.0) → flag sebagai anomali

func detectAnomalies(data []DataPoint, threshold float64) []Anomaly {
    mean, stddev := calculateStats(data)
    var anomalies []Anomaly
    for _, p := range data {
        z := math.Abs(p.Value-mean) / stddev
        if z > threshold {
            anomalies = append(anomalies, Anomaly{
                Point:    p,
                ZScore:   z,
                Severity: classifySeverity(z),
            })
        }
    }
    return anomalies
}
```

Domain yang didukung: `sales_orders`, `stock_movements`, `payroll`, `purchase_orders`

---

## Frontend Integration

| Halaman | Endpoint yang dipanggil |
|---------|------------------------|
| `BIDashboardsPage.jsx` | `GET /api/aibi/dashboards/summary` |
| `ForecastingPage.jsx` | `GET /api/aibi/forecasting/{metric}` |
| `AnomalyDetectionPage.jsx` | `GET /api/aibi/anomaly-detection/{domain}` |

Semua lewat api-gateway dengan prefix `/api/aibi/`.

---

## Keterbatasan & Roadmap

**Saat ini**:
- Regresi linear sederhana — akurat untuk data dengan tren konstan, tidak untuk seasonal/cyclical
- Z-score sederhana — tidak ada adaptive threshold, tidak ada window function
- Tidak ada model training/versioning
- Tidak ada batch prediction storage

**Ekstensi potensial** (belum diimplementasikan):
- Query langsung ke ClickHouse fact tables (lebih efisien dari HTTP ke Postgres per-service)
- Seasonal decomposition untuk forecasting
- Adaptive z-score dengan rolling window
- Export hasil ke MinIO lake untuk audit trail prediksi
