package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
	"github.com/enterprise-digital-platform/dw-service/internal/etl"
	"github.com/enterprise-digital-platform/dw-service/internal/sourcedb"
)

// lakeBuild membangun ulang Silver semua fact lalu Gold. Sinkron, seperti
// POST /sync: dia membaca seluruh Bronze, jadi lama kalau Bronze besar, dan
// itu keadaan yang mau dilihat pemanggilnya, bukan disembunyikan di latar.
func (h *Handler) lakeBuild(w http.ResponseWriter, r *http.Request) {
	if h.lake == nil {
		writeError(w, http.StatusServiceUnavailable, "Data lake (MinIO) tidak tersedia")
		return
	}
	res := h.lake.BuildAll(r.Context(), liveKeyFuncs(h.sources))
	status := http.StatusOK
	if len(res.Errors) > 0 {
		// 207 bukan 500: sebagian fact berhasil dibangun, dan hasil per-fact
		// tetap ada di badan respons.
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, res)
}

// liveKeyFuncs menyiapkan, untuk tiap fact, cara menanyakan ke sumber baris mana
// yang masih ada -- dipakai Silver untuk memangkas baris yang dihapus di sumber.
// sources nil (mis. di test tanpa Postgres) berarti tidak ada pemangkasan.
func liveKeyFuncs(sources *sourcedb.Pools) map[string]datalake.LiveKeys {
	if sources == nil {
		return nil
	}
	out := make(map[string]datalake.LiveKeys, len(etl.Facts))
	for _, f := range etl.Facts {
		f := f
		out[f.Name] = func(ctx context.Context) (map[string]struct{}, error) {
			return f.LiveKeys(ctx, f.Source(sources))
		}
	}
	return out
}

var goldDatasets = map[string]string{
	"finance-monthly": datalake.GoldFinanceMonthly,
	"sales-monthly":   datalake.GoldSalesMonthly,
}

// lakeGold mengembalikan satu dataset Gold untuk SATU company. company_id
// wajib, sama dengan endpoint /analytics/*: berkas Gold memuat semua
// company, dan gateway menegakkan hak akses berdasarkan company_id di query.
func (h *Handler) lakeGold(w http.ResponseWriter, r *http.Request) {
	if h.lake == nil {
		writeError(w, http.StatusServiceUnavailable, "Data lake (MinIO) tidak tersedia")
		return
	}
	dataset, ok := goldDatasets[r.PathValue("dataset")]
	if !ok {
		writeError(w, http.StatusNotFound, "Dataset Gold tidak dikenal")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	lines, err := h.lake.ReadGold(r.Context(), dataset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca Gold: "+err.Error())
		return
	}
	out := []json.RawMessage{}
	for _, l := range lines {
		var probe struct {
			CompanyID string `json:"company_id"`
		}
		if json.Unmarshal(l, &probe) == nil && probe.CompanyID == companyID.String() {
			out = append(out, json.RawMessage(l))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// lakeBackfill menulis seluruh isi tabel sumber ke Bronze. Ini menutup celah
// baris yang sudah tersalin ke ClickHouse sebelum lake ada (atau saat penulisan
// ke lake gagal): watermark membuat ETL biasa tidak pernah membacanya lagi.
// Setelah ini jalankan POST /lake/build lalu GET /lake/reconcile.
func (h *Handler) lakeBackfill(w http.ResponseWriter, r *http.Request) {
	if h.lake == nil {
		writeError(w, http.StatusServiceUnavailable, "Data lake (MinIO) tidak tersedia")
		return
	}
	results := etl.BackfillAll(r.Context(), h.sources, h.lake)
	status := http.StatusOK
	for _, res := range results {
		if res.Error != "" {
			status = http.StatusMultiStatus
			break
		}
	}
	writeJSON(w, status, results)
}

type reconcileFact struct {
	Fact           string `json:"fact"`
	SilverRows     int    `json:"silver_rows"`
	ClickHouseRows uint64 `json:"clickhouse_rows"`
	// Difference = ClickHouse - Silver. Positif: ada baris di ClickHouse yang
	// tidak ada di lake (jalankan backfill). Negatif: lake punya baris yang
	// tidak ada di ClickHouse (selidiki; bukan sesuatu yang diperbaiki otomatis).
	Difference int64  `json:"difference"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}

const (
	reconcileMatch   = "MATCH"
	reconcileMissing = "MISSING_FROM_LAKE"
	reconcileExtra   = "EXTRA_IN_LAKE"
	reconcileError   = "ERROR"
)

// lakeReconcile membandingkan jumlah baris Silver dengan baris ClickHouse yang
// berlaku (FINAL) untuk tiap fact. Ini pemeriksaan HITUNGAN, bukan isi: dua
// selisih yang saling meniadakan (lake kelebihan satu baris, kekurangan satu
// baris lain) lolos sebagai MATCH. Silver yang usang juga terbaca sebagai
// selisih, jadi jalankan POST /lake/build dulu kalau baru ada sync atau backfill.
func (h *Handler) lakeReconcile(w http.ResponseWriter, r *http.Request) {
	if h.lake == nil {
		writeError(w, http.StatusServiceUnavailable, "Data lake (MinIO) tidak tersedia")
		return
	}
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	consistent, facts := reconcileAll(r.Context(), h.dest, h.lake)
	writeJSON(w, http.StatusOK, map[string]any{"consistent": consistent, "facts": facts})
}

// reconcileAll membandingkan Silver dengan ClickHouse untuk semua fact. Dipakai
// handler GET /lake/reconcile dan job berkala (RunLakeCycle), supaya keduanya
// tidak bisa berselisih soal apa artinya "cocok".
func reconcileAll(ctx context.Context, dest *ch.Client, lake *datalake.Client) (bool, []reconcileFact) {
	facts := make([]reconcileFact, 0, len(etl.Facts))
	consistent := true
	for _, f := range etl.Facts {
		rf := reconcileFact{Fact: f.Name}
		silver, err := lake.ReadSilver(ctx, f.Name)
		if err != nil {
			rf.Status, rf.Error = reconcileError, err.Error()
		} else if n, err := dest.CountCurrentRows(ctx, f.Table); err != nil {
			rf.Status, rf.Error = reconcileError, err.Error()
		} else {
			rf.SilverRows, rf.ClickHouseRows = len(silver), n
			rf.Difference, rf.Status = classifyReconcile(len(silver), n)
		}
		if rf.Status != reconcileMatch {
			consistent = false
		}
		facts = append(facts, rf)
	}
	return consistent, facts
}

// classifyReconcile mengubah dua hitungan menjadi selisih (ClickHouse - Silver)
// dan statusnya.
func classifyReconcile(silverRows int, clickhouseRows uint64) (int64, string) {
	diff := int64(clickhouseRows) - int64(silverRows)
	switch {
	case diff == 0:
		return 0, reconcileMatch
	case diff > 0:
		return diff, reconcileMissing
	default:
		return diff, reconcileExtra
	}
}
