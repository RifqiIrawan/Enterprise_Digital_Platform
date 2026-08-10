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

	"github.com/enterprise-digital-platform/ecommerce-service/internal/httpapi"
	"github.com/enterprise-digital-platform/ecommerce-service/internal/store"
	"github.com/enterprise-digital-platform/ecommerce-service/internal/warehouseclient"
	"github.com/enterprise-digital-platform/ecommerce-service/migrations"
)

var pool *pgxpool.Pool

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/ecommerce_service_test?sslmode=disable"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	adminURL := getEnv("ECOMMERCE_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: ecommerce-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: ecommerce-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE ecommerce_service_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create ecommerce_service_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("ECOMMERCE_TEST_DATABASE_URL", testDatabaseURL)
	pool, err = store.Connect(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to ecommerce_service_test: %v\n", err)
		os.Exit(0)
	}
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Printf("FAIL: migration of ecommerce_service_test failed: %v\n", err)
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

// newServer wires the real handler with a warehouse client pointed at an
// unreachable address -- fine for every endpoint except shipOrder, which
// has its own dedicated stub setup below (newServerWithWarehouseStub).
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	warehouse := warehouseclient.New("http://127.0.0.1:1")
	handler := httpapi.NewHandler(pool, nil, warehouse)
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

// newWarehouseStub fakes warehouse-service's POST /stock-movements/batch
// contract (the handler doesn't unmarshal the response body at all, just
// checks the status code -- see warehouseclient.PostMovementBatch).
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

// newServerWithWarehouseStub wires the handler to a stub warehouse server so
// shipOrder can be exercised end-to-end without a real warehouse-service
// running.
func newServerWithWarehouseStub(t *testing.T, warehouseFail bool) (srv *httptest.Server, warehouseCalls *[]stubCall) {
	t.Helper()
	warehouseStub, wCalls := newWarehouseStub(t, warehouseFail)
	warehouse := warehouseclient.New(warehouseStub.URL)
	handler := httpapi.NewHandler(pool, nil, warehouse)
	mux := http.NewServeMux()
	handler.Register(mux)
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, wCalls
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

type orderFixture struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	OrderNumber string  `json:"order_number"`
	TotalAmount float64 `json:"total_amount"`
}

func defaultOrderPayload(companyID string) map[string]any {
	return map[string]any{
		"company_id":       companyID,
		"customer_name":    "Budi Santoso",
		"customer_email":   "budi@example.com",
		"shipping_address": "Jl. Merdeka No. 1",
		"lines": []map[string]any{
			{"product_id": uuid.NewString(), "product_sku": "SKU-001", "product_name": "Produk Uji", "unit_price": 50000, "quantity": 2},
		},
	}
}

func mustSeedOrder(t *testing.T, srv *httptest.Server, companyID string) orderFixture {
	t.Helper()
	resp := postJSON(t, srv.URL+"/orders", defaultOrderPayload(companyID))
	requireStatus(t, resp, http.StatusCreated)
	var o orderFixture
	resp.decode(t, &o)
	return o
}

func mustPayOrder(t *testing.T, baseURL, orderID string) {
	t.Helper()
	requireStatus(t, postJSON(t, baseURL+"/orders/"+orderID+"/pay", nil), http.StatusOK)
}
