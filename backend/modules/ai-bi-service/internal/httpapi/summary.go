package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// dashboardSummary mengagregasi data dari 8 service lain lewat HTTP langsung
// (bukan lewat api-gateway, sama seperti pola financeclient/warehouseclient),
// dipanggil paralel supaya latensi total kira-kira sama dengan panggilan
// paling lambat, bukan jumlah semuanya. Kalau satu service gagal dihubungi,
// bagian dashboard itu dikosongkan (nilai nol) dan dicatat di field "errors"
// -- dashboard tetap tampil dengan data yang berhasil didapat, bukan gagal
// total, karena mengandalkan 8 service hidup bersamaan adalah asumsi yang
// rapuh untuk sebuah endpoint agregasi.
type dashboardResponse struct {
	CompanyID   string            `json:"company_id"`
	GeneratedAt time.Time         `json:"generated_at"`
	Sales       salesSummary      `json:"sales"`
	Purchasing  purchasingSummary `json:"purchasing"`
	Finance     financeSummary    `json:"finance"`
	Warehouse   warehouseSummary  `json:"warehouse"`
	Production  productionSummary `json:"production"`
	QC          qcSummary         `json:"qc"`
	HR          hrSummary         `json:"hr"`
	Asset       assetSummary      `json:"asset"`
	Errors      []sourceError     `json:"errors"`
}

type sourceError struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}

type salesSummary struct {
	TotalOrders  int            `json:"total_orders"`
	TotalRevenue float64        `json:"total_revenue"`
	ByStatus     map[string]int `json:"by_status"`
}

type purchasingSummary struct {
	TotalOrders int            `json:"total_orders"`
	TotalSpend  float64        `json:"total_spend"`
	ByStatus    map[string]int `json:"by_status"`
}

type financeSummary struct {
	ARTotal             float64 `json:"ar_total"`
	AROutstanding       float64 `json:"ar_outstanding"`
	APTotal             float64 `json:"ap_total"`
	APOutstanding       float64 `json:"ap_outstanding"`
	JournalEntriesCount int     `json:"journal_entries_count"`
}

type warehouseSummary struct {
	TotalProducts   int `json:"total_products"`
	TotalWarehouses int `json:"total_warehouses"`
	TotalStockLines int `json:"total_stock_lines"`
	LowStockCount   int `json:"low_stock_count"`
}

type productionSummary struct {
	TotalWorkOrders int            `json:"total_work_orders"`
	ByStatus        map[string]int `json:"by_status"`
}

type qcSummary struct {
	TotalInspections int     `json:"total_inspections"`
	PassCount        int     `json:"pass_count"`
	FailCount        int     `json:"fail_count"`
	PartialCount     int     `json:"partial_count"`
	PassRatePct      float64 `json:"pass_rate_pct"`
}

type hrSummary struct {
	TotalEmployees  int `json:"total_employees"`
	ActiveEmployees int `json:"active_employees"`
}

type assetSummary struct {
	TotalAssets             int `json:"total_assets"`
	ActiveAssets            int `json:"active_assets"`
	OverdueMaintenanceCount int `json:"overdue_maintenance_count"`
}

// lowStockThreshold adalah heuristik sementara (produk belum punya field
// minimum stock level tersendiri) supaya dashboard tetap bisa menunjukkan
// sinyal "stok menipis" tanpa menunggu fitur minimum-stock di Warehouse.
const lowStockThreshold = 10

