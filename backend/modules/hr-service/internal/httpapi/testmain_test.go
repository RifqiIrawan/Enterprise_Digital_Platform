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

	"github.com/enterprise-digital-platform/hr-service/internal/financeclient"
	"github.com/enterprise-digital-platform/hr-service/internal/httpapi"
	"github.com/enterprise-digital-platform/hr-service/internal/store"
	"github.com/enterprise-digital-platform/hr-service/migrations"
)

var pool *pgxpool.Pool

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/hr_service_test?sslmode=disable"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	adminURL := getEnv("HR_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: hr-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: hr-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE hr_service_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create hr_service_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("HR_TEST_DATABASE_URL", testDatabaseURL)
	pool, err = store.Connect(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to hr_service_test: %v\n", err)
		os.Exit(0)
	}
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Printf("FAIL: migration of hr_service_test failed: %v\n", err)
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

// newServer wires the real handler with a financeclient pointed at an
// unreachable address — fine for every endpoint except postPayrollRun,
// which has its own dedicated setup (newServerWithFinanceStub) since it's
// the only handler that calls out to finance-service.
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	finance := financeclient.New("http://127.0.0.1:1")
	handler := httpapi.NewHandler(pool, nil, finance)
	mux := http.NewServeMux()
	handler.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// financeStubCall records one request the handler sent to the (stubbed)
// finance-service, so tests can assert on the journal lines hr-service built.
type financeStubCall struct {
	path string
	body []byte
}

// newFinanceStub fakes just enough of finance-service's contract
// (POST /journal-entries -> {id,status}, POST /journal-entries/{id}/post -> 200)
// for financeclient.CreateAndPostJournalEntry to succeed, without pulling in
// finance-service's actual implementation (services stay independently
// testable via HTTP, same as they talk to each other in production).
// If failOnCreate is true, the first call returns 500 to exercise the
// "finance-service unreachable/erroring" path.
func newFinanceStub(t *testing.T, failOnCreate bool) (*httptest.Server, *[]financeStubCall) {
	t.Helper()
	calls := &[]financeStubCall{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /journal-entries", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*calls = append(*calls, financeStubCall{path: r.URL.Path, body: body})
		if failOnCreate {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"simulated finance-service failure"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": uuid.NewString(), "status": "DRAFT"})
	})
	mux.HandleFunc("POST /journal-entries/{id}/post", func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, financeStubCall{path: r.URL.Path})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "POSTED"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, calls
}

func newServerWithFinanceStub(t *testing.T, failOnCreate bool) (*httptest.Server, *[]financeStubCall) {
	t.Helper()
	stub, calls := newFinanceStub(t, failOnCreate)
	finance := financeclient.New(stub.URL)
	handler := httpapi.NewHandler(pool, nil, finance)
	mux := http.NewServeMux()
	handler.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, calls
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

type employeeFixture struct {
	ID           string `json:"id"`
	EmployeeCode string `json:"employee_code"`
	Status       string `json:"status"`
}

// mustSeedEmployee creates an ACTIVE employee via the real HTTP endpoint.
func mustSeedEmployee(t *testing.T, srv *httptest.Server, companyID string, basicSalary, monthlyAllowance float64) employeeFixture {
	t.Helper()
	code := "EMP-" + uuid.NewString()[:8]
	resp := postJSON(t, srv.URL+"/employees", map[string]any{
		"company_id":        companyID,
		"employee_code":     code,
		"first_name":        "Test",
		"last_name":         "Employee",
		"email":             code + "@example.test",
		"employment_type":   "PERMANENT",
		"hire_date":         "2020-01-01",
		"basic_salary":      basicSalary,
		"monthly_allowance": monthlyAllowance,
		"ptkp_status":       "TK/0",
	})
	requireStatus(t, resp, http.StatusCreated)
	var e employeeFixture
	resp.decode(t, &e)
	return e
}
