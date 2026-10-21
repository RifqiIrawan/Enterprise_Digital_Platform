package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/enterprise-digital-platform/ecommerce-service/internal/httpapi"
	"github.com/enterprise-digital-platform/ecommerce-service/internal/warehouseclient"
)

// stockShortStub meniru jawaban warehouse-service saat saldo tidak cukup: 409
// dengan kode yang bisa dibaca mesin.
func stockShortStub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /stock-movements/batch", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"Stok tidak cukup untuk SKU-1 - Tepung di gudang WH-A: diminta 5, tersedia 2","code":"INSUFFICIENT_STOCK"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestShipOrder_InsufficientStockIsConflictNotBadGateway(t *testing.T) {
	handler := httpapi.NewHandler(pool, nil, warehouseclient.New(stockShortStub(t).URL))
	mux := http.NewServeMux()
	handler.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	companyID := newCompanyID(t)
	o := mustSeedOrder(t, srv, companyID)
	mustPayOrder(t, srv.URL, o.ID)

	resp := postJSON(t, srv.URL+"/orders/"+o.ID+"/ship", map[string]any{"warehouse_id": uuid.NewString()})
	requireStatus(t, resp, http.StatusConflict)
	if !strings.Contains(resp.errorMessage(), "diminta 5, tersedia 2") {
		t.Errorf("pesan = %q, want pesan stok dari warehouse-service diteruskan apa adanya", resp.errorMessage())
	}
	if strings.Contains(resp.errorMessage(), "warehouse-service returned") {
		t.Errorf("pesan masih membungkus JSON mentah: %q", resp.errorMessage())
	}

	getResp := getJSON(t, srv.URL+"/orders/"+o.ID)
	var reloaded struct {
		Status string `json:"status"`
	}
	getResp.decode(t, &reloaded)
	if reloaded.Status != "PAID" {
		t.Errorf("status = %q, want PAID", reloaded.Status)
	}
}
