package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// Stok tidak boleh negatif. Aturannya satu: sebuah OUT hanya berhasil kalau
// saldo mencukupi; IN tidak pernah ditolak. Semua alur (manual, batch dari
// service lain, konfirmasi transfer, posting opname) lewat applyStockMovement,
// jadi satu aturan itu cukup -- test di bawah membuktikan tiap alur memakainya,
// dan bahwa penolakan tidak meninggalkan jejak (saldo utuh, tanpa baris ledger).

func TestStockOut_RejectedWhenBalanceInsufficient(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)
	mustGiveStock(t, srv, companyID, wh.ID, p.ID, 10)
	before := stockMovementCount(t, srv, companyID, wh.ID)

	resp := postJSON(t, srv.URL+"/stock-movements", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "product_id": p.ID, "movement_type": "OUT", "quantity": 11,
	})
	requireStatus(t, resp, http.StatusConflict)
	// Pesannya menyebut yang diminta dan yang tersedia, supaya orang tahu
	// berapa yang bisa dikeluarkan.
	msg := resp.errorMessage()
	if !strings.Contains(msg, "diminta 11") || !strings.Contains(msg, "tersedia 10") {
		t.Errorf("pesan = %q, want memuat 'diminta 11' dan 'tersedia 10'", msg)
	}
	// Kode yang bisa dibaca mesin: service pemanggil memakainya untuk
	// meneruskan "stok tidak cukup" sebagai 409, bukan 502.
	var body struct {
		Code string `json:"code"`
	}
	resp.decode(t, &body)
	if body.Code != "INSUFFICIENT_STOCK" {
		t.Errorf("code = %q, want INSUFFICIENT_STOCK", body.Code)
	}
	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, p.ID); got != 10 {
		t.Errorf("saldo setelah OUT ditolak = %.2f, want tetap 10.00", got)
	}
	if after := stockMovementCount(t, srv, companyID, wh.ID); after != before {
		t.Errorf("OUT yang ditolak meninggalkan %d baris ledger", after-before)
	}
}

func TestStockOut_AllowedDownToExactlyZero(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)
	mustGiveStock(t, srv, companyID, wh.ID, p.ID, 10)

	requireStatus(t, postJSON(t, srv.URL+"/stock-movements", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "product_id": p.ID, "movement_type": "OUT", "quantity": 10,
	}), http.StatusCreated)
	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, p.ID); got != 0 {
		t.Errorf("saldo = %.2f, want 0.00 (menghabiskan stok itu sah)", got)
	}
}

func TestStockOut_RejectedWhenThereIsNoBalanceRow(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/stock-movements", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "product_id": p.ID, "movement_type": "OUT", "quantity": 1,
	})
	requireStatus(t, resp, http.StatusConflict)
	if !strings.Contains(resp.errorMessage(), "tersedia 0") {
		t.Errorf("pesan = %q, want 'tersedia 0'", resp.errorMessage())
	}
}

// Satu baris batch yang kurang membatalkan SELURUH batch: barang dari baris
// pertama yang sudah "keluar" harus kembali, atau pesanan yang gagal
// di-fulfill tetap mengurangi stok sebagian.
func TestStockOutBatch_IsAllOrNothing(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)
	enough := mustSeedProduct(t, srv, companyID)
	short := mustSeedProduct(t, srv, companyID)
	mustGiveStock(t, srv, companyID, wh.ID, enough.ID, 50)
	mustGiveStock(t, srv, companyID, wh.ID, short.ID, 2)
	before := stockMovementCount(t, srv, companyID, wh.ID)

	resp := postJSON(t, srv.URL+"/stock-movements/batch", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "movement_type": "OUT", "reference_type": "SALES_ORDER",
		"lines": []map[string]any{
			{"product_id": enough.ID, "quantity": 5},
			{"product_id": short.ID, "quantity": 3},
		},
	})
	requireStatus(t, resp, http.StatusConflict)
	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, enough.ID); got != 50 {
		t.Errorf("baris pertama tetap terpotong: saldo = %.2f, want 50.00", got)
	}
	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, short.ID); got != 2 {
		t.Errorf("saldo produk kurang = %.2f, want 2.00", got)
	}
	if after := stockMovementCount(t, srv, companyID, wh.ID); after != before {
		t.Errorf("batch yang ditolak meninggalkan %d baris ledger", after-before)
	}
}

