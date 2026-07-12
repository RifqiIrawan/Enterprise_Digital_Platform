package httpapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/enterprise-digital-platform/ai-bi-service/internal/config"
	"github.com/enterprise-digital-platform/ai-bi-service/internal/httpapi"
)

// fakeService is a stand-in for one of the 8 real services ai-bi-service
// aggregates from. Unlike every other module in this repo, ai-bi-service has
// no database of its own (see internal/config/config.go) -- it's pure HTTP
// fan-out -- so there's nothing to run against a real Postgres here. Instead
// every dependency is a small httptest.Server whose per-path responses tests
// configure directly, which is both faster and keeps each test independent
// of the other 8 services' actual implementations (same "stub the HTTP
// contract, don't run the real thing" principle used for financeclient/
// warehouseclient stubs in the other modules' test suites).
type fakeService struct {
	mu     sync.Mutex
	routes map[string]http.HandlerFunc
}

func newFakeService(t *testing.T) (*httptest.Server, *fakeService) {
	t.Helper()
	fs := &fakeService{routes: map[string]http.HandlerFunc{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		h, ok := fs.routes[r.URL.Path]
		fs.mu.Unlock()
		if !ok {
			// Unconfigured endpoint: behave like an empty-but-healthy service
			// (matches how e.g. a brand-new company with no data looks) so
			// tests only need to set up the paths they actually care about.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
			return
		}
		h(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, fs
}

// json configures fs to answer GET <path> with the given status and JSON body.
func (fs *fakeService) json(path string, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.routes[path] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(data)
	}
}

// fail configures fs to answer GET <path> with a 500, simulating that
// downstream service being unreachable/erroring.
func (fs *fakeService) fail(path string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.routes[path] = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"simulated failure"}`))
	}
}

// backends bundles the 8 fake services dashboardSummary/forecastingSummary/
// anomalyScan fan out to, named the same way config.Config does.
type backends struct {
	sales      *fakeService
	purchasing *fakeService
	finance    *fakeService
	warehouse  *fakeService
	production *fakeService
	qc         *fakeService
	hr         *fakeService
	asset      *fakeService
}

// newServer wires a real Handler to 8 independent fake services -- every
// test starts from a clean slate where every downstream endpoint returns
// `[]` until explicitly configured via backends.<service>.json(...).
func newServer(t *testing.T) (*httptest.Server, *backends) {
	t.Helper()
	salesSrv, sales := newFakeService(t)
	purchasingSrv, purchasing := newFakeService(t)
	financeSrv, finance := newFakeService(t)
	warehouseSrv, warehouse := newFakeService(t)
	productionSrv, production := newFakeService(t)
	qcSrv, qc := newFakeService(t)
	hrSrv, hr := newFakeService(t)
	assetSrv, asset := newFakeService(t)

	cfg := &config.Config{
		Port:                 "0",
		SalesServiceURL:      salesSrv.URL,
		PurchasingServiceURL: purchasingSrv.URL,
		FinanceServiceURL:    financeSrv.URL,
		WarehouseServiceURL:  warehouseSrv.URL,
		ProductionServiceURL: productionSrv.URL,
		QCServiceURL:         qcSrv.URL,
		HRServiceURL:         hrSrv.URL,
		AssetServiceURL:      assetSrv.URL,
	}
	handler := httpapi.NewHandler(cfg)
	mux := http.NewServeMux()
	handler.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv, &backends{
		sales: sales, purchasing: purchasing, finance: finance, warehouse: warehouse,
		production: production, qc: qc, hr: hr, asset: asset,
	}
}

type apiResponse struct {
	status int
	body   []byte
}

func (r apiResponse) decode(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("decode response body %q: %v", r.body, err)
	}
}

func getJSON(t *testing.T, url string) apiResponse {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return apiResponse{status: resp.StatusCode, body: body}
}

func requireStatus(t *testing.T, resp apiResponse, want int) {
	t.Helper()
	if resp.status != want {
		t.Fatalf("expected status %d, got %d (body: %s)", want, resp.status, resp.body)
	}
}

func newCompanyID(t *testing.T) string {
	t.Helper()
	return uuid.NewString()
}
