package gateway_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/enterprise-digital-platform/api-gateway/internal/config"
	"github.com/enterprise-digital-platform/api-gateway/internal/gateway"
)

const (
	testJWTSecret = "secret-khusus-test"
	testOrigin    = "https://app.contoh.test"
)

// recordingBackend berdiri sebagai service di belakang gateway dan menyimpan
// request terakhir yang diterimanya -- itulah yang diperiksa test: path sudah
// dipotong prefix-nya, header identitas sudah diisi gateway, dsb.
type recordingBackend struct {
	*httptest.Server

	mu      sync.Mutex
	method  string
	path    string
	query   string
	headers http.Header
	body    string
}

func newBackend(t *testing.T) *recordingBackend {
	t.Helper()
	b := &recordingBackend{}
	b.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		b.mu.Lock()
		b.method, b.path, b.query = r.Method, r.URL.Path, r.URL.RawQuery
		b.headers = r.Header.Clone()
		b.body = string(body)
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"from":"backend"}`))
	}))
	t.Cleanup(b.Close)
	return b
}

func (b *recordingBackend) last(t *testing.T) (method, path, query string, headers http.Header, body string) {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.method == "" {
		t.Fatal("backend tidak pernah menerima request")
	}
	return b.method, b.path, b.query, b.headers, b.body
}

// newGateway mengarahkan SELURUH prefix ke satu backend uji, supaya test bisa
// memakai prefix mana pun tanpa harus menyalakan 20 backend.
//
// AuthzEnforce default MATI di sini: test di file ini menguji routing, header
// identitas, CORS dan request id -- semuanya harus tetap benar terlepas dari
// hak akses pemanggil, dan menyalakannya berarti setiap test harus ikut
// menyediakan rbac-service palsu beserta hak yang pas untuk path yang
// kebetulan dipakainya. Penegakannya sendiri diuji di authz_test.go, yang
// menyalakannya lewat opts.
func newGateway(t *testing.T, target string, opts ...func(*config.Config)) *httptest.Server {
	t.Helper()
	cfg := &config.Config{
		AppEnv:               "test",
		AuthServiceURL:       target,
		CompanyServiceURL:    target,
		RBACServiceURL:       target,
		AuditServiceURL:      target,
		FinanceServiceURL:    target,
		HRServiceURL:         target,
		SalesServiceURL:      target,
		PurchasingServiceURL: target,
		WarehouseServiceURL:  target,
		ProductionServiceURL: target,
		QCServiceURL:         target,
		AssetServiceURL:      target,
		AIBIServiceURL:       target,
		IoTServiceURL:        target,
		DWServiceURL:         target,
		CRMServiceURL:        target,
		TicketingServiceURL:  target,
		EcommerceServiceURL:  target,
		FleetServiceURL:      target,
		ProjectServiceURL:    target,
		JWTSecret:            testJWTSecret,
		CORSAllowedOrigin:    testOrigin,
		AuthzEnforce:         false,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	srv := httptest.NewServer(gateway.New(cfg))
	t.Cleanup(srv.Close)
	return srv
}

type tokenOptions struct {
	secret       string
	subject      string
	email        string
	isSuperAdmin bool
	ttl          time.Duration
}

func issueToken(t *testing.T, opts tokenOptions) string {
	t.Helper()
	if opts.secret == "" {
		opts.secret = testJWTSecret
	}
	if opts.ttl == 0 {
		opts.ttl = time.Hour
	}
	claims := jwt.MapClaims{
		"sub":            opts.subject,
		"email":          opts.email,
		"is_super_admin": opts.isSuperAdmin,
		"exp":            jwt.NewNumericDate(time.Now().Add(opts.ttl)),
		"iat":            jwt.NewNumericDate(time.Now()),
		"iss":            "auth-service",
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(opts.secret))
	if err != nil {
		t.Fatalf("bikin token: %v", err)
	}
	return token
}

func do(t *testing.T, method, url string, headers map[string]string, body string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// Redirect tidak pernah diharapkan dari gateway; matikan supaya kalau
	// suatu saat muncul, test-nya gagal alih-alih diam-diam mengikutinya.
	client := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func requireStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status %d, got %d (body: %s)", want, resp.StatusCode, body)
	}
}

func TestHealthNeedsNoToken(t *testing.T) {
	backend := newBackend(t)
	gw := newGateway(t, backend.URL)

	resp := do(t, http.MethodGet, gw.URL+"/health", nil, "")
	requireStatus(t, resp, http.StatusOK)

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "api-gateway") {
		t.Errorf("expected the gateway's own health body, got %s", body)
	}
}

// Login & refresh harus bisa dipanggil sebelum client punya token sama sekali.
func TestPublicRoutesPassThroughWithoutAToken(t *testing.T) {
	backend := newBackend(t)
	gw := newGateway(t, backend.URL)

	for _, path := range []string{"/api/auth/login", "/api/auth/refresh"} {
		t.Run(path, func(t *testing.T) {
			resp := do(t, http.MethodPost, gw.URL+path, nil, `{"email":"a@b.c"}`)
			requireStatus(t, resp, http.StatusOK)
			_, gotPath, _, _, body := backend.last(t)
			if want := strings.TrimPrefix(path, "/api/auth"); gotPath != want {
				t.Errorf("expected backend path %q, got %q", want, gotPath)
			}
			if body != `{"email":"a@b.c"}` {
				t.Errorf("body tidak diteruskan utuh, got %q", body)
			}
		})
	}
}

// Daftar public route dicocokkan dengan METHOD + PATH persis: endpoint auth
// lain (logout, daftar user) tetap butuh token, begitu juga method lain di
// path yang sama.
func TestOnlyLoginAndRefreshArePublic(t *testing.T) {
	backend := newBackend(t)
	gw := newGateway(t, backend.URL)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/auth/logout"},
		{http.MethodGet, "/api/auth/users"},
		{http.MethodGet, "/api/auth/login"},
		{http.MethodPost, "/api/auth/login/extra"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resp := do(t, tc.method, gw.URL+tc.path, nil, "")
			requireStatus(t, resp, http.StatusUnauthorized)
		})
	}
}

func TestProtectedRoutesRejectBadTokens(t *testing.T) {
	backend := newBackend(t)
	gw := newGateway(t, backend.URL)

	cases := []struct {
		name    string
		headers map[string]string
	}{
		{"tanpa header", nil},
		{"tanpa prefix Bearer", map[string]string{"Authorization": issueToken(t, tokenOptions{subject: "u1"})}},
		{"Bearer kosong", map[string]string{"Authorization": "Bearer "}},
		{"token asal-asalan", bearer("bukan.sebuah.token")},
		{"secret berbeda", bearer(issueToken(t, tokenOptions{secret: "secret-lain", subject: "u1"}))},
		{"sudah kedaluwarsa", bearer(issueToken(t, tokenOptions{subject: "u1", ttl: -time.Minute}))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := do(t, http.MethodGet, gw.URL+"/api/hr/employees", tc.headers, "")
			requireStatus(t, resp, http.StatusUnauthorized)
			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("expected a JSON error, got Content-Type %q", ct)
			}
		})
	}
}

// Identitas hasil verifikasi token diteruskan sebagai header X-* -- inilah satu-
// satunya sumber identitas untuk service di belakang gateway (mis. menu-tree di
// rbac-service membaca X-Is-Super-Admin).
func TestValidTokenIsTranslatedIntoIdentityHeaders(t *testing.T) {
	backend := newBackend(t)
	gw := newGateway(t, backend.URL)

	token := issueToken(t, tokenOptions{subject: "user-123", email: "user@edp.test", isSuperAdmin: true})
	resp := do(t, http.MethodGet, gw.URL+"/api/rbac/menu-tree?user_id=user-123", bearer(token), "")
	requireStatus(t, resp, http.StatusOK)

	_, path, query, headers, _ := backend.last(t)
	if path != "/menu-tree" || query != "user_id=user-123" {
		t.Errorf("expected /menu-tree?user_id=user-123 di backend, got %s?%s", path, query)
	}
	if got := headers.Get("X-User-Id"); got != "user-123" {
		t.Errorf("expected X-User-Id user-123, got %q", got)
	}
	if got := headers.Get("X-User-Email"); got != "user@edp.test" {
		t.Errorf("expected X-User-Email user@edp.test, got %q", got)
	}
	if got := headers.Get("X-Is-Super-Admin"); got != "true" {
		t.Errorf("expected X-Is-Super-Admin true, got %q", got)
	}
}

// X-Is-Super-Admin selalu ditulis eksplisit, termasuk saat false -- kalau hanya
// diset ketika true, header sisa dari client bisa lolos apa adanya.
func TestNonSuperAdminGetsAnExplicitFalseHeader(t *testing.T) {
	backend := newBackend(t)
	gw := newGateway(t, backend.URL)

	token := issueToken(t, tokenOptions{subject: "user-biasa", email: "biasa@edp.test"})
	resp := do(t, http.MethodGet, gw.URL+"/api/hr/employees", bearer(token), "")
	requireStatus(t, resp, http.StatusOK)

	_, _, _, headers, _ := backend.last(t)
	if got := headers.Get("X-Is-Super-Admin"); got != "false" {
		t.Errorf("expected X-Is-Super-Admin false, got %q", got)
	}
}

// Header identitas yang dikirim client sendiri harus DITIMPA hasil verifikasi
// token: tanpa ini, siapa pun yang punya token user biasa bisa mengaku super
// admin (atau jadi user lain) hanya dengan menambah header.
func TestClientSuppliedIdentityHeadersAreOverwritten(t *testing.T) {
	backend := newBackend(t)
	gw := newGateway(t, backend.URL)

	token := issueToken(t, tokenOptions{subject: "user-asli", email: "asli@edp.test"})
	headers := bearer(token)
	headers["X-User-Id"] = "user-palsu"
	headers["X-User-Email"] = "palsu@edp.test"
	headers["X-Is-Super-Admin"] = "true"

	resp := do(t, http.MethodGet, gw.URL+"/api/finance/invoices", headers, "")
	requireStatus(t, resp, http.StatusOK)

	_, _, _, got, _ := backend.last(t)
	if got.Get("X-User-Id") != "user-asli" || got.Get("X-User-Email") != "asli@edp.test" {
		t.Errorf("identitas palsu dari client lolos: %s / %s", got.Get("X-User-Id"), got.Get("X-User-Email"))
	}
	if got.Get("X-Is-Super-Admin") != "false" {
		t.Errorf("klaim super admin palsu lolos: %q", got.Get("X-Is-Super-Admin"))
	}
}

func TestPrefixIsStrippedBeforeProxying(t *testing.T) {
	backend := newBackend(t)
	gw := newGateway(t, backend.URL)
	token := issueToken(t, tokenOptions{subject: "u1"})

	cases := []struct {
		requestPath string
		backendPath string
	}{
		{"/api/hr/leave-requests", "/leave-requests"},
		{"/api/hr/leave-requests/123/approve", "/leave-requests/123/approve"},
		{"/api/ai-bi/dashboards", "/dashboards"},
		// Prefix persis tanpa sisa apa pun tetap jadi "/" di backend, bukan
		// path kosong yang membuat request tidak valid.
		{"/api/hr", "/"},
	}
	for _, tc := range cases {
		t.Run(tc.requestPath, func(t *testing.T) {
			resp := do(t, http.MethodGet, gw.URL+tc.requestPath, bearer(token), "")
			requireStatus(t, resp, http.StatusOK)
			_, path, _, _, _ := backend.last(t)
			if path != tc.backendPath {
				t.Errorf("expected backend path %q, got %q", tc.backendPath, path)
			}
		})
	}
}

// Pencocokan prefix harus di batas segmen path: "/api/hrx" bukan milik
// hr-service, dan prefix yang tidak dikenal jadi 404, bukan salah alamat.
func TestUnknownPrefixesAreNotFound(t *testing.T) {
	backend := newBackend(t)
	gw := newGateway(t, backend.URL)
	token := issueToken(t, tokenOptions{subject: "u1"})

	for _, path := range []string{"/api/hrx/employees", "/api/entah/apa", "/api/"} {
		t.Run(path, func(t *testing.T) {
			resp := do(t, http.MethodGet, gw.URL+path, bearer(token), "")
			requireStatus(t, resp, http.StatusNotFound)
		})
	}
}

func TestRequestMethodAndBodyAreForwarded(t *testing.T) {
	backend := newBackend(t)
	gw := newGateway(t, backend.URL)
	token := issueToken(t, tokenOptions{subject: "u1"})

	payload := `{"nama":"Produk Uji","qty":3}`
	resp := do(t, http.MethodPut, gw.URL+"/api/warehouse/products/42", bearer(token), payload)
	requireStatus(t, resp, http.StatusOK)

	method, path, _, _, body := backend.last(t)
	if method != http.MethodPut || path != "/products/42" || body != payload {
		t.Errorf("request tidak diteruskan utuh: %s %s body=%q", method, path, body)
	}
}

// X-Request-Id mengikat log gateway dengan log service tujuan; dibuat di sini
// kalau client belum mengirimnya, lalu dikembalikan ke client juga.
func TestRequestIDIsGeneratedForwardedAndEchoed(t *testing.T) {
	backend := newBackend(t)
	gw := newGateway(t, backend.URL)
	token := issueToken(t, tokenOptions{subject: "u1"})

	resp := do(t, http.MethodGet, gw.URL+"/api/qc/inspections", bearer(token), "")
	requireStatus(t, resp, http.StatusOK)

	echoed := resp.Header.Get("X-Request-Id")
	if echoed == "" {
		t.Fatal("expected X-Request-Id in the response")
	}
	_, _, _, headers, _ := backend.last(t)
	if got := headers.Get("X-Request-Id"); got != echoed {
		t.Errorf("request id di backend (%q) beda dengan yang dikembalikan (%q)", got, echoed)
	}
}

func TestRequestIDFromTheCallerIsKept(t *testing.T) {
	backend := newBackend(t)
	gw := newGateway(t, backend.URL)
	token := issueToken(t, tokenOptions{subject: "u1"})

	headers := bearer(token)
	headers["X-Request-Id"] = "id-dari-client"

	resp := do(t, http.MethodGet, gw.URL+"/api/qc/inspections", headers, "")
	requireStatus(t, resp, http.StatusOK)

	if got := resp.Header.Get("X-Request-Id"); got != "id-dari-client" {
		t.Errorf("expected the caller's request id to be kept, got %q", got)
	}
	_, _, _, backendHeaders, _ := backend.last(t)
	if got := backendHeaders.Get("X-Request-Id"); got != "id-dari-client" {
		t.Errorf("request id client tidak diteruskan, got %q", got)
	}
}

// Service tujuan yang mati harus jadi 502 berbentuk JSON, bukan halaman error
// bawaan httputil yang tidak bisa dibaca frontend.
func TestUnreachableServiceBecomesAJSONBadGateway(t *testing.T) {
	// Port 1 dipilih karena praktis tidak pernah dilayani apa pun.
	gw := newGateway(t, "http://127.0.0.1:1")
	token := issueToken(t, tokenOptions{subject: "u1"})

	resp := do(t, http.MethodGet, gw.URL+"/api/sales/customers", bearer(token), "")
	requireStatus(t, resp, http.StatusBadGateway)

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "error") {
		t.Errorf("expected a JSON error body, got %s", body)
	}
}

func TestPreflightIsAnsweredWithoutAToken(t *testing.T) {
	backend := newBackend(t)
	gw := newGateway(t, backend.URL)

	resp := do(t, http.MethodOptions, gw.URL+"/api/hr/employees",
		map[string]string{"Origin": "http://localhost:3000"}, "")
	requireStatus(t, resp, http.StatusNoContent)

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("expected the localhost origin to be echoed, got %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") {
		t.Errorf("expected Authorization to be allowed, got %q", got)
	}
	if got := resp.Header.Get("Vary"); got != "Origin" {
		t.Errorf("expected Vary: Origin, got %q", got)
	}
}

// Dev lokal: Vite bisa naik di port mana pun kalau 3000 terpakai, jadi origin
// localhost/127.0.0.1 apa pun dipantulkan; selain itu dipakai nilai konfigurasi.
func TestCORSOriginHandling(t *testing.T) {
	backend := newBackend(t)
	gw := newGateway(t, backend.URL)

	cases := []struct {
		origin string
		want   string
	}{
		{"http://localhost:3000", "http://localhost:3000"},
		{"http://localhost:5173", "http://localhost:5173"},
		{"http://127.0.0.1:3002", "http://127.0.0.1:3002"},
		{"https://situs-lain.test", testOrigin},
		{"", testOrigin},
	}
	for _, tc := range cases {
		t.Run(tc.origin, func(t *testing.T) {
			headers := map[string]string{}
			if tc.origin != "" {
				headers["Origin"] = tc.origin
			}
			resp := do(t, http.MethodGet, gw.URL+"/health", headers, "")
			requireStatus(t, resp, http.StatusOK)
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != tc.want {
				t.Errorf("expected Allow-Origin %q, got %q", tc.want, got)
			}
		})
	}
}

func TestEveryConfiguredPrefixIsRouted(t *testing.T) {
	backend := newBackend(t)
	gw := newGateway(t, backend.URL)
	token := issueToken(t, tokenOptions{subject: "u1"})

	// Seluruh 20 prefix yang didaftarkan di gateway.New. Daftar ini sengaja
	// ditulis ulang di sini supaya modul baru yang lupa didaftarkan (atau
	// prefix yang salah ketik) ketahuan sebagai 404, bukan baru terasa saat
	// halamannya dibuka di browser.
	prefixes := []string{
		"auth", "company", "rbac", "audit", "finance", "hr", "sales", "purchasing",
		"warehouse", "production", "qc", "asset", "ai-bi", "iot", "dw", "crm",
		"ticketing", "ecommerce", "fleet", "project",
	}
	for _, prefix := range prefixes {
		t.Run(prefix, func(t *testing.T) {
			resp := do(t, http.MethodGet, gw.URL+"/api/"+prefix+"/ping", bearer(token), "")
			requireStatus(t, resp, http.StatusOK)
			_, path, _, _, _ := backend.last(t)
			if path != "/ping" {
				t.Errorf("expected /ping di backend, got %q", path)
			}
		})
	}
}
