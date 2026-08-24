package httpapi

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Recommendation engine (Fase 10) -- rekomendasi pemesanan ulang barang.
//
// Pertanyaannya sederhana dan sering: barang mana yang akan habis lebih dulu,
// dan berapa banyak yang perlu dipesan? Jawabannya dihitung dari dua hal yang
// sudah dicatat warehouse-service: saldo stok saat ini dan pergerakan stok
// keluar belakangan ini.
//
//	kecepatan pemakaian = total stok keluar / rentang hari yang tercakup
//	sisa hari           = stok saat ini / kecepatan pemakaian
//	saran pesan         = (kecepatan x target hari) - stok saat ini
//
// TIGA HAL YANG SENGAJA DIBATASI, dan disebutkan di respons supaya tidak
// dikira lebih pintar daripada yang sebenarnya:
//
//  1. GET /stock-movements di warehouse-service mengembalikan 200 pergerakan
//     TERAKHIR. Untuk company yang ramai, itu bisa berarti beberapa hari saja.
//     Rentangnya karena itu dihitung dari data yang benar-benar diterima
//     (bukan diasumsikan 30 hari), dan field `truncated` memberi tahu kalau
//     batas 200 itu memang tersentuh.
//  2. Tidak ada musiman, tren, maupun lead time pemasok. Ini rata-rata datar.
//     Barang yang pemakaiannya musiman akan disarankan terlalu sedikit
//     menjelang puncaknya -- dan itu lebih baik dinyatakan di sini daripada
//     ditutupi rumus yang datanya belum cukup untuk mendukungnya.
//  3. Barang yang tidak punya pergerakan keluar sama sekali TIDAK
//     direkomendasikan. Tanpa pemakaian, tidak ada dasar menghitung kapan dia
//     habis; stok mati bukan urusan endpoint ini.

// Target hari persediaan yang ingin dipertahankan saat menyarankan jumlah
// pesanan, dan ambang mendesak/menengah untuk sisa hari.
const (
	coverTargetDays    = 30.0
	urgentCoverDays    = 7.0
	mediumCoverDays    = 14.0
	stockMovementCap   = 200 // LIMIT di warehouse-service; dipakai mendeteksi data terpotong
	maxRecommendations = 50
)

type reorderWindow struct {
	From                string `json:"from"`
	To                  string `json:"to"`
	Days                int    `json:"days"`
	MovementsConsidered int    `json:"movements_considered"`
	// Truncated menandai bahwa daftar pergerakan menyentuh batas 200 baris,
	// jadi rentang di atas kemungkinan lebih pendek daripada riwayat nyata.
	Truncated bool `json:"truncated"`
}

type reorderItem struct {
	ProductID     string  `json:"product_id"`
	ProductSKU    string  `json:"product_sku"`
	ProductName   string  `json:"product_name"`
	ProductUnit   string  `json:"product_unit"`
	OnHand        float64 `json:"on_hand"`
	OutQuantity   float64 `json:"out_quantity"`
	DailyVelocity float64 `json:"daily_velocity"`
	DaysOfCover   float64 `json:"days_of_cover"`
	SuggestedQty  float64 `json:"suggested_reorder_qty"`
	Urgency       string  `json:"urgency"`
	Reason        string  `json:"reason"`
}

type reorderResponse struct {
	CompanyID   string        `json:"company_id"`
	GeneratedAt time.Time     `json:"generated_at"`
	Window      reorderWindow `json:"window"`
	Items       []reorderItem `json:"items"`
	Errors      []sourceError `json:"errors"`
}

type stockBalanceView struct {
	ProductID   string  `json:"product_id"`
	Quantity    float64 `json:"quantity"`
	ProductSKU  string  `json:"product_sku"`
	ProductName string  `json:"product_name"`
	ProductUnit string  `json:"product_unit"`
}

type stockMovementView struct {
	ProductID    string  `json:"product_id"`
	MovementType string  `json:"movement_type"`
	Quantity     float64 `json:"quantity"`
	MovementDate string  `json:"movement_date"`
	ProductSKU   string  `json:"product_sku"`
	ProductName  string  `json:"product_name"`
}

func (h *Handler) reorderRecommendations(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	resp := reorderResponse{
		CompanyID:   companyID,
		GeneratedAt: time.Now(),
		Items:       []reorderItem{},
		Errors:      []sourceError{},
	}
	var mu sync.Mutex
	addErr := func(source string, err error) {
		mu.Lock()
		defer mu.Unlock()
		resp.Errors = append(resp.Errors, sourceError{Source: source, Message: err.Error()})
	}

	var (
		balances  []stockBalanceView
		movements []stockMovementView
	)
	var wg sync.WaitGroup
	wg.Go(func() {
		if err := h.getJSON(h.cfg.WarehouseServiceURL, "/stock", companyID, &balances); err != nil {
			addErr("warehouse-service", err)
		}
	})
	wg.Go(func() {
		if err := h.getJSON(h.cfg.WarehouseServiceURL, "/stock-movements", companyID, &movements); err != nil {
			addErr("warehouse-service", err)
		}
	})
	wg.Wait()

	resp.Window, resp.Items = buildReorderRecommendations(balances, movements)
	writeJSON(w, http.StatusOK, resp)
}