// Stok yang sudah negatif dari masa sebelum aturan ini tidak boleh terkunci:
// barang masuk adalah cara saldo itu pulih, dan OUT tetap ditolak sampai pulih.
func TestStockIn_StillAllowedOnLegacyNegativeBalance(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)
	mustGiveStock(t, srv, companyID, wh.ID, p.ID, 1)
	if _, err := pool.Exec(context.Background(),
		`UPDATE stock_balances SET quantity = -320 WHERE warehouse_id = $1 AND product_id = $2`, wh.ID, p.ID); err != nil {
		t.Fatal(err)
	}

	requireStatus(t, postJSON(t, srv.URL+"/stock-movements", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "product_id": p.ID, "movement_type": "IN", "quantity": 100,
	}), http.StatusCreated)
	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, p.ID); got != -220 {
		t.Errorf("saldo = %.2f, want -220.00 (IN diterima walau masih negatif)", got)
	}
	requireStatus(t, postJSON(t, srv.URL+"/stock-movements", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "product_id": p.ID, "movement_type": "OUT", "quantity": 1,
	}), http.StatusConflict)
}

func TestConfirmStockTransfer_RejectedWhenSourceShort(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	whA := mustSeedWarehouse(t, srv, companyID)
	whB := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)
	mustGiveStock(t, srv, companyID, whA.ID, p.ID, 5)
	tr := mustCreateStockTransfer(t, srv.URL, companyID, whA.ID, whB.ID, p.ID, 30)

	requireStatus(t, postJSON(t, srv.URL+"/stock-transfers/"+tr.ID+"/confirm", nil), http.StatusConflict)

	if got := mustGetStockQuantity(t, srv, companyID, whA.ID, p.ID); got != 5 {
		t.Errorf("gudang asal = %.2f, want 5.00", got)
	}
	if got := mustGetStockQuantity(t, srv, companyID, whB.ID, p.ID); got != 0 {
		t.Errorf("gudang tujuan = %.2f, want 0.00 (tidak boleh menerima apa pun)", got)
	}
	// Setelah stok ditambah, transfer yang sama masih bisa dikonfirmasi:
	// penolakan tidak boleh mengubah statusnya menjadi sesuatu yang final.
	mustGiveStock(t, srv, companyID, whA.ID, p.ID, 25)
	requireStatus(t, postJSON(t, srv.URL+"/stock-transfers/"+tr.ID+"/confirm", nil), http.StatusOK)
	if got := mustGetStockQuantity(t, srv, companyID, whB.ID, p.ID); got != 30 {
		t.Errorf("gudang tujuan setelah konfirmasi = %.2f, want 30.00", got)
	}
}

// Opname menyesuaikan dengan selisih yang di-snapshot saat dibuat. Kalau stok
// keluar sesudah itu, penyesuaian turun bisa melampaui saldo -- ditolak, bukan
// dibiarkan membuat saldo negatif.
func TestPostStockOpname_RejectedWhenAdjustmentExceedsCurrentBalance(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)
	mustGiveStock(t, srv, companyID, wh.ID, p.ID, 40)

	createResp := postJSON(t, srv.URL+"/stock-opnames", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "opname_date": today(),
		"lines": []map[string]any{{"product_id": p.ID, "counted_quantity": 5}}, // selisih -35 dari snapshot 40
	})
	requireStatus(t, createResp, http.StatusCreated)
	var o stockOpnameView
	createResp.decode(t, &o)

	// Stok keluar setelah snapshot: tinggal 10, padahal penyesuaian minta -35.
	requireStatus(t, postJSON(t, srv.URL+"/stock-movements", map[string]any{
		"company_id": companyID, "warehouse_id": wh.ID, "product_id": p.ID, "movement_type": "OUT", "quantity": 30,
	}), http.StatusCreated)

	requireStatus(t, postJSON(t, srv.URL+"/stock-opnames/"+o.ID+"/post", nil), http.StatusConflict)
	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, p.ID); got != 10 {
		t.Errorf("saldo = %.2f, want tetap 10.00", got)
	}
}

// Dua OUT bersamaan atas saldo yang sama tidak boleh sama-sama lolos. Ini
// alasan pengurangannya UPDATE bersyarat, bukan baca-lalu-tulis.
func TestStockOut_ConcurrentRequestsCannotOversell(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)
	p := mustSeedProduct(t, srv, companyID)
	mustGiveStock(t, srv, companyID, wh.ID, p.ID, 10)

	const workers = 8 // masing-masing minta 6: paling banyak satu yang boleh lolos
	var wg sync.WaitGroup
	statuses := make([]int, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			statuses[i] = postJSON(t, srv.URL+"/stock-movements", map[string]any{
				"company_id": companyID, "warehouse_id": wh.ID, "product_id": p.ID, "movement_type": "OUT", "quantity": 6,
			}).status
		}(i)
	}
	wg.Wait()

	created := 0
	for _, s := range statuses {
		if s == http.StatusCreated {
			created++
		} else if s != http.StatusConflict {
			t.Errorf("status tak terduga %d", s)
		}
	}
	if created != 1 {
		t.Errorf("%d OUT lolos dari saldo 10 dengan permintaan 6 masing-masing, want tepat 1", created)
	}
	if got := mustGetStockQuantity(t, srv, companyID, wh.ID, p.ID); got != 4 {
		t.Errorf("saldo akhir = %.2f, want 4.00", got)
	}
}
