package httpapi

import (
	"net/http"

	"github.com/google/uuid"
)

func (h *Handler) financeMonthlySummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.MonthlyFinanceSummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan finance: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) stockMovementMonthlySummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.MonthlyStockMovementSummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan pergerakan stok: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) crmPipelineSummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.CRMPipelineSummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan pipeline CRM: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) salesMonthlySummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.MonthlySalesSummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan sales: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) projectCostSummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.ProjectCostSummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan biaya proyek: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
