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

func (h *Handler) fleetDeliveryMonthlySummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.FleetDeliveryMonthlySummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan pengiriman: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) hrLeaveMonthlySummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.HRLeaveMonthlySummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan cuti: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) hrKPISummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.HRKPISummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan KPI: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// hrKPIDepartmentSummary: parameter period opsional. Kalau kosong, dipakai
// periode terakhir yang punya penilaian disetujui -- itu yang hampir selalu
// diinginkan pembuka dashboard, dan menghindarkan UI menebak sendiri periode
// mana yang sudah final.
func (h *Handler) hrKPIDepartmentSummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period, err = h.dest.LatestKPIPeriod(r.Context(), companyID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal menentukan periode KPI terakhir: "+err.Error())
			return
		}
		if period == "" {
			// Belum ada penilaian yang disetujui sama sekali: daftar kosong,
			// bukan error -- dashboard baru memang belum punya data.
			writeJSON(w, http.StatusOK, []any{})
			return
		}
	}

	rows, err := h.dest.HRKPIDepartmentSummary(r.Context(), companyID, period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan KPI per departemen: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) qcMonthlySummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.QCMonthlySummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan QC: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) productionMonthlySummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.ProductionMonthlySummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan produksi: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) purchasingSupplierSummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.PurchasingSupplierSummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan belanja per supplier: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) ticketingMonthlySummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.TicketingMonthlySummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan tiket: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) payrollPeriodSummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.PayrollPeriodSummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan payroll: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) assetMaintenanceSummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.AssetMaintenanceSummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan perawatan aset: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) iotDeviceSummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.IoTDeviceSummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan sensor IoT: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) ecommerceMonthlySummary(w http.ResponseWriter, r *http.Request) {
	if h.dest == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse tidak tersedia")
		return
	}
	companyID, err := uuid.Parse(r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Parameter company_id wajib berupa UUID valid")
		return
	}
	rows, err := h.dest.EcommerceMonthlySummary(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan penjualan online: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