// buildReorderRecommendations dipisahkan dari handler-nya supaya perhitungan
// ini bisa diuji sebagai fungsi murni, tanpa HTTP di tengahnya.
func buildReorderRecommendations(balances []stockBalanceView, movements []stockMovementView) (reorderWindow, []reorderItem) {
	onHand := map[string]float64{}
	names := map[string]stockBalanceView{}
	for _, b := range balances {
		onHand[b.ProductID] += b.Quantity
		if _, ok := names[b.ProductID]; !ok {
			names[b.ProductID] = b
		}
	}

	outQty := map[string]float64{}
	movedNames := map[string]stockMovementView{}
	var earliest, latest time.Time
	for _, m := range movements {
		if d, err := time.Parse("2006-01-02", firstTen(m.MovementDate)); err == nil {
			if earliest.IsZero() || d.Before(earliest) {
				earliest = d
			}
			if latest.IsZero() || d.After(latest) {
				latest = d
			}
		}
		if m.MovementType != "OUT" {
			continue
		}
		outQty[m.ProductID] += m.Quantity
		if _, ok := movedNames[m.ProductID]; !ok {
			movedNames[m.ProductID] = m
		}
	}

	window := reorderWindow{
		MovementsConsidered: len(movements),
		Truncated:           len(movements) >= stockMovementCap,
	}
	// Satu hari data tetap satu hari, bukan nol -- pembagian dengan rentang
	// nol adalah cara termudah menghasilkan "kecepatan tak terhingga".
	days := 1
	if !earliest.IsZero() && !latest.IsZero() {
		window.From = earliest.Format("2006-01-02")
		window.To = latest.Format("2006-01-02")
		if d := int(latest.Sub(earliest).Hours()/24) + 1; d > 1 {
			days = d
		}
	}
	window.Days = days

	items := []reorderItem{}
	for productID, out := range outQty {
		if out <= 0 {
			continue
		}
		velocity := out / float64(days)
		stock := onHand[productID]
		// Saldo stok bisa MINUS: warehouse-service tidak menghalangi
		// pengeluaran yang melebihi saldo (mis. work order yang mengonsumsi
		// komponen yang belum tercatat masuk). "Cukup untuk -35 hari" bukan
		// kalimat yang berarti apa pun, jadi sisa harinya dibulatkan ke nol --
		// barangnya memang sudah habis, dan minusnya diberitahukan tersendiri
		// di `reason` karena itu sinyal yang layak dibaca orang gudang.
		cover := 0.0
		if stock > 0 {
			cover = stock / velocity
		}
		if cover >= coverTargetDays {
			continue
		}

		info := names[productID]
		if info.ProductSKU == "" {
			// Barang yang saldonya sudah nol tidak muncul di /stock, jadi
			// identitasnya diambil dari baris pergerakannya.
			moved := movedNames[productID]
			info.ProductSKU, info.ProductName = moved.ProductSKU, moved.ProductName
		}

		suggested := math.Ceil(velocity*coverTargetDays - stock)
		if suggested < 1 {
			suggested = 1
		}

		urgency := "LOW"
		switch {
		case cover < urgentCoverDays:
			urgency = "HIGH"
		case cover < mediumCoverDays:
			urgency = "MEDIUM"
		}

		items = append(items, reorderItem{
			ProductID:     productID,
			ProductSKU:    info.ProductSKU,
			ProductName:   info.ProductName,
			ProductUnit:   info.ProductUnit,
			OnHand:        round2(stock),
			OutQuantity:   round2(out),
			DailyVelocity: round2(velocity),
			DaysOfCover:   round1(cover),
			SuggestedQty:  suggested,
			Urgency:       urgency,
			Reason:        reorderReason(out, days, stock, cover),
		})
	}

	// Yang paling cepat habis lebih dulu; SKU sebagai pemecah seri supaya
	// urutannya stabil antar panggilan.
	sort.Slice(items, func(i, j int) bool {
		if items[i].DaysOfCover != items[j].DaysOfCover {
			return items[i].DaysOfCover < items[j].DaysOfCover
		}
		return items[i].ProductSKU < items[j].ProductSKU
	})
	if len(items) > maxRecommendations {
		items = items[:maxRecommendations]
	}
	return window, items
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// formatQty menulis angka apa adanya tanpa nol berekor: "12" bukan "12.00",
// "12.5" bukan "12.50" -- ini teks untuk dibaca orang.
func formatQty(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// reorderReason menjelaskan dasar sebuah saran dalam satu kalimat. Saldo minus
// disebut apa adanya: itu berarti ada pengeluaran yang melebihi stok tercatat,
// dan menyembunyikannya di balik "0 hari" menghilangkan satu-satunya petunjuk
// bahwa pencatatannya perlu diperiksa.
func reorderReason(out float64, days int, stock, cover float64) string {
	base := "Keluar " + formatQty(out) + " dalam " + itoa(days) + " hari; "
	if stock < 0 {
		return base + "stok tercatat MINUS " + formatQty(-stock) + " (pengeluaran melebihi saldo)"
	}
	if stock == 0 {
		return base + "stok sudah habis"
	}
	return base + "sisa stok cukup untuk " + formatQty(round1(cover)) + " hari"
}
