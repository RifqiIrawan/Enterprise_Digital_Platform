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

	"github.com/enterprise-digital-platform/finance-service/internal/httpapi"
	"github.com/enterprise-digital-platform/finance-service/internal/store"
	"github.com/enterprise-digital-platform/finance-service/migrations"
)

// pool adalah koneksi ke database test (finance_service_test), dipakai
// bersama oleh semua test file di package ini. Diisi oleh TestMain.
var pool *pgxpool.Pool

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/finance_service_test?sslmode=disable"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	adminURL := getEnv("FINANCE_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: finance-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: finance-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE finance_service_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create finance_service_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("FINANCE_TEST_DATABASE_URL", testDatabaseURL)
	pool, err = store.Connect(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to finance_service_test: %v\n", err)
		os.Exit(0)
	}
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Printf("FAIL: migration of finance_service_test failed: %v\n", err)
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

// newCompanyID gives each test its own random company_id so tests never
// collide on the (company_id, account_code)/(company_id, entry_number)/
// (company_id, invoice_number) unique constraints and don't need any
// TRUNCATE between runs.
func newCompanyID(t *testing.T) string {
	t.Helper()
	return uuid.NewString()
}

// newServer wires the real handler (with events disabled, see NewHandler
// nil-events note below) into an httptest server, so requests go through
// actual routing + JSON encode/decode exactly like production.
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	// events=nil is safe: eventbus.Publisher.Publish checks p == nil before
	// touching its writer (internal/eventbus/eventbus.go), so no Kafka
	// broker is required for these tests.
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

type accountFixture struct {
	ID          string `json:"id"`
	AccountCode string `json:"account_code"`
	AccountType string `json:"account_type"`
}

// mustSeedAccount creates a posting account for companyID via the real HTTP
// endpoint (not a direct DB insert), so fixtures exercise the same code
// path as everything else and stay valid if createAccount's shape changes.
func mustSeedAccount(t *testing.T, srv *httptest.Server, companyID, accountType string) accountFixture {
	t.Helper()
	resp := postJSON(t, srv.URL+"/accounts", map[string]any{
		"company_id":   companyID,
		"account_code": "ACC-" + uuid.NewString()[:8],
		"account_name": "Test Account " + accountType,
		"account_type": accountType,
	})
	requireStatus(t, resp, http.StatusCreated)
	var acc accountFixture
	resp.decode(t, &acc)
	return acc
}
