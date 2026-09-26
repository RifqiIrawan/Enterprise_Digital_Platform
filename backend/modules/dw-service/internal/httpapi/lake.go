package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
)

// lakeBuild membangun ulang Silver semua fact lalu Gold. Sinkron, seperti
// POST /sync: dia membaca seluruh Bronze, jadi lama kalau Bronze besar, dan
// itu keadaan yang mau dilihat pemanggilnya, bukan disembunyikan di latar.
func (h *Handler) lakeBuild(w http.ResponseWriter, r *http.Request) {
	if h.lake == nil {
		writeError(w, http.StatusServiceUnavailable, "Data lake (MinIO) tidak tersedia")
		return
	}
	res := h.lake.BuildAll(r.Context())
	status := http.StatusOK
	if len(res.Errors) > 0 {
		// 207 bukan 500: sebagian fact berhasil dibangun, dan hasil per-fact
		// tetap ada di badan respons.
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, res)
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
