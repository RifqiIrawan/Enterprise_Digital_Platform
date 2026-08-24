package httpapi_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

type reorderItemView struct {
	ProductID     string  `json:"product_id"`
	ProductSKU    string  `json:"product_sku"`
	ProductName   string  `json:"product_name"`
	OnHand        float64 `json:"on_hand"`
	OutQuantity   float64 `json:"out_quantity"`
	DailyVelocity float64 `json:"daily_velocity"`
	DaysOfCover   float64 `json:"days_of_cover"`
	SuggestedQty  float64 `json:"suggested_reorder_qty"`
	Urgency       string  `json:"urgency"`
	Reason        string  `json:"reason"`
}

type reorderView struct {
	Window struct {
		From                string `json:"from"`
		To                  string `json:"to"`
		Days                int    `json:"days"`
		MovementsConsidered int    `json:"movements_considered"`
		Truncated           bool   `json:"truncated"`
	} `json:"window"`
	Items  []reorderItemView `json:"items"`
	Errors []struct {
		Source string `json:"source"`
	} `json:"errors"`
}

func (v reorderView) bySKU(sku string) (reorderItemView, bool) {
	for _, item := range v.Items {
		if item.ProductSKU == sku {
			return item, true
		}
	}
	return reorderItemView{}, false
}

func movement(productID, sku, movementType string, qty float64, date string) map[string]any {
	return map[string]any{
		"product_id": productID, "product_sku": sku, "product_name": "Produk " + sku,
		"movement_type": movementType, "quantity": qty, "movement_date": date,
	}
}

func balance(productID, sku string, qty float64) map[string]any {
	return map[string]any{
		"product_id": productID, "quantity": qty,
		"product_sku": sku, "product_name": "Produk " + sku, "product_unit": "pcs",
	}
}

func TestReorderRecommendations_MissingCompanyID(t *testing.T) {
	srv, _ := newServer(t)
	requireStatus(t, getJSON(t, srv.URL+"/recommendations/reorder"), http.StatusBadRequest)
}

func TestReorderRecommendations_VelocityCoverAndSuggestion(t *testing.T) {
	srv, backends := newServer(t)
	companyID := newCompanyID(t)

	// Rentang 1-10 Agustus = 10 hari.
	backends.warehouse.json("/stock-movements", http.StatusOK, []map[string]any{
		movement("p-a", "SKU-A", "OUT", 100, "2026-08-01"),
		movement("p-a", "SKU-A", "IN", 500, "2026-08-05"), // stok masuk tidak boleh mengurangi pemakaian
		movement("p-b", "SKU-B", "OUT", 20, "2026-08-10"),
		movement("p-c", "SKU-C", "OUT", 30, "2026-08-03"),
		movement("p-e", "SKU-E", "OUT", 15, "2026-08-09"),
	})
	backends.warehouse.json("/stock", http.StatusOK, []map[string]any{
		balance("p-a", "SKU-A", 30),
		balance("p-a", "SKU-A", 20), // dua gudang, dijumlahkan jadi 50
		balance("p-b", "SKU-B", 100),
		balance("p-c", "SKU-C", 30),
		balance("p-d", "SKU-D", 999), // tanpa pemakaian: tidak bisa dinilai
	})

	var body reorderView
	getJSON(t, srv.URL+"/recommendations/reorder?company_id="+companyID).decode(t, &body)

	if body.Window.Days != 10 || body.Window.From != "2026-08-01" || body.Window.To != "2026-08-10" {
		t.Fatalf("window = %+v, want 1-10 Agustus (10 hari)", body.Window)
	}
	if body.Window.MovementsConsidered != 5 || body.Window.Truncated {
		t.Errorf("window = %+v, want 5 pergerakan & tidak terpotong", body.Window)
	}

	a, ok := body.bySKU("SKU-A")
	if !ok {
		t.Fatalf("SKU-A tidak direkomendasikan: %+v", body.Items)
	}
	if a.OnHand != 50 {
		t.Errorf("on_hand = %v, want 50 (saldo dua gudang dijumlahkan)", a.OnHand)
	}
	if a.OutQuantity != 100 || a.DailyVelocity != 10 {
		t.Errorf("pemakaian = %v (%v/hari), want 100 (10/hari) -- stok masuk seharusnya diabaikan", a.OutQuantity, a.DailyVelocity)
	}
	if a.DaysOfCover != 5 || a.Urgency != "HIGH" {
		t.Errorf("cover = %v (%s), want 5 hari HIGH", a.DaysOfCover, a.Urgency)
	}
	// Target 30 hari: 10/hari x 30 - 50 di tangan = 250.
	if a.SuggestedQty != 250 {
		t.Errorf("suggested_reorder_qty = %v, want 250", a.SuggestedQty)
	}
	if a.Reason == "" {
		t.Error("rekomendasi harus menjelaskan dasarnya")
	}

	c, ok := body.bySKU("SKU-C")
	if !ok {
		t.Fatalf("SKU-C tidak direkomendasikan: %+v", body.Items)
	}
	if c.DaysOfCover != 10 || c.Urgency != "MEDIUM" || c.SuggestedQty != 60 {
		t.Errorf("SKU-C = %+v, want cover 10 MEDIUM saran 60", c)
	}

	// SKU-B punya persediaan 50 hari -- di atas target, jadi tidak disarankan.
	if _, ok := body.bySKU("SKU-B"); ok {
		t.Error("SKU-B (cover 50 hari) tidak seharusnya direkomendasikan")
	}
	// SKU-D tidak pernah keluar: stok mati bukan urusan endpoint ini.
	if _, ok := body.bySKU("SKU-D"); ok {
		t.Error("SKU-D tanpa pemakaian tidak seharusnya direkomendasikan")
	}

	// SKU-E habis sama sekali (tidak ada saldo) tapi masih terpakai --
	// identitasnya diambil dari baris pergerakan, dan dia yang paling mendesak.
	e, ok := body.bySKU("SKU-E")
	if !ok {
		t.Fatalf("SKU-E tidak direkomendasikan padahal stoknya nol: %+v", body.Items)
	}
	if e.OnHand != 0 || e.DaysOfCover != 0 || e.Urgency != "HIGH" {
		t.Errorf("SKU-E = %+v, want stok 0, cover 0, HIGH", e)
	}
	if body.Items[0].ProductSKU != "SKU-E" {
		t.Errorf("urutan pertama = %q, want SKU-E (paling cepat habis)", body.Items[0].ProductSKU)
	}
}

