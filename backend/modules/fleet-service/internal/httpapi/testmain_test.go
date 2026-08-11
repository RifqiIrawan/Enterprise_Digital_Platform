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

	"github.com/enterprise-digital-platform/fleet-service/internal/ecommerceclient"
	"github.com/enterprise-digital-platform/fleet-service/internal/httpapi"
	"github.com/enterprise-digital-platform/fleet-service/internal/store"
	"github.com/enterprise-digital-platform/fleet-service/migrations"
)

var pool *pgxpool.Pool

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/fleet_service_test?sslmode=disable"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	adminURL := getEnv("FLEET_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: fleet-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: fleet-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE fleet_service_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create fleet_service_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("FLEET_TEST_DATABASE_URL", testDatabaseURL)
	pool, err = store.Connect(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to fleet_service_test: %v\n", err)
		os.Exit(0)
	}
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Printf("FAIL: migration of fleet_service_test failed: %v\n", err)
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

// newServer wires the real handler with an ecommerce client pointed at an
// unreachable address -- correct for every endpoint that does not touch
// ecommerce-service. Anything that does gets newServerWithEcommerceStub.
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	return newServerWith(t, ecommerceclient.New("http://127.0.0.1:1"))
}

func newServerWith(t *testing.T, ecommerce *ecommerceclient.Client) *httptest.Server {
	t.Helper()
	handler := httpapi.NewHandler(pool, nil, ecommerce)
	mux := http.NewServeMux()
	handler.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

type stubCall struct {
	method string
	path   string
	actor  string
}

// ecommerceStubConfig describes the fake ecommerce-service. Defaults model the
// happy path: an order that exists, belongs to the given company, and is
// SHIPPED (the only status createDeliveryOrder accepts).
type ecommerceStubConfig struct {
	orderID       string
	companyID     string
	orderStatus   string
	orderNumber   string
	customerName  string
	address       string
	getFails      bool
	deliverFails  bool
	orderNotFound bool
}

// newEcommerceStub fakes the two ecommerce-service endpoints fleet-service
// actually calls. GET /orders/{id} deliberately returns the FLAT shape that
// model.OrderWithItems really marshals to (embedded struct, no "order" key) --
// a stub that returned a nested shape would let a client bug pass unnoticed.
func newEcommerceStub(t *testing.T, cfg ecommerceStubConfig) (*httptest.Server, *[]stubCall) {
	t.Helper()
	calls := &[]stubCall{}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, stubCall{method: http.MethodGet, path: r.URL.Path, actor: r.Header.Get("X-User-Id")})
		if cfg.getFails {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"simulated ecommerce-service failure"}`))
			return
		}
		if cfg.orderNotFound {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Order tidak ditemukan"}`))
			return
		}
		writeStubJSON(w, http.StatusOK, map[string]any{
			"id":               cfg.orderID,
			"company_id":       cfg.companyID,
			"order_number":     cfg.orderNumber,
			"customer_name":    cfg.customerName,
			"shipping_address": cfg.address,
			"status":           cfg.orderStatus,
			"items":            []any{},
		})
	})

	mux.HandleFunc("POST /orders/{id}/deliver", func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, stubCall{method: http.MethodPost, path: r.URL.Path, actor: r.Header.Get("X-User-Id")})
		if cfg.deliverFails {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"Order tidak ditemukan atau tidak berstatus SHIPPED"}`))
			return
		}
		writeStubJSON(w, http.StatusOK, map[string]any{"id": cfg.orderID, "status": "DELIVERED"})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, calls
}

func writeStubJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func newServerWithEcommerceStub(t *testing.T, cfg ecommerceStubConfig) (*httptest.Server, *[]stubCall) {
	t.Helper()
	stub, calls := newEcommerceStub(t, cfg)
	return newServerWith(t, ecommerceclient.New(stub.URL)), calls
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

func putJSON(t *testing.T, url string, payload any) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodPut, url, payload, uuid.NewString())
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

type vehicleFixture struct {
	ID          string `json:"id"`
	VehicleCode string `json:"vehicle_code"`
	Status      string `json:"status"`
}

type driverFixture struct {
	ID         string `json:"id"`
	DriverCode string `json:"driver_code"`
	Status     string `json:"status"`
}

type deliveryFixture struct {
	ID                 string  `json:"id"`
	DeliveryNumber     string  `json:"delivery_number"`
	Status             string  `json:"status"`
	VehicleID          string  `json:"vehicle_id"`
	DriverID           string  `json:"driver_id"`
	RecipientName      string  `json:"recipient_name"`
	DestinationAddress string  `json:"destination_address"`
	ReferenceNumber    *string `json:"reference_number"`
	EcommerceOrderID   *string `json:"ecommerce_order_id"`
}

func mustSeedVehicle(t *testing.T, srv *httptest.Server, companyID string) vehicleFixture {
	t.Helper()
	resp := postJSON(t, srv.URL+"/vehicles", map[string]any{
		"company_id":   companyID,
		"vehicle_code": "VHC-" + uuid.NewString()[:8],
		"plate_number": "B 1234 XYZ",
		"name":         "Toyota Hiace",
		"vehicle_type": "VAN",
		"capacity_kg":  1000,
	})
	requireStatus(t, resp, http.StatusCreated)
	var v vehicleFixture
	resp.decode(t, &v)
	return v
}

func mustSeedDriver(t *testing.T, srv *httptest.Server, companyID string) driverFixture {
	t.Helper()
	resp := postJSON(t, srv.URL+"/drivers", map[string]any{
		"company_id":     companyID,
		"driver_code":    "DRV-" + uuid.NewString()[:8],
		"name":           "Joko Susilo",
		"phone":          "08123456789",
		"license_number": "SIM-B1-001",
	})
	requireStatus(t, resp, http.StatusCreated)
	var d driverFixture
	resp.decode(t, &d)
	return d
}

// mustSeedDelivery creates a standalone delivery (no e-commerce order), the
// case that does not touch ecommerce-service at all.
func mustSeedDelivery(t *testing.T, srv *httptest.Server, companyID string) (deliveryFixture, vehicleFixture, driverFixture) {
	t.Helper()
	v := mustSeedVehicle(t, srv, companyID)
	d := mustSeedDriver(t, srv, companyID)
	resp := postJSON(t, srv.URL+"/delivery-orders", map[string]any{
		"company_id":          companyID,
		"vehicle_id":          v.ID,
		"driver_id":           d.ID,
		"recipient_name":      "Siti Aminah",
		"recipient_phone":     "08987654321",
		"destination_address": "Jl. Sudirman No. 5",
	})
	requireStatus(t, resp, http.StatusCreated)
	var del deliveryFixture
	resp.decode(t, &del)
	return del, v, d
}

func fetchVehicleStatus(t *testing.T, srv *httptest.Server, companyID, vehicleID string) string {
	t.Helper()
	resp := getJSON(t, srv.URL+"/vehicles?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var vehicles []vehicleFixture
	resp.decode(t, &vehicles)
	for _, v := range vehicles {
		if v.ID == vehicleID {
			return v.Status
		}
	}
	t.Fatalf("vehicle %s not found for company %s", vehicleID, companyID)
	return ""
}

func fetchDriverStatus(t *testing.T, srv *httptest.Server, companyID, driverID string) string {
	t.Helper()
	resp := getJSON(t, srv.URL+"/drivers?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var drivers []driverFixture
	resp.decode(t, &drivers)
	for _, d := range drivers {
		if d.ID == driverID {
			return d.Status
		}
	}
	t.Fatalf("driver %s not found for company %s", driverID, companyID)
	return ""
}
