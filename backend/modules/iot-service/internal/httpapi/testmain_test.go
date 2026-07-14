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

	"github.com/enterprise-digital-platform/iot-service/internal/httpapi"
	"github.com/enterprise-digital-platform/iot-service/internal/store"
	"github.com/enterprise-digital-platform/iot-service/migrations"
)

var pool *pgxpool.Pool

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/iot_service_test?sslmode=disable"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	adminURL := getEnv("IOT_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: iot-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: iot-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE iot_service_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create iot_service_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("IOT_TEST_DATABASE_URL", testDatabaseURL)
	pool, err = store.Connect(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to iot_service_test: %v\n", err)
		os.Exit(0)
	}
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Printf("FAIL: migration of iot_service_test failed: %v\n", err)
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

// newServer wires the real handler with events disabled (nil is safe, see
// eventbus.Publisher.Publish) -- iot-service's HTTP layer has no outbound
// cross-service clients, MQTT/simulator/ingest are exercised separately in
// internal/ingest's own tests.
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

type deviceFixture struct {
	ID         string `json:"id"`
	DeviceCode string `json:"device_code"`
	DeviceType string `json:"device_type"`
	Status     string `json:"status"`
}

func mustSeedDevice(t *testing.T, srv *httptest.Server, companyID, deviceType string) deviceFixture {
	t.Helper()
	code := "DEV-" + uuid.NewString()[:8]
	resp := postJSON(t, srv.URL+"/devices", map[string]any{
		"company_id": companyID, "device_code": code, "device_type": deviceType, "name": "Test Device " + code,
	})
	requireStatus(t, resp, http.StatusCreated)
	var d deviceFixture
	resp.decode(t, &d)
	return d
}

// mustSeedReading inserts a reading directly via SQL (there's no POST
// /readings endpoint by design -- readings only ever arrive through the
// MQTT ingest path, see internal/ingest) so listReadings/alert fixtures
// have something to query.
func mustSeedReading(t *testing.T, deviceID, companyID, readingType string, valueNumeric *float64, valueText *string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO readings (device_id, company_id, reading_type, value_numeric, value_text, recorded_at)
		VALUES ($1, $2, $3, $4, $5, now())
		RETURNING id`,
		deviceID, companyID, readingType, valueNumeric, valueText,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed reading: %v", err)
	}
	return id
}

// mustSeedAlert inserts an OPEN alert directly via SQL for the same reason
// as mustSeedReading -- alerts are only ever created by internal/ingest.
func mustSeedAlert(t *testing.T, deviceID, companyID string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO alerts (device_id, company_id, alert_type, severity, message)
		VALUES ($1, $2, 'THRESHOLD_BREACH', 'MEDIUM', 'Test alert')
		RETURNING id`,
		deviceID, companyID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed alert: %v", err)
	}
	return id
}
