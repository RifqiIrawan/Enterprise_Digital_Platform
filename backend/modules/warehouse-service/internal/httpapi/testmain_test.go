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

	"github.com/enterprise-digital-platform/warehouse-service/internal/httpapi"
	"github.com/enterprise-digital-platform/warehouse-service/internal/store"
	"github.com/enterprise-digital-platform/warehouse-service/migrations"
)

var pool *pgxpool.Pool

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/warehouse_service_test?sslmode=disable"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	adminURL := getEnv("WAREHOUSE_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: warehouse-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: warehouse-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE warehouse_service_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create warehouse_service_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("WAREHOUSE_TEST_DATABASE_URL", testDatabaseURL)
	pool, err = store.Connect(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to warehouse_service_test: %v\n", err)
		os.Exit(0)
	}
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Printf("FAIL: migration of warehouse_service_test failed: %v\n", err)
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

// newServer wires the real handler; unlike the other modules' services,
// warehouse-service has no outbound cross-service clients (it's the
// RECEIVER of calls from purchasing/sales/production-service via
// POST /stock-movements/batch, not a caller), so there's nothing to stub.
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	handler := httpapi.NewHandler(pool, nil)
	mux := http.NewServeMux()
	handler.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
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

type warehouseFixture struct {
	ID   string `json:"id"`
	Code string `json:"code"`
}

func mustSeedWarehouse(t *testing.T, srv *httptest.Server, companyID string) warehouseFixture {
	t.Helper()
	code := "WH-" + uuid.NewString()[:8]
	resp := postJSON(t, srv.URL+"/warehouses", map[string]any{
		"company_id": companyID, "code": code, "name": "Test Warehouse " + code,
	})
	requireStatus(t, resp, http.StatusCreated)
	var wh warehouseFixture
	resp.decode(t, &wh)
	return wh
}

type productFixture struct {
	ID   string `json:"id"`
	SKU  string `json:"sku"`
	Unit string `json:"unit"`
}

func mustSeedProduct(t *testing.T, srv *httptest.Server, companyID string) productFixture {
	t.Helper()
	sku := "SKU-" + uuid.NewString()[:8]
	resp := postJSON(t, srv.URL+"/products", map[string]any{
		"company_id": companyID, "sku": sku, "name": "Test Product " + sku,
	})
	requireStatus(t, resp, http.StatusCreated)
	var p productFixture
	resp.decode(t, &p)
	return p
}

// mustGetStockQuantity reads the on-hand quantity for a warehouse+product
// from GET /stock, returning 0 if there's no stock_balances row at all
// (mirrors how the frontend would treat an absent balance).
func mustGetStockQuantity(t *testing.T, srv *httptest.Server, companyID, warehouseID, productID string) float64 {
	t.Helper()
	resp := getJSON(t, srv.URL+"/stock?company_id="+companyID+"&warehouse_id="+warehouseID)
	requireStatus(t, resp, http.StatusOK)
	var balances []struct {
		ProductID string  `json:"product_id"`
		Quantity  float64 `json:"quantity"`
	}
	resp.decode(t, &balances)
	for _, b := range balances {
		if b.ProductID == productID {
			return b.Quantity
		}
	}
	return 0
}
