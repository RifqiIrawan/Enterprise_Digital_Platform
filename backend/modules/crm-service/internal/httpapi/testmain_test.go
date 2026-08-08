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

	"github.com/enterprise-digital-platform/crm-service/internal/httpapi"
	"github.com/enterprise-digital-platform/crm-service/internal/store"
	"github.com/enterprise-digital-platform/crm-service/migrations"
)

var pool *pgxpool.Pool

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/crm_service_test?sslmode=disable"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	adminURL := getEnv("CRM_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: crm-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: crm-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE crm_service_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create crm_service_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("CRM_TEST_DATABASE_URL", testDatabaseURL)
	pool, err = store.Connect(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to crm_service_test: %v\n", err)
		os.Exit(0)
	}
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Printf("FAIL: migration of crm_service_test failed: %v\n", err)
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

// newServer wires the real handler; events publisher passed as nil is safe
// (eventbus.Publisher.Publish/Close are nil-receiver-safe).
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

type leadFixture struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func mustSeedLead(t *testing.T, srv *httptest.Server, companyID string) leadFixture {
	t.Helper()
	resp := postJSON(t, srv.URL+"/leads", map[string]any{
		"company_id": companyID, "first_name": "Test", "last_name": "Lead", "company_name": "Test Co",
	})
	requireStatus(t, resp, http.StatusCreated)
	var l leadFixture
	resp.decode(t, &l)
	return l
}

// mustSetLeadStatus updates a lead's status via the real HTTP endpoint,
// used to put a lead into QUALIFIED for convert tests. updateLead validates
// source against an explicit allow-list with no default for blank (same
// idiom as asset-service's updateAsset requiring status resent every time),
// so callers must always resend a valid source too.
func mustSetLeadStatus(t *testing.T, srv *httptest.Server, leadID, status string) {
	t.Helper()
	requireStatus(t, doRequest(t, http.MethodPut, srv.URL+"/leads/"+leadID, map[string]any{
		"first_name": "Test", "last_name": "Lead", "company_name": "Test Co", "source": "OTHER", "status": status,
	}, ""), http.StatusOK)
}

type accountFixture struct {
	ID          string `json:"id"`
	AccountCode string `json:"account_code"`
}

func mustSeedAccount(t *testing.T, srv *httptest.Server, companyID string) accountFixture {
	t.Helper()
	code := "ACC-" + uuid.NewString()[:8]
	resp := postJSON(t, srv.URL+"/accounts", map[string]any{
		"company_id": companyID, "account_code": code, "name": "Test Account " + code,
	})
	requireStatus(t, resp, http.StatusCreated)
	var a accountFixture
	resp.decode(t, &a)
	return a
}

type contactFixture struct {
	ID string `json:"id"`
}

func mustSeedContact(t *testing.T, srv *httptest.Server, companyID, accountID string) contactFixture {
	t.Helper()
	resp := postJSON(t, srv.URL+"/contacts", map[string]any{
		"company_id": companyID, "account_id": accountID, "first_name": "Test", "last_name": "Contact",
	})
	requireStatus(t, resp, http.StatusCreated)
	var c contactFixture
	resp.decode(t, &c)
	return c
}

type opportunityFixture struct {
	ID    string `json:"id"`
	Stage string `json:"stage"`
}

func mustSeedOpportunity(t *testing.T, srv *httptest.Server, companyID, accountID string) opportunityFixture {
	t.Helper()
	resp := postJSON(t, srv.URL+"/opportunities", map[string]any{
		"company_id": companyID, "account_id": accountID, "name": "Test Deal", "amount": 1000,
	})
	requireStatus(t, resp, http.StatusCreated)
	var o opportunityFixture
	resp.decode(t, &o)
	return o
}