func (h *Handler) dashboardSummary(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	resp := dashboardResponse{
		CompanyID:   companyID,
		GeneratedAt: time.Now(),
		Errors:      []sourceError{},
	}
	var mu sync.Mutex
	addErr := func(source string, err error) {
		mu.Lock()
		defer mu.Unlock()
		resp.Errors = append(resp.Errors, sourceError{Source: source, Message: err.Error()})
	}

	var wg sync.WaitGroup

	wg.Go(func() {
		s, err := h.fetchSalesSummary(companyID)
		if err != nil {
			addErr("sales-service", err)
			return
		}
		mu.Lock()
		resp.Sales = s
		mu.Unlock()
	})

	wg.Go(func() {
		s, err := h.fetchPurchasingSummary(companyID)
		if err != nil {
			addErr("purchasing-service", err)
			return
		}
		mu.Lock()
		resp.Purchasing = s
		mu.Unlock()
	})

	wg.Go(func() {
		s, err := h.fetchFinanceSummary(companyID)
		if err != nil {
			addErr("finance-service", err)
			return
		}
		mu.Lock()
		resp.Finance = s
		mu.Unlock()
	})

	wg.Go(func() {
		s, err := h.fetchWarehouseSummary(companyID)
		if err != nil {
			addErr("warehouse-service", err)
			return
		}
		mu.Lock()
		resp.Warehouse = s
		mu.Unlock()
	})

	wg.Go(func() {
		s, err := h.fetchProductionSummary(companyID)
		if err != nil {
			addErr("production-service", err)
			return
		}
		mu.Lock()
		resp.Production = s
		mu.Unlock()
	})

	wg.Go(func() {
		s, err := h.fetchQCSummary(companyID)
		if err != nil {
			addErr("qc-service", err)
			return
		}
		mu.Lock()
		resp.QC = s
		mu.Unlock()
	})

	wg.Go(func() {
		s, err := h.fetchHRSummary(companyID)
		if err != nil {
			addErr("hr-service", err)
			return
		}
		mu.Lock()
		resp.HR = s
		mu.Unlock()
	})

	wg.Go(func() {
		s, err := h.fetchAssetSummary(companyID)
		if err != nil {
			addErr("asset-service", err)
			return
		}
		mu.Lock()
		resp.Asset = s
		mu.Unlock()
	})

	wg.Wait()
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) getJSON(baseURL, path, companyID string, out any) error {
	url := fmt.Sprintf("%s%s?company_id=%s", baseURL, path, companyID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("returned %d: %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, out)
}

func (h *Handler) fetchSalesSummary(companyID string) (salesSummary, error) {
	var orders []struct {
		Status      string  `json:"status"`
		TotalAmount float64 `json:"total_amount"`
	}
	if err := h.getJSON(h.cfg.SalesServiceURL, "/sales-orders", companyID, &orders); err != nil {
		return salesSummary{}, err
	}
	s := salesSummary{ByStatus: map[string]int{}}
	for _, o := range orders {
		s.TotalOrders++
		s.TotalRevenue += o.TotalAmount
		s.ByStatus[o.Status]++
	}
	return s, nil
}

func (h *Handler) fetchPurchasingSummary(companyID string) (purchasingSummary, error) {
	var orders []struct {
		Status      string  `json:"status"`
		TotalAmount float64 `json:"total_amount"`
	}
	if err := h.getJSON(h.cfg.PurchasingServiceURL, "/purchase-orders", companyID, &orders); err != nil {
		return purchasingSummary{}, err
	}
	s := purchasingSummary{ByStatus: map[string]int{}}
	for _, o := range orders {
		s.TotalOrders++
		s.TotalSpend += o.TotalAmount
		s.ByStatus[o.Status]++
	}
	return s, nil
}

func (h *Handler) fetchFinanceSummary(companyID string) (financeSummary, error) {
	var arAp []struct {
		InvoiceType       string  `json:"invoice_type"`
		TotalAmount       float64 `json:"total_amount"`
		OutstandingAmount float64 `json:"outstanding_amount"`
	}
	if err := h.getJSON(h.cfg.FinanceServiceURL, "/ar-ap-summary", companyID, &arAp); err != nil {
		return financeSummary{}, err
	}
	var journalEntries []struct {
		ID string `json:"id"`
	}
	if err := h.getJSON(h.cfg.FinanceServiceURL, "/journal-entries", companyID, &journalEntries); err != nil {
		return financeSummary{}, err
	}

	s := financeSummary{JournalEntriesCount: len(journalEntries)}
	for _, row := range arAp {
		switch row.InvoiceType {
		case "AR":
			s.ARTotal += row.TotalAmount
			s.AROutstanding += row.OutstandingAmount
		case "AP":
			s.APTotal += row.TotalAmount
			s.APOutstanding += row.OutstandingAmount
		}
	}
	return s, nil
}

func (h *Handler) fetchWarehouseSummary(companyID string) (warehouseSummary, error) {
	var products []struct {
		ID string `json:"id"`
	}
	if err := h.getJSON(h.cfg.WarehouseServiceURL, "/products", companyID, &products); err != nil {
		return warehouseSummary{}, err
	}
	var warehouses []struct {
		ID string `json:"id"`
	}
	if err := h.getJSON(h.cfg.WarehouseServiceURL, "/warehouses", companyID, &warehouses); err != nil {
		return warehouseSummary{}, err
	}
	var stock []struct {
		Quantity float64 `json:"quantity"`
	}
	if err := h.getJSON(h.cfg.WarehouseServiceURL, "/stock", companyID, &stock); err != nil {
		return warehouseSummary{}, err
	}

	s := warehouseSummary{
		TotalProducts:   len(products),
		TotalWarehouses: len(warehouses),
		TotalStockLines: len(stock),
	}
	for _, line := range stock {
		if line.Quantity < lowStockThreshold {
			s.LowStockCount++
		}
	}
	return s, nil
}

func (h *Handler) fetchProductionSummary(companyID string) (productionSummary, error) {
	var orders []struct {
		Status string `json:"status"`
	}
	if err := h.getJSON(h.cfg.ProductionServiceURL, "/work-orders", companyID, &orders); err != nil {
		return productionSummary{}, err
	}
	s := productionSummary{ByStatus: map[string]int{}}
	for _, o := range orders {
		s.TotalWorkOrders++
		s.ByStatus[o.Status]++
	}
	return s, nil
}

func (h *Handler) fetchQCSummary(companyID string) (qcSummary, error) {
	var inspections []struct {
		Result string `json:"result"`
	}
	if err := h.getJSON(h.cfg.QCServiceURL, "/inspections", companyID, &inspections); err != nil {
		return qcSummary{}, err
	}
	s := qcSummary{}
	for _, insp := range inspections {
		s.TotalInspections++
		switch insp.Result {
		case "PASS":
			s.PassCount++
		case "FAIL":
			s.FailCount++
		case "PARTIAL":
			s.PartialCount++
		}
	}
	if s.TotalInspections > 0 {
		s.PassRatePct = float64(s.PassCount) / float64(s.TotalInspections) * 100
	}
	return s, nil
}

func (h *Handler) fetchHRSummary(companyID string) (hrSummary, error) {
	var employees []struct {
		IsActive bool `json:"is_active"`
	}
	if err := h.getJSON(h.cfg.HRServiceURL, "/employees", companyID, &employees); err != nil {
		return hrSummary{}, err
	}
	s := hrSummary{TotalEmployees: len(employees)}
	for _, e := range employees {
		if e.IsActive {
			s.ActiveEmployees++
		}
	}
	return s, nil
}

func (h *Handler) fetchAssetSummary(companyID string) (assetSummary, error) {
	var assets []struct {
		Status string `json:"status"`
	}
	if err := h.getJSON(h.cfg.AssetServiceURL, "/assets", companyID, &assets); err != nil {
		return assetSummary{}, err
	}
	var schedules []struct {
		Status        string `json:"status"`
		ScheduledDate string `json:"scheduled_date"`
	}
	if err := h.getJSON(h.cfg.AssetServiceURL, "/maintenance-schedules", companyID, &schedules); err != nil {
		return assetSummary{}, err
	}

	s := assetSummary{TotalAssets: len(assets)}
	for _, a := range assets {
		if a.Status == "ACTIVE" {
			s.ActiveAssets++
		}
	}
	today := time.Now().Format("2006-01-02")
	for _, sched := range schedules {
		if sched.Status == "SCHEDULED" && sched.ScheduledDate[:10] < today {
			s.OverdueMaintenanceCount++
		}
	}
	return s, nil
}
