package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
	"github.com/enterprise-digital-platform/dw-service/internal/etl"
	"github.com/enterprise-digital-platform/dw-service/internal/sourcedb"
)

type Handler struct {
	sources *sourcedb.Pools
	dest    *ch.Client
}

func NewHandler(sources *sourcedb.Pools, dest *ch.Client) *Handler {
	return &Handler{sources: sources, dest: dest}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("POST /sync", h.sync)
	mux.HandleFunc("GET /sync/status", h.syncStatus)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "dw-service"})
}

type syncResult struct {
	Fact  string `json:"fact"`
	Rows  int    `json:"rows_synced"`
	Error string `json:"error,omitempty"`
}

// RunSync menjalankan seluruh Sync ETL secara sinkron dan mengembalikan
// jumlah baris per fact -- dipakai baik oleh ticker background di
// cmd/server maupun endpoint POST /sync untuk trigger manual (tombol "Sync
// Now" di frontend, dan untuk verifikasi tanpa perlu menunggu ticker).
// Satu fact yang gagal tidak menghentikan fact lainnya -- errornya
// dilaporkan per-fact di hasil, bukan bikin seluruh sync gagal total.
func RunSync(ctx context.Context, sources *sourcedb.Pools, dest *ch.Client) []syncResult {
	results := make([]syncResult, 0, 9)

	if n, err := etl.SyncFinance(ctx, sources.Finance, dest); err != nil {
		results = append(results, syncResult{Fact: "finance_journal_lines", Error: err.Error()})
	} else {
		results = append(results, syncResult{Fact: "finance_journal_lines", Rows: n})
	}

	if n, err := etl.SyncSales(ctx, sources.Sales, dest); err != nil {
		results = append(results, syncResult{Fact: "sales_order_lines", Error: err.Error()})
	} else {
		results = append(results, syncResult{Fact: "sales_order_lines", Rows: n})
	}

	if n, err := etl.SyncInventory(ctx, sources.Warehouse, dest); err != nil {
		results = append(results, syncResult{Fact: "inventory_movements", Error: err.Error()})
	} else {
		results = append(results, syncResult{Fact: "inventory_movements", Rows: n})
	}

	if n, err := etl.SyncHR(ctx, sources.HR, dest); err != nil {
		results = append(results, syncResult{Fact: "hr_payroll_details", Error: err.Error()})
	} else {
		results = append(results, syncResult{Fact: "hr_payroll_details", Rows: n})
	}

	if n, err := etl.SyncPurchasing(ctx, sources.Purchasing, dest); err != nil {
		results = append(results, syncResult{Fact: "purchasing_order_lines", Error: err.Error()})
	} else {
		results = append(results, syncResult{Fact: "purchasing_order_lines", Rows: n})
	}

	if n, err := etl.SyncProduction(ctx, sources.Production, dest); err != nil {
		results = append(results, syncResult{Fact: "production_work_orders", Error: err.Error()})
	} else {
		results = append(results, syncResult{Fact: "production_work_orders", Rows: n})
	}

	if n, err := etl.SyncQC(ctx, sources.QC, dest); err != nil {
		results = append(results, syncResult{Fact: "qc_inspections", Error: err.Error()})
	} else {
		results = append(results, syncResult{Fact: "qc_inspections", Rows: n})
	}

	if n, err := etl.SyncAsset(ctx, sources.Asset, dest); err != nil {
		results = append(results, syncResult{Fact: "asset_maintenance", Error: err.Error()})
	} else {
		results = append(results, syncResult{Fact: "asset_maintenance", Rows: n})
	}

	if n, err := etl.SyncIoT(ctx, sources.IoT, dest); err != nil {
		results = append(results, syncResult{Fact: "iot_readings", Error: err.Error()})
	} else {
		results = append(results, syncResult{Fact: "iot_readings", Rows: n})
	}

	return results
}

func (h *Handler) sync(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	results := RunSync(r.Context(), h.sources, h.dest)
	writeJSON(w, http.StatusOK, results)
}

func (h *Handler) syncStatus(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	ctx := r.Context()
	facts := []struct {
		name  string
		table string
	}{
		{"finance_journal_lines", "fact_finance_journal_lines"},
		{"sales_order_lines", "fact_sales_order_lines"},
		{"inventory_movements", "fact_inventory_movements"},
		{"hr_payroll_details", "fact_hr_payroll_details"},
		{"purchasing_order_lines", "fact_purchasing_order_lines"},
		{"production_work_orders", "fact_production_work_orders"},
		{"qc_inspections", "fact_qc_inspections"},
		{"asset_maintenance", "fact_asset_maintenance"},
		{"iot_readings", "fact_iot_readings"},
	}

	type factStatus struct {
		Fact         string `json:"fact"`
		RowCount     uint64 `json:"row_count"`
		LastSyncedAt string `json:"last_synced_at,omitempty"`
	}

	status := make([]factStatus, 0, len(facts))
	for _, f := range facts {
		count, err := h.dest.CountRows(ctx, f.table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memuat status sync: "+err.Error())
			return
		}
		watermark, err := h.dest.GetWatermark(ctx, f.name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memuat watermark: "+err.Error())
			return
		}
		fs := factStatus{Fact: f.name, RowCount: count}
		if !watermark.IsZero() {
			fs.LastSyncedAt = watermark.Format("2006-01-02T15:04:05Z07:00")
		}
		status = append(status, fs)
	}
	writeJSON(w, http.StatusOK, status)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
