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

	"github.com/enterprise-digital-platform/production-service/internal/httpapi"
	"github.com/enterprise-digital-platform/production-service/internal/store"
	"github.com/enterprise-digital-platform/production-service/internal/warehouseclient"
	"github.com/enterprise-digital-platform/production-service/migrations"
)

var pool *pgxpool.Pool

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/production_service_test?sslmode=disable"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	adminURL := getEnv("PRODUCTION_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: production-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: production-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE production_service_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create production_service_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("PRODUCTION_TEST_DATABASE_URL", testDatabaseURL)
	pool, err = store.Connect(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to production_service_test: %v\n", err)
		os.Exit(0)
	}
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Printf("FAIL: migration of production_service_test failed: %v\n", err)
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
// unreachable address — fine for every endpoint except completeWorkOrder,
// which has its own dedicated stub setup below.
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

type warehouseStubCall struct {
	movementType string
	body         []byte
}

// newWarehouseStub fakes warehouse-service's POST /stock-movements/batch
// contract. completeWorkOrder makes it TWICE per call (OUT for components,
// then IN for the finished product) -- failOnCallNumber lets a test target
// either the first (component consumption) or second (finished-goods) call
// specifically, to exercise both halves of the two-call failure path.
func newWarehouseStub(t *testing.T, failOnCallNumber int) (*httptest.Server, *[]warehouseStubCall) {
	t.Helper()
	calls := &[]warehouseStubCall{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /stock-movements/batch", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var parsed struct {
			MovementType string `json:"movement_type"`
		}
		_ = json.Unmarshal(body, &parsed)
		*calls = append(*calls, warehouseStubCall{movementType: parsed.MovementType, body: body})
		if failOnCallNumber == len(*calls) {
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

// newServerWithWarehouseStub wires the handler to a stub warehouse-service.
// failOnCallNumber: 0 = never fail, 1 = fail the OUT (component) call,
// 2 = fail the IN (finished-goods) call after OUT already succeeded.
func newServerWithWarehouseStub(t *testing.T, failOnCallNumber int) (srv *httptest.Server, calls *[]warehouseStubCall) {
	t.Helper()
	stub, c := newWarehouseStub(t, failOnCallNumber)
	warehouse := warehouseclient.New(stub.URL)
	handler := httpapi.NewHandler(pool, nil, warehouse)
	mux := http.NewServeMux()
	handler.Register(mux)
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, c
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

// bomFixture bundles the created BOM together with the (fake) finished-good
// and component product IDs used to build it -- production-service trusts
// whatever product_id the frontend sends (no FK to warehouse-service's own
// database), so tests can use arbitrary UUIDs without a real warehouse-service.
type bomFixture struct {
	ID                 string `json:"id"`
	BOMCode            string `json:"bom_code"`
	ProductID          string `json:"product_id"`
	ComponentProductID string
	QuantityPerUnit    float64
}

func mustSeedBOM(t *testing.T, srv *httptest.Server, companyID string) bomFixture {
	t.Helper()
	code := "BOM-" + uuid.NewString()[:8]
	productID := uuid.NewString()
	componentID := uuid.NewString()
	resp := postJSON(t, srv.URL+"/boms", map[string]any{
		"company_id": companyID, "bom_code": code, "name": "Test BOM " + code, "product_id": productID,
		"lines": []map[string]any{{"component_product_id": componentID, "quantity_per_unit": 2}},
	})
	requireStatus(t, resp, http.StatusCreated)
	var b bomFixture
	resp.decode(t, &b)
	b.ComponentProductID = componentID
	b.QuantityPerUnit = 2
	return b
}
