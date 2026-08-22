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

	"github.com/enterprise-digital-platform/company-service/internal/httpapi"
	"github.com/enterprise-digital-platform/company-service/internal/store"
	"github.com/enterprise-digital-platform/company-service/migrations"
)

var pool *pgxpool.Pool

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/company_service_test?sslmode=disable"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	adminURL := getEnv("COMPANY_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: company-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: company-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE company_service_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create company_service_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("COMPANY_TEST_DATABASE_URL", testDatabaseURL)
	pool, err = store.Connect(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to company_service_test: %v\n", err)
		os.Exit(0)
	}
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Printf("FAIL: migration of company_service_test failed: %v\n", err)
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

// newServer memasang handler asli dengan publisher nil. Publisher.Publish
// sudah nil-safe (lihat internal/eventbus), jadi test tidak butuh Kafka --
// sama seperti service lain yang publish best-effort.
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	httpapi.NewHandler(pool, nil).Register(mux)
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

func doRequest(t *testing.T, method, url string, payload any) apiResponse {
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

func getJSON(t *testing.T, url string) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodGet, url, nil)
}

func postJSON(t *testing.T, url string, payload any) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodPost, url, payload)
}

func putJSON(t *testing.T, url string, payload any) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodPut, url, payload)
}

func deleteJSON(t *testing.T, url string) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodDelete, url, nil)
}

func requireStatus(t *testing.T, resp apiResponse, want int) {
	t.Helper()
	if resp.status != want {
		t.Fatalf("expected status %d, got %d (body: %s)", want, resp.status, resp.body)
	}
}

type companyFixture struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type branchFixture struct {
	ID        string `json:"id"`
	CompanyID string `json:"company_id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Status    string `json:"status"`
}

type departmentFixture struct {
	ID        string  `json:"id"`
	CompanyID string  `json:"company_id"`
	BranchID  *string `json:"branch_id"`
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Status    string  `json:"status"`
}

// shortCode: tabel companies/branches/departments punya UNIQUE pada code, dan
// test dijalankan berulang kali terhadap database yang sama (tidak di-drop di
// antara run). Kode acak per pemanggilan membuat tiap test berdiri sendiri
// tanpa perlu TRUNCATE yang bisa menghapus data run yang berjalan paralel.
func shortCode(prefix string) string {
	return prefix + "-" + strings.ToUpper(uuid.NewString()[:8])
}

func mustSeedCompany(t *testing.T, srv *httptest.Server) companyFixture {
	t.Helper()
	resp := postJSON(t, srv.URL+"/companies", map[string]any{
		"code": shortCode("CMP"),
		"name": "PT Uji Coba",
	})
	requireStatus(t, resp, http.StatusCreated)
	var c companyFixture
	resp.decode(t, &c)
	return c
}

func mustSeedBranch(t *testing.T, srv *httptest.Server, companyID string) branchFixture {
	t.Helper()
	resp := postJSON(t, srv.URL+"/companies/"+companyID+"/branches", map[string]any{
		"code":    shortCode("BR"),
		"name":    "Cabang Pusat",
		"address": "Jl. Sudirman 1",
	})
	requireStatus(t, resp, http.StatusCreated)
	var b branchFixture
	resp.decode(t, &b)
	return b
}

func mustSeedDepartment(t *testing.T, srv *httptest.Server, companyID string, branchID *string) departmentFixture {
	t.Helper()
	resp := postJSON(t, srv.URL+"/companies/"+companyID+"/departments", map[string]any{
		"code":      shortCode("DEP"),
		"name":      "Keuangan",
		"branch_id": branchID,
	})
	requireStatus(t, resp, http.StatusCreated)
	var d departmentFixture
	resp.decode(t, &d)
	return d
}

func fetchBranches(t *testing.T, srv *httptest.Server, companyID string) []branchFixture {
	t.Helper()
	resp := getJSON(t, srv.URL+"/companies/"+companyID+"/branches")
	requireStatus(t, resp, http.StatusOK)
	var out []branchFixture
	resp.decode(t, &out)
	return out
}

func fetchDepartments(t *testing.T, srv *httptest.Server, companyID string) []departmentFixture {
	t.Helper()
	resp := getJSON(t, srv.URL+"/companies/"+companyID+"/departments")
	requireStatus(t, resp, http.StatusOK)
	var out []departmentFixture
	resp.decode(t, &out)
	return out
}
