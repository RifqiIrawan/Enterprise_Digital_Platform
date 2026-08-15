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

	"github.com/enterprise-digital-platform/project-service/internal/financeclient"
	"github.com/enterprise-digital-platform/project-service/internal/hrclient"
	"github.com/enterprise-digital-platform/project-service/internal/httpapi"
	"github.com/enterprise-digital-platform/project-service/internal/store"
	"github.com/enterprise-digital-platform/project-service/migrations"
)

var pool *pgxpool.Pool

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/project_service_test?sslmode=disable"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	adminURL := getEnv("PROJECT_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: project-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: project-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE project_service_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create project_service_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("PROJECT_TEST_DATABASE_URL", testDatabaseURL)
	pool, err = store.Connect(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to project_service_test: %v\n", err)
		os.Exit(0)
	}
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Printf("FAIL: migration of project_service_test failed: %v\n", err)
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

// hrStubConfig menggambarkan hr-service palsu. Default-nya jalur bahagia:
// karyawan ada, milik company yang diminta, ACTIVE.
type hrStubConfig struct {
	employeeID  string
	companyID   string
	firstName   string
	lastName    string
	status      string
	basicSalary float64
	fails       bool
	notFound    bool
}

// financeStubConfig menggambarkan finance-service palsu. postFails meniru
// kegagalan pada langkah KEDUA (POST /journal-entries/{id}/post), yang penting
// dibedakan dari kegagalan langkah pertama: jurnal sudah terlanjur dibuat
// sebagai DRAFT di sana, jadi pemanggil tetap harus memperlakukannya gagal.
type financeStubConfig struct {
	journalEntryID string
	createFails    bool
	postFails      bool
}

type stubCall struct {
	method string
	path   string
	actor  string
	body   map[string]any
}

func newHRStub(t *testing.T, cfg hrStubConfig) (*httptest.Server, *[]stubCall) {
	t.Helper()
	calls := &[]stubCall{}
	mux := http.NewServeMux()

	// Bentuk response SENGAJA meniru model.Employee hr-service yang asli:
	// FLAT (tidak dibungkus key "employee") dan nama terpisah first_name/
	// last_name. Stub dengan bentuk lain akan membuat bug client lolos diam-diam.
	mux.HandleFunc("GET /employees/{id}", func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, stubCall{method: http.MethodGet, path: r.URL.Path})
		if cfg.fails {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"simulated hr-service failure"}`))
			return
		}
		if cfg.notFound {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Karyawan tidak ditemukan"}`))
			return
		}
		writeStubJSON(w, http.StatusOK, map[string]any{
			"id":            cfg.employeeID,
			"company_id":    cfg.companyID,
			"employee_code": "EMP-001",
			"first_name":    cfg.firstName,
			"last_name":     cfg.lastName,
			"status":        cfg.status,
			"basic_salary":  cfg.basicSalary,
			"is_active":     cfg.status == "ACTIVE",
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, calls
}

func newFinanceStub(t *testing.T, cfg financeStubConfig) (*httptest.Server, *[]stubCall) {
	t.Helper()
	calls := &[]stubCall{}
	mux := http.NewServeMux()

	mux.HandleFunc("POST /journal-entries", func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		*calls = append(*calls, stubCall{method: http.MethodPost, path: r.URL.Path, actor: r.Header.Get("X-User-Id"), body: body})
		if cfg.createFails {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"simulated finance-service failure"}`))
			return
		}
		writeStubJSON(w, http.StatusCreated, map[string]any{"id": cfg.journalEntryID, "status": "DRAFT"})
	})

	mux.HandleFunc("POST /journal-entries/{id}/post", func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, stubCall{method: http.MethodPost, path: r.URL.Path, actor: r.Header.Get("X-User-Id")})
		if cfg.postFails {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"Journal entry sudah POSTED"}`))
			return
		}
		writeStubJSON(w, http.StatusOK, map[string]any{"id": cfg.journalEntryID, "status": "POSTED"})
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

// newServer memasang handler asli dengan KEDUA client menunjuk alamat yang
// tidak bisa dihubungi -- benar untuk semua endpoint yang tidak menyentuh
// service lain. Yang menyentuhnya memakai newServerWithStubs.
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	return newServerWith(t, hrclient.New("http://127.0.0.1:1"), financeclient.New("http://127.0.0.1:1"))
}

