package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/purchasing-service/internal/financeclient"
	"github.com/enterprise-digital-platform/purchasing-service/internal/httpapi"
	"github.com/enterprise-digital-platform/purchasing-service/internal/store"
	"github.com/enterprise-digital-platform/purchasing-service/internal/warehouseclient"
	"github.com/enterprise-digital-platform/purchasing-service/migrations"
)

var pool *pgxpool.Pool

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/purchasing_service_test?sslmode=disable"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	adminURL := getEnv("PURCHASING_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: purchasing-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: purchasing-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE purchasing_service_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create purchasing_service_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("PURCHASING_TEST_DATABASE_URL", testDatabaseURL)
	pool, err = store.Connect(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to purchasing_service_test: %v\n", err)
		os.Exit(0)
	}
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Printf("FAIL: migration of purchasing_service_test failed: %v\n", err)
		pool.Close()
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newCompanyID(t *testing.T) string {
	t.Helper()
	return uuid.NewString()
}

// newServer wires the real handler with finance/warehouse clients pointed at
// unreachable addresses — fine for every endpoint except receivePurchaseOrder
// and invoicePurchaseOrder, which have their own dedicated stub setup below.
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	finance := financeclient.New("http://127.0.0.1:1")
	warehouse := warehouseclient.New("http://127.0.0.1:1")
	handler := httpapi.NewHandler(pool, nil, finance, warehouse)
	mux := http.NewServeMux()
	handler.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

type stubCall struct {
	path string
	body []byte
}

// newFinanceStub fakes just enough of finance-service's contract
// (POST /invoices -> {id,status}, POST /invoices/{id}/post -> 200) for
// financeclient.CreateAndPostInvoice to succeed, without pulling in
// finance-service's actual implementation (same pattern used for
// hr-service/sales-service's cross-service posting tests).
func newFinanceStub(t *testing.T, failOnCreate bool) (*httptest.Server, *[]stubCall) {
	t.Helper()
	calls := &[]stubCall{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /invoices", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*calls = append(*calls, stubCall{path: r.URL.Path, body: body})
		if failOnCreate {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"simulated finance-service failure"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": uuid.NewString(), "status": "DRAFT"})
	})
	mux.HandleFunc("POST /invoices/{id}/post", func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, stubCall{path: r.URL.Path})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "POSTED"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, calls
}

// newWarehouseStub fakes warehouse-service's POST /stock-movements/batch
// contract (the handler doesn't unmarshal the response body at all, just
// checks the status code — see warehouseclient.PostMovementBatch).
func newWarehouseStub(t *testing.T, fail bool) (*httptest.Server, *[]stubCall) {
	t.Helper()
	calls := &[]stubCall{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /stock-movements/batch", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*calls = append(*calls, stubCall{path: r.URL.Path, body: body})
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"simulated warehouse-service failure"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`[]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, calls
}

// newServerWithStubs wires the handler to stub finance/warehouse servers so
// receivePurchaseOrder/invoicePurchaseOrder can be exercised end-to-end
// without a real finance-service or warehouse-service running.
func newServerWithStubs(t *testing.T, financeFail, warehouseFail bool) (srv *httptest.Server, financeCalls, warehouseCalls *[]stubCall) {
	t.Helper()
	financeStub, fCalls := newFinanceStub(t, financeFail)
	warehouseStub, wCalls := newWarehouseStub(t, warehouseFail)
	finance := financeclient.New(financeStub.URL)
	warehouse := warehouseclient.New(warehouseStub.URL)
	handler := httpapi.NewHandler(pool, nil, finance, warehouse)
	mux := http.NewServeMux()
	handler.Register(mux)
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, fCalls, wCalls
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

func (r apiResponse) errorMessage() string {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(r.body, &e)
	return e.Error
}

func doRequest(t *testing.T, method, url string, payload any, actorUserID string) apiResponse {
	t.Helper()
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal request payload: %v", err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if actorUserID != "" {
		req.Header.Set("X-User-Id", actorUserID)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return apiResponse{status: resp.StatusCode, body: respBody}
}

func postJSON(t *testing.T, url string, payload any) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodPost, url, payload, uuid.NewString())
}

func getJSON(t *testing.T, url string) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodGet, url, nil, "")
}

func requireStatus(t *testing.T, resp apiResponse, want int) {
	t.Helper()
	if resp.status != want {
		t.Fatalf("expected status %d, got %d (body: %s)", want, resp.status, resp.body)
	}
}

type supplierFixture struct {
	ID           string `json:"id"`
	SupplierCode string `json:"supplier_code"`
}

func mustSeedSupplier(t *testing.T, srv *httptest.Server, companyID string) supplierFixture {
	t.Helper()
	code := "SUP-" + uuid.NewString()[:8]
	resp := postJSON(t, srv.URL+"/suppliers", map[string]any{
		"company_id": companyID, "supplier_code": code, "name": "Test Supplier " + code,
	})
	requireStatus(t, resp, http.StatusCreated)
	var s supplierFixture
	resp.decode(t, &s)
	return s
}
