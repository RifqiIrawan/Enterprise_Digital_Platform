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

	"github.com/enterprise-digital-platform/rbac-service/internal/httpapi"
	"github.com/enterprise-digital-platform/rbac-service/internal/store"
	"github.com/enterprise-digital-platform/rbac-service/migrations"
)

var pool *pgxpool.Pool

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/rbac_service_test?sslmode=disable"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	adminURL := getEnv("RBAC_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: rbac-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: rbac-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE rbac_service_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create rbac_service_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("RBAC_TEST_DATABASE_URL", testDatabaseURL)
	pool, err = store.Connect(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to rbac_service_test: %v\n", err)
		os.Exit(0)
	}
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Printf("FAIL: migration of rbac_service_test failed: %v\n", err)
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

func doRequest(t *testing.T, method, url string, payload any, headers map[string]string) apiResponse {
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
	for k, v := range headers {
		req.Header.Set(k, v)
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

func getJSON(t *testing.T, url string) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodGet, url, nil, nil)
}

func getJSONWithHeaders(t *testing.T, url string, headers map[string]string) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodGet, url, nil, headers)
}

func postJSON(t *testing.T, url string, payload any) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodPost, url, payload, nil)
}

func postJSONWithHeaders(t *testing.T, url string, payload any, headers map[string]string) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodPost, url, payload, headers)
}

func putJSON(t *testing.T, url string, payload any) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodPut, url, payload, nil)
}

func deleteJSON(t *testing.T, url string) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodDelete, url, nil, nil)
}

// fetchMenuTreeForCompany memanggil menu-tree dengan company_id, jalur yang
// memperhitungkan override permission per user.
func fetchMenuTreeForCompany(t *testing.T, srv *httptest.Server, userID, companyID string) []menuTreeModule {
	t.Helper()
	resp := getJSON(t, srv.URL+"/menu-tree?user_id="+userID+"&company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var tree []menuTreeModule
	resp.decode(t, &tree)
	return tree
}

// doRawRequest mengirim body apa adanya (tanpa lewat json.Marshal) supaya test
// bisa menguji payload yang memang tidak valid sebagai JSON.
func doRawRequest(t *testing.T, method, url, body string) apiResponse {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
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

func postRawJSON(t *testing.T, url, body string) apiResponse {
	t.Helper()
	return doRawRequest(t, http.MethodPost, url, body)
}

func putRawJSON(t *testing.T, url, body string) apiResponse {
	t.Helper()
	return doRawRequest(t, http.MethodPut, url, body)
}

func requireStatus(t *testing.T, resp apiResponse, want int) {
	t.Helper()
	if resp.status != want {
		t.Fatalf("expected status %d, got %d (body: %s)", want, resp.status, resp.body)
	}
}

type roleFixture struct {
	ID          string  `json:"id"`
	CompanyID   *string `json:"company_id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	IsSystem    bool    `json:"is_system"`
}

type menuPermission struct {
	ID         string `json:"id"`
	ModuleID   string `json:"module_id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Icon       string `json:"icon"`
	SortOrder  int    `json:"sort_order"`
	CanView    bool   `json:"can_view"`
	CanCreate  bool   `json:"can_create"`
	CanUpdate  bool   `json:"can_update"`
	CanDelete  bool   `json:"can_delete"`
	CanApprove bool   `json:"can_approve"`
	CanExport  bool   `json:"can_export"`
}

type userRoleFixture struct {
	ID        string  `json:"id"`
	UserID    string  `json:"user_id"`
	RoleID    string  `json:"role_id"`
	RoleCode  string  `json:"role_code"`
	RoleName  string  `json:"role_name"`
	CompanyID string  `json:"company_id"`
	BranchID  *string `json:"branch_id"`
}

type menuTreeModule struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Menus []menuTreeEntry `json:"menus"`
}

type menuTreeEntry struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Path     string          `json:"path"`
	Icon     string          `json:"icon"`
	Children []menuTreeEntry `json:"children"`
}

// roleCode: tabel roles punya UNIQUE (COALESCE(company_id, ...), code) dan test
// dijalankan berulang kali terhadap database yang sama (tidak di-drop di antara
// run). Kode acak per pemanggilan membuat tiap test berdiri sendiri tanpa perlu
// TRUNCATE yang bisa menghapus data seed migrasi.
func roleCode(prefix string) string {
	return prefix + "_" + strings.ToLower(uuid.NewString()[:8])
}

func mustCreateRole(t *testing.T, srv *httptest.Server) roleFixture {
	t.Helper()
	resp := postJSON(t, srv.URL+"/roles", map[string]any{
		"code":        roleCode("qa_role"),
		"name":        "QA Role",
		"description": "Role bikinan test",
	})
	requireStatus(t, resp, http.StatusCreated)
	var role roleFixture
	resp.decode(t, &role)
	t.Cleanup(func() {
		// Role custom (is_system = false) boleh dihapus; cascade ikut
		// membersihkan role_menu_permissions & user_roles miliknya.
		_, _ = pool.Exec(context.Background(), `DELETE FROM roles WHERE id = $1`, role.ID)
	})
	return role
}

// systemRoleID mengambil id role bawaan platform hasil seed 002.
func systemRoleID(t *testing.T, code string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM roles WHERE code = $1 AND is_system = true`, code).Scan(&id); err != nil {
		t.Fatalf("cari role sistem %q: %v", code, err)
	}
	return id
}

// menuIDByCode mengambil id menu hasil seed berdasarkan module + code menu.
func menuIDByCode(t *testing.T, moduleCode, menuCode string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		SELECT m.id FROM menus m
		JOIN modules mod ON mod.id = m.module_id
		WHERE mod.code = $1 AND m.code = $2`, moduleCode, menuCode).Scan(&id); err != nil {
		t.Fatalf("cari menu %s/%s: %v", moduleCode, menuCode, err)
	}
	return id
}
