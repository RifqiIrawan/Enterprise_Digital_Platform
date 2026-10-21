package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/enterprise-digital-platform/production-service/internal/httpapi"
	"github.com/enterprise-digital-platform/production-service/internal/warehouseclient"
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

// Bahan baku kurang saat work order diselesaikan: 409 dengan pesan stok, dan
// work order tetap IN_PROGRESS tanpa quantity_produced.
func TestCompleteWorkOrder_InsufficientComponentStockIsConflictNotBadGateway(t *testing.T) {
	handler := httpapi.NewHandler(pool, nil, warehouseclient.New(stockShortStub(t).URL))
	mux := http.NewServeMux()
	handler.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	companyID := newCompanyID(t)
	bom := mustSeedBOM(t, srv, companyID)
	wo := mustCreateWorkOrder(t, srv.URL, companyID, bom.ID, uuid.NewString(), 10)
	mustStartWorkOrder(t, srv.URL, wo.ID)

	resp := postJSON(t, srv.URL+"/work-orders/"+wo.ID+"/complete", map[string]any{"quantity_produced": 9})
	requireStatus(t, resp, http.StatusConflict)
	if !strings.Contains(resp.errorMessage(), "diminta 5, tersedia 2") {
		t.Errorf("pesan = %q, want pesan stok dari warehouse-service diteruskan apa adanya", resp.errorMessage())
	}
	if strings.Contains(resp.errorMessage(), "warehouse-service returned") {
		t.Errorf("pesan masih membungkus JSON mentah: %q", resp.errorMessage())
	}

	getResp := getJSON(t, srv.URL+"/work-orders/"+wo.ID)
	var reloaded workOrderView
	getResp.decode(t, &reloaded)
	if reloaded.Status != "IN_PROGRESS" || reloaded.QuantityProduced != nil {
		t.Errorf("work order berubah: status=%q quantity_produced=%v", reloaded.Status, reloaded.QuantityProduced)
	}
}
