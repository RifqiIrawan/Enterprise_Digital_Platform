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

	"github.com/enterprise-digital-platform/auth-service/internal/httpapi"
	"github.com/enterprise-digital-platform/auth-service/internal/store"
	"github.com/enterprise-digital-platform/auth-service/migrations"
)

var pool *pgxpool.Pool

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/auth_service_test?sslmode=disable"

	testJWTSecret  = "secret-khusus-test"
	testAccessTTL  = 15 * time.Minute
	testRefreshTTL = 24 * time.Hour
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	adminURL := getEnv("AUTH_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: auth-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: auth-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE auth_service_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create auth_service_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("AUTH_TEST_DATABASE_URL", testDatabaseURL)
	pool, err = store.Connect(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to auth_service_test: %v\n", err)
		os.Exit(0)
	}
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Printf("FAIL: migration of auth_service_test failed: %v\n", err)
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
	httpapi.NewHandler(pool, nil, testJWTSecret, testAccessTTL, testRefreshTTL).Register(mux)
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
	return sendRequest(t, method, url, body)
}

// doRawRequest mengirim body apa adanya supaya test bisa menguji payload yang
// memang tidak valid sebagai JSON.
func doRawRequest(t *testing.T, method, url, body string) apiResponse {
	t.Helper()
	return sendRequest(t, method, url, strings.NewReader(body))
}

func sendRequest(t *testing.T, method, url string, body io.Reader) apiResponse {
	t.Helper()
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

func postJSONWithHeaders(t *testing.T, url string, payload any, headers map[string]string) apiResponse {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
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
		t.Fatalf("do request POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return apiResponse{status: resp.StatusCode, body: respBody}
}

func putJSON(t *testing.T, url string, payload any) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodPut, url, payload)
}

func postRawJSON(t *testing.T, url, body string) apiResponse {
	t.Helper()
	return doRawRequest(t, http.MethodPost, url, body)
}

func requireStatus(t *testing.T, resp apiResponse, want int) {
	t.Helper()
	if resp.status != want {
		t.Fatalf("expected status %d, got %d (body: %s)", want, resp.status, resp.body)
	}
}

type userFixture struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	Username     string     `json:"username"`
	FullName     string     `json:"full_name"`
	Phone        string     `json:"phone"`
	IsSuperAdmin bool       `json:"is_super_admin"`
	Status       string     `json:"status"`
	LastLoginAt  *time.Time `json:"last_login_at"`

	MustChangePassword bool `json:"must_change_password"`
}

type loginResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	TokenType    string      `json:"token_type"`
	ExpiresIn    int         `json:"expires_in"`
	User         userFixture `json:"user"`
}

const testPassword = "Rahasia@12345"

// testEmail: kolom email DAN username sama-sama UNIQUE, dan username diturunkan
// dari bagian sebelum "@". Bagian lokal yang acak per pemanggilan membuat tiap
// test berdiri sendiri terhadap database yang tidak di-drop antar run.
func testEmail() string {
	return "qa-" + strings.ToLower(uuid.NewString()[:12]) + "@edp.test"
}

func mustCreateUser(t *testing.T, srv *httptest.Server) (userFixture, string) {
	t.Helper()
	email := testEmail()
	resp := postJSON(t, srv.URL+"/users", map[string]any{
		"email":     email,
		"full_name": "Karyawan QA",
		"phone":     "08123456789",
		"password":  testPassword,
	})
	requireStatus(t, resp, http.StatusCreated)
	var user userFixture
	resp.decode(t, &user)
	t.Cleanup(func() {
		// refresh_tokens ikut terhapus lewat ON DELETE CASCADE.
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
	})
	return user, email
}

func mustLogin(t *testing.T, srv *httptest.Server, email, password string) loginResponse {
	t.Helper()
	resp := postJSON(t, srv.URL+"/login", map[string]any{"email": email, "password": password})
	requireStatus(t, resp, http.StatusOK)
	var out loginResponse
	resp.decode(t, &out)
	return out
}

func setUserStatus(t *testing.T, userID, status string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE users SET status = $1 WHERE id = $2`, status, userID); err != nil {
		t.Fatalf("ubah status user: %v", err)
	}
}
