package authz

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestPolicyTableIsWellFormed(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLookupPrefersLiteralSegmentsOverWildcards(t *testing.T) {
	// /api/dw/sync/status dan /api/dw/analytics/* sama-sama tiga segmen setelah
	// /api; tanpa pemilihan berdasarkan segmen literal, urutan penulisan di
	// tabel yang menentukan siapa yang menang -- dan itu berarti kebijakan bisa
	// berubah hanya karena satu baris dipindah.
	rule, ok := Lookup("GET", "/api/dw/sync/status")
	if !ok {
		t.Fatal("GET /api/dw/sync/status tidak ketemu")
	}
	if rule.Pattern != "/api/dw/sync/status" {
		t.Fatalf("cocok ke pola yang salah: %s", rule.Pattern)
	}

	rule, ok = Lookup("GET", "/api/dw/analytics/sales-monthly-summary")
	if !ok || rule.Pattern != "/api/dw/analytics/*" {
		t.Fatalf("analytics tidak cocok ke pola wildcard-nya: %+v ok=%v", rule, ok)
	}
}

func TestLookupDistinguishesMethodAndDepth(t *testing.T) {
	if _, ok := Lookup("DELETE", "/api/finance/invoices"); ok {
		t.Fatal("DELETE /api/finance/invoices tidak terdaftar tapi cocok dengan sesuatu")
	}
	if _, ok := Lookup("GET", "/api/finance/invoices/abc/post"); ok {
		t.Fatal("GET terhadap path milik POST seharusnya tidak cocok")
	}
	if _, ok := Lookup("POST", "/api/finance/invoices/abc/post"); !ok {
		t.Fatal("POST /api/finance/invoices/{id}/post seharusnya cocok")
	}
}

// Aksi yang menghasilkan angka di buku besar atau stok harus butuh `approve`,
// bukan `update` -- aturan 2 di policy.go. Yang dijaga di sini adalah
// keputusannya, supaya perubahan di tabel yang menurunkannya jadi `update`
// tidak lewat tanpa disadari.
func TestLedgerAndStockActionsRequireApprove(t *testing.T) {
	cases := []struct{ method, path string }{
		{"POST", "/api/finance/invoices/x/post"},
		{"POST", "/api/finance/journal-entries/x/post"},
		{"POST", "/api/hr/payroll-runs/x/post"},
		{"POST", "/api/purchasing/purchase-orders/x/receive"},
		{"POST", "/api/purchasing/purchase-orders/x/invoice"},
		{"POST", "/api/sales/sales-orders/x/fulfill"},
		{"POST", "/api/sales/sales-orders/x/invoice"},
		{"POST", "/api/warehouse/stock-opnames/x/post"},
		{"POST", "/api/warehouse/stock-transfers/x/confirm"},
		{"POST", "/api/production/work-orders/x/complete"},
		{"POST", "/api/ecommerce/orders/x/ship"},
		{"POST", "/api/project/projects/x/post-cost"},
	}
	for _, c := range cases {
		rule, ok := Lookup(c.method, c.path)
		if !ok {
			t.Errorf("%s %s tidak terdaftar", c.method, c.path)
			continue
		}
		for _, n := range rule.Req.needs {
			if n.Action != Approve {
				t.Errorf("%s %s butuh %s, seharusnya approve", c.method, c.path, n.Action)
			}
		}
	}
}

// Perpindahan status yang TIDAK menghasilkan angka cukup `update`. Sisi lain
// dari test di atas: tanpa ini, "amankan saja semuanya jadi approve" lolos
// begitu saja dan hak approve kehilangan artinya.
func TestPlainStatusChangesOnlyRequireUpdate(t *testing.T) {
	cases := []struct{ method, path string }{
		{"POST", "/api/sales/quotations/x/send"},
		{"POST", "/api/sales/sales-orders/x/confirm"},
		{"POST", "/api/purchasing/requisitions/x/submit"},
		{"POST", "/api/production/work-orders/x/start"},
		{"POST", "/api/ticketing/tickets/x/close"},
		{"POST", "/api/fleet/delivery-orders/x/dispatch"},
		{"POST", "/api/ecommerce/orders/x/deliver"},
		{"POST", "/api/project/projects/x/complete"},
	}
	for _, c := range cases {
		rule, ok := Lookup(c.method, c.path)
		if !ok {
			t.Errorf("%s %s tidak terdaftar", c.method, c.path)
			continue
		}
		for _, n := range rule.Req.needs {
			if n.Action != Update {
				t.Errorf("%s %s butuh %s, seharusnya update", c.method, c.path, n.Action)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Cakupan tabel terhadap route yang BENAR-BENAR terdaftar di seluruh service.
//
// Ini test yang paling penting di file ini. Tanpanya, endpoint baru yang lupa
// didaftarkan di policy.go baru ketahuan saat ada orang yang memakainya dan
// mendapat 403 tanpa sebab yang jelas. Route dibaca langsung dari source tiap
// service (bukan daftar yang disalin ke sini), jadi daftarnya tidak bisa basi.
// ---------------------------------------------------------------------------

var handleFuncRe = regexp.MustCompile(`HandleFunc\("([A-Z]+) ([^"]+)"`)

type registeredRoute struct {
	method string
	path   string // sudah berprefix /api/<modul>
}

func TestPolicyCoversEveryRegisteredRoute(t *testing.T) {
	routes := discoverRoutes(t)
	if len(routes) < 150 {
		// Penjaga bagi penjaganya sendiri: kalau pembacaan source gagal
		// (struktur folder berubah, pola HandleFunc diganti), test ini akan
		// "lulus" dengan nol route dan tidak menjaga apa pun.
		t.Fatalf("hanya menemukan %d route dari source service, jauh di bawah yang diharapkan -- pembacaannya kemungkinan rusak", len(routes))
	}

	var missing []string
	for _, r := range routes {
		if _, ok := Lookup(r.method, r.path); !ok {
			missing = append(missing, r.method+" "+r.path)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("%d endpoint terdaftar di service tapi tidak punya kebijakan hak akses di policy.go:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

func TestPolicyHasNoRuleForRoutesThatNoLongerExist(t *testing.T) {
	routes := discoverRoutes(t)

	var orphan []string
	for _, rule := range Rules {
		matched := false
		for _, r := range routes {
			if r.method != rule.Method {
				continue
			}
			if _, ok := matchScore(segments(rule.Pattern), segments(r.path)); ok {
				matched = true
				break
			}
		}
		if !matched {
			orphan = append(orphan, rule.Method+" "+rule.Pattern)
		}
	}
	if len(orphan) > 0 {
		sort.Strings(orphan)
		t.Fatalf("%d rule menjaga endpoint yang tidak ada lagi:\n  %s", len(orphan), strings.Join(orphan, "\n  "))
	}
}

// discoverRoutes membaca seluruh mux.HandleFunc("METHOD /path") di source tiap
// service, lalu menempelkan prefix gateway-nya. Nama folder service -> prefix
// mengikuti aturan yang sama dengan routes[] di gateway.go: buang akhiran
// "-service" (ai-bi-service -> /api/ai-bi, dw-service -> /api/dw).
func discoverRoutes(t *testing.T) []registeredRoute {
	t.Helper()

	backend := backendDir(t)
	var routes []registeredRoute

	for _, group := range []string{"modules", "services"} {
		entries, err := os.ReadDir(filepath.Join(backend, group))
		if err != nil {
			t.Fatalf("baca %s/%s: %v", backend, group, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == "api-gateway" {
				continue
			}
			prefix := "/api/" + strings.TrimSuffix(entry.Name(), "-service")
			routes = append(routes, routesInDir(t, filepath.Join(backend, group, entry.Name()), prefix)...)
		}
	}
	return routes
}

func routesInDir(t *testing.T, dir, prefix string) []registeredRoute {
	t.Helper()
	var routes []registeredRoute
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range handleFuncRe.FindAllStringSubmatch(string(src), -1) {
			method, route := m[1], m[2]
			if route == "/health" || route == "/metrics" {
				continue // tidak pernah diteruskan gateway
			}
			if publicRoute[method+" "+prefix+route] {
				continue
			}
			// {id} di route Go disamakan dengan segmen apa pun; Lookup sendiri
			// yang mencocokkannya ke wildcard di pola kebijakan.
			routes = append(routes, registeredRoute{method: method, path: prefix + replaceParams(route)})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("telusuri %s: %v", dir, err)
	}
	return routes
}

// publicRoute adalah endpoint yang sudah diloloskan gateway SEBELUM lapisan
// hak akses dipanggil (publicRoutes di internal/gateway/gateway.go) -- login
// dan refresh harus bisa dipanggil justru saat pemanggilnya belum punya
// identitas apa pun. Mendaftarkannya di policy.go akan menyiratkan sesuatu
// yang tidak pernah dievaluasi.
var publicRoute = map[string]bool{
	"POST /api/auth/login":   true,
	"POST /api/auth/refresh": true,
}

var paramRe = regexp.MustCompile(`\{[^}]+\}`)

func replaceParams(route string) string { return paramRe.ReplaceAllString(route, "x") }

// backendDir naik dari folder paket ini sampai ketemu folder yang punya
// go.work -- itulah backend/. Dicari, bukan ditulis "../../../..", supaya test
// tidak diam-diam rusak kalau paket ini dipindah satu tingkat.
func backendDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for range 10 {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("tidak menemukan backend/go.work dari working directory test")
	return ""
}