func newServerWith(t *testing.T, hr *hrclient.Client, finance *financeclient.Client) *httptest.Server {
	t.Helper()
	handler := httpapi.NewHandler(pool, nil, hr, finance)
	mux := http.NewServeMux()
	handler.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newServerWithStubs(t *testing.T, hrCfg hrStubConfig, financeCfg financeStubConfig) (*httptest.Server, *[]stubCall, *[]stubCall) {
	t.Helper()
	hrStub, hrCalls := newHRStub(t, hrCfg)
	financeStub, financeCalls := newFinanceStub(t, financeCfg)
	return newServerWith(t, hrclient.New(hrStub.URL), financeclient.New(financeStub.URL)), hrCalls, financeCalls
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

type projectFixture struct {
	ID           string  `json:"id"`
	ProjectCode  string  `json:"project_code"`
	Name         string  `json:"name"`
	Status       string  `json:"status"`
	BudgetAmount float64 `json:"budget_amount"`
	ActualCost   float64 `json:"actual_cost"`
	ManagerName  *string `json:"manager_name"`
	ManagerID    *string `json:"manager_employee_id"`
	CompletedAt  *string `json:"completed_at"`
	EndDate      *string `json:"end_date"`
	Notes        string  `json:"notes"`
}

type taskFixture struct {
	ID           string  `json:"id"`
	TaskNumber   string  `json:"task_number"`
	Title        string  `json:"title"`
	Status       string  `json:"status"`
	Priority     string  `json:"priority"`
	ProjectID    string  `json:"project_id"`
	AssigneeName *string `json:"assignee_name"`
	CompletedAt  *string `json:"completed_at"`
}

type timesheetFixture struct {
	ID             string  `json:"id"`
	ProjectID      string  `json:"project_id"`
	TaskID         *string `json:"task_id"`
	EmployeeName   string  `json:"employee_name"`
	Hours          float64 `json:"hours"`
	HourlyRate     float64 `json:"hourly_rate"`
	Amount         float64 `json:"amount"`
	Status         string  `json:"status"`
	JournalEntryID *string `json:"journal_entry_id"`
}

type postCostFixture struct {
	Project        projectFixture `json:"project"`
	JournalEntryID string         `json:"journal_entry_id"`
	PostedCount    int            `json:"posted_count"`
	PostedAmount   float64        `json:"posted_amount"`
}

func mustSeedProject(t *testing.T, srv *httptest.Server, companyID string) projectFixture {
	t.Helper()
	resp := postJSON(t, srv.URL+"/projects", map[string]any{
		"company_id":    companyID,
		"project_code":  "PRJ-" + uuid.NewString()[:8],
		"name":          "Migrasi Data Center",
		"budget_amount": 100000000,
	})
	requireStatus(t, resp, http.StatusCreated)
	var p projectFixture
	resp.decode(t, &p)
	return p
}

// mustSeedActiveProject: banyak aturan modul ini hanya berlaku pada proyek
// ACTIVE (timesheet, posting biaya), jadi jalur PLANNING -> ACTIVE dipakai
// berulang kali sebagai titik awal.
func mustSeedActiveProject(t *testing.T, srv *httptest.Server, companyID string) projectFixture {
	t.Helper()
	p := mustSeedProject(t, srv, companyID)
	resp := postJSON(t, srv.URL+"/projects/"+p.ID+"/activate", nil)
	requireStatus(t, resp, http.StatusOK)
	resp.decode(t, &p)
	return p
}

func mustSeedTask(t *testing.T, srv *httptest.Server, companyID, projectID string) taskFixture {
	t.Helper()
	resp := postJSON(t, srv.URL+"/tasks", map[string]any{
		"company_id":      companyID,
		"project_id":      projectID,
		"title":           "Inventarisasi server lama",
		"estimated_hours": 8,
	})
	requireStatus(t, resp, http.StatusCreated)
	var task taskFixture
	resp.decode(t, &task)
	return task
}

func fetchProject(t *testing.T, srv *httptest.Server, projectID string) projectFixture {
	t.Helper()
	resp := getJSON(t, srv.URL+"/projects/"+projectID)
	requireStatus(t, resp, http.StatusOK)
	var p projectFixture
	resp.decode(t, &p)
	return p
}

// testServerBundle menyatukan server dengan daftar panggilan ke kedua stub,
// supaya test yang butuh memeriksa "apa yang benar-benar dikirim ke
// finance-service" tidak perlu meneruskan tiga nilai terpisah ke mana-mana.
type testServerBundle struct {
	srv          *httptest.Server
	hrCalls      *[]stubCall
	financeCalls *[]stubCall
}

func mustSeedTimesheet(t *testing.T, srv *httptest.Server, companyID, projectID, employeeID string, hours float64) timesheetFixture {
	t.Helper()
	resp := postJSON(t, srv.URL+"/timesheets", map[string]any{
		"company_id":  companyID,
		"project_id":  projectID,
		"employee_id": employeeID,
		"hours":       hours,
		"description": "Pengerjaan modul",
	})
	requireStatus(t, resp, http.StatusCreated)
	var ts timesheetFixture
	resp.decode(t, &ts)
	return ts
}

func fetchTimesheets(t *testing.T, srv *httptest.Server, companyID, projectID string) []timesheetFixture {
	t.Helper()
	resp := getJSON(t, srv.URL+"/timesheets?company_id="+companyID+"&project_id="+projectID)
	requireStatus(t, resp, http.StatusOK)
	var out []timesheetFixture
	resp.decode(t, &out)
	return out
}