// warehouse-service memotong daftar pergerakan di 200 baris. Rentang yang
// dihitung dari data terpotong bisa jauh lebih pendek daripada riwayat nyata,
// dan itu harus terlihat oleh pemakainya.
func TestReorderRecommendations_FlagsTruncatedHistory(t *testing.T) {
	srv, backends := newServer(t)
	companyID := newCompanyID(t)

	movements := make([]map[string]any, 0, 200)
	for i := 0; i < 200; i++ {
		movements = append(movements, movement(fmt.Sprintf("p-%d", i), fmt.Sprintf("SKU-%03d", i), "OUT", 1, "2026-08-10"))
	}
	backends.warehouse.json("/stock-movements", http.StatusOK, movements)
	backends.warehouse.json("/stock", http.StatusOK, []map[string]any{})

	var body reorderView
	getJSON(t, srv.URL+"/recommendations/reorder?company_id="+companyID).decode(t, &body)

	if !body.Window.Truncated || body.Window.MovementsConsidered != 200 {
		t.Errorf("window = %+v, want ditandai terpotong", body.Window)
	}
	// Rentang satu hari tetap dihitung satu hari, bukan nol (pembagian dengan
	// nol akan menghasilkan kecepatan tak terhingga).
	if body.Window.Days != 1 {
		t.Errorf("days = %d, want 1", body.Window.Days)
	}
	// Daftar dibatasi supaya layar tidak dibanjiri 200 baris sekaligus.
	if len(body.Items) != 50 {
		t.Errorf("items = %d, want dibatasi 50", len(body.Items))
	}
}

func TestReorderRecommendations_WarehouseDown(t *testing.T) {
	srv, backends := newServer(t)
	companyID := newCompanyID(t)
	backends.warehouse.fail("/stock")
	backends.warehouse.fail("/stock-movements")

	resp := getJSON(t, srv.URL+"/recommendations/reorder?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)

	var body reorderView
	resp.decode(t, &body)
	if len(body.Errors) == 0 {
		t.Error("kegagalan warehouse-service harus dilaporkan, bukan tampil sebagai 'tidak ada rekomendasi'")
	}
	if len(body.Items) != 0 {
		t.Errorf("items = %+v, want kosong", body.Items)
	}
}

// Saldo stok bisa MINUS (warehouse-service tidak menghalangi pengeluaran yang
// melebihi saldo). Ditemukan saat verifikasi end-to-end: halaman rekomendasi
// sempat menampilkan "cukup untuk -35 hari".
func TestReorderRecommendations_NegativeStock(t *testing.T) {
	srv, backends := newServer(t)
	companyID := newCompanyID(t)

	backends.warehouse.json("/stock-movements", http.StatusOK, []map[string]any{
		movement("p-a", "SKU-A", "OUT", 100, "2026-08-01"),
		movement("p-a", "SKU-A", "OUT", 300, "2026-08-10"),
	})
	backends.warehouse.json("/stock", http.StatusOK, []map[string]any{
		balance("p-a", "SKU-A", -320),
	})

	var body reorderView
	getJSON(t, srv.URL+"/recommendations/reorder?company_id="+companyID).decode(t, &body)

	a, ok := body.bySKU("SKU-A")
	if !ok {
		t.Fatalf("SKU-A tidak direkomendasikan: %+v", body.Items)
	}
	if a.DaysOfCover != 0 {
		t.Errorf("days_of_cover = %v, want 0 -- sisa hari negatif bukan kalimat yang berarti", a.DaysOfCover)
	}
	if a.Urgency != "HIGH" {
		t.Errorf("urgency = %q, want HIGH", a.Urgency)
	}
	if !strings.Contains(a.Reason, "MINUS") {
		t.Errorf("reason = %q, want menyebut saldo minus -- itu petunjuk bahwa pencatatannya perlu diperiksa", a.Reason)
	}
	// Saran pesan tetap menutup defisitnya, bukan cuma kebutuhan 30 hari.
	if a.SuggestedQty <= 320 {
		t.Errorf("suggested_reorder_qty = %v, want lebih dari 320 (menutup defisit + kebutuhan 30 hari)", a.SuggestedQty)
	}
}
