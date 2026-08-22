package gateway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/enterprise-digital-platform/api-gateway/internal/config"
)

// fakeRBAC berdiri sebagai rbac-service untuk endpoint /access yang dipanggil
// gateway saat menegakkan hak akses. Jawabannya ditentukan per (user, company)
// supaya test bisa menyusun kasus "punya hak di company A, tidak di B".
type fakeRBAC struct {
	*httptest.Server
	calls   atomic.Int64
	answers map[string]map[string]any // key: user|company
}

func newFakeRBAC(t *testing.T, answers map[string]map[string]any) *fakeRBAC {
	t.Helper()
	f := &fakeRBAC{answers: answers}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/access" {
			// Gateway juga mem-proxy /api/rbac/* ke sini, jadi server ini
			// sekaligus berperan sebagai rbac-service biasa -- kalau tidak,
			// request yang LOLOS pemeriksaan hak akan tetap berakhir 404 dan
			// test tidak bisa membedakan "ditolak" dari "diteruskan".
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"from":"rbac"}`))
			return
		}
		f.calls.Add(1)
		key := r.URL.Query().Get("user_id") + "|" + r.URL.Query().Get("company_id")
		answer, ok := f.answers[key]
		if !ok {
			answer = map[string]any{"member": false, "permissions": map[string]any{}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(f.Close)
	return f
}

// grants menyusun jawaban /access untuk satu user di satu company.
func grants(perms map[string]map[string]bool) map[string]any {
	permissions := map[string]any{}
	for menu, actions := range perms {
		entry := map[string]any{}
		for action, allowed := range actions {
			entry["can_"+action] = allowed
		}
		permissions[menu] = entry
	}
	return map[string]any{"member": true, "permissions": permissions}
}

func enforcing(rbacURL string) func(*config.Config) {
	return func(cfg *config.Config) {
		cfg.AuthzEnforce = true
		cfg.RBACServiceURL = rbacURL
		cfg.AuthzCacheTTL = 0 // tiap test menyusun jawabannya sendiri; cache diuji terpisah
	}
}

const (
	testUser     = "11111111-1111-1111-1111-111111111111"
	testCompany  = "22222222-2222-2222-2222-222222222222"
	otherCompany = "33333333-3333-3333-3333-333333333333"
)

func authzRequest(t *testing.T, srv *httptest.Server, method, path, token, body string) *http.Response {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func userToken(t *testing.T) string {
	t.Helper()
	return issueToken(t, tokenOptions{subject: testUser, email: "user@contoh.test"})
}

func TestViewRightIsRequiredToReadAList(t *testing.T) {
	backend := newBackend(t)
	rbac := newFakeRBAC(t, map[string]map[string]any{
		testUser + "|" + testCompany: grants(map[string]map[string]bool{
			"/finance/invoices": {"view": true},
		}),
	})
	srv := newGateway(t, backend.URL, enforcing(rbac.URL))
	token := userToken(t)

	resp := authzRequest(t, srv, http.MethodGet, "/api/finance/invoices?company_id="+testCompany, token, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("hak view ada, tapi ditolak dengan %d", resp.StatusCode)
	}

	// Menu lain di service yang sama tidak ikut terbuka.
	resp = authzRequest(t, srv, http.MethodGet, "/api/finance/journal-entries?company_id="+testCompany, token, "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("menu tanpa hak seharusnya 403, dapat %d", resp.StatusCode)
	}
}

// Membuat dan MEMBUKUKAN adalah dua hak berbeda: inilah yang membuat pemisahan
// tugas (yang membuat invoice bukan yang memposting) benar-benar berlaku, bukan
// sekadar tombol yang disembunyikan.
func TestPostingNeedsApproveNotCreate(t *testing.T) {
	backend := newBackend(t)
	rbac := newFakeRBAC(t, map[string]map[string]any{
		testUser + "|" + testCompany: grants(map[string]map[string]bool{
			"/finance/invoices": {"view": true, "create": true},
		}),
	})
	srv := newGateway(t, backend.URL, enforcing(rbac.URL))
	token := userToken(t)

	body := `{"company_id":"` + testCompany + `","customer_name":"PT Contoh"}`
	if resp := authzRequest(t, srv, http.MethodPost, "/api/finance/invoices", token, body); resp.StatusCode != http.StatusOK {
		t.Fatalf("hak create ada, membuat invoice seharusnya lolos, dapat %d", resp.StatusCode)
	}

	resp := authzRequest(t, srv, http.MethodPost, "/api/finance/invoices/abc/post?company_id="+testCompany, token, "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("posting tanpa hak approve seharusnya 403, dapat %d", resp.StatusCode)
	}
}

func TestSuperAdminSkipsPermissionLookupEntirely(t *testing.T) {
	backend := newBackend(t)
	rbac := newFakeRBAC(t, nil)
	srv := newGateway(t, backend.URL, enforcing(rbac.URL))
	token := issueToken(t, tokenOptions{subject: "super", email: "root@contoh.test", isSuperAdmin: true})

	resp := authzRequest(t, srv, http.MethodPost, "/api/finance/invoices/abc/post?company_id="+testCompany, token, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("super admin seharusnya lolos, dapat %d", resp.StatusCode)
	}
	if rbac.calls.Load() != 0 {
		t.Fatalf("super admin seharusnya tidak perlu menanyakan hak apa pun, rbac dipanggil %d kali", rbac.calls.Load())
	}
}

// Endpoint internal ditutup untuk SEMUA pemanggil lewat gateway, super admin
// termasuk: yang dibatasi di sini adalah jalurnya, bukan tinggi haknya.
func TestInternalEndpointsAreClosedEvenForSuperAdmin(t *testing.T) {
	backend := newBackend(t)
	rbac := newFakeRBAC(t, nil)
	srv := newGateway(t, backend.URL, enforcing(rbac.URL))
	token := issueToken(t, tokenOptions{subject: "super", email: "root@contoh.test", isSuperAdmin: true})

	for _, path := range []string{
		"/api/warehouse/stock-movements/batch",
		"/api/rbac/access?user_id=x&company_id=y",
	} {
		method := http.MethodPost
		if strings.Contains(path, "/rbac/access") {
			method = http.MethodGet
		}
		resp := authzRequest(t, srv, method, path, token, "")
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s %s seharusnya 403, dapat %d", method, path, resp.StatusCode)
		}
	}
}

func TestUnknownEndpointsAreDeniedRatherThanProxied(t *testing.T) {
	backend := newBackend(t)
	rbac := newFakeRBAC(t, nil)
	srv := newGateway(t, backend.URL, enforcing(rbac.URL))
	token := issueToken(t, tokenOptions{subject: "super", email: "root@contoh.test", isSuperAdmin: true})

	resp := authzRequest(t, srv, http.MethodGet, "/api/finance/endpoint-yang-belum-didaftarkan", token, "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("endpoint tanpa kebijakan seharusnya 403, dapat %d", resp.StatusCode)
	}
}

// Company yang DISEBUT REQUEST menang atas header. Tanpa ini, user yang punya
// hak penuh di company A bisa memasang header company A lalu menyentuh data
// company B lewat query -- persis lubang yang ingin ditutup penegakan ini.
func TestCompanyInTheRequestBeatsTheHeader(t *testing.T) {
	backend := newBackend(t)
	rbac := newFakeRBAC(t, map[string]map[string]any{
		testUser + "|" + testCompany: grants(map[string]map[string]bool{
			"/finance/invoices": {"view": true},
		}),
		// Di company lain user memang anggota, tapi hak invoice-nya dicabut.
		testUser + "|" + otherCompany: grants(map[string]map[string]bool{}),
	})
	srv := newGateway(t, backend.URL, enforcing(rbac.URL))
	token := userToken(t)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/finance/invoices?company_id="+otherCompany, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Company-Id", testCompany) // company "yang sedang dipilih di layar"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("hak company di header seharusnya tidak dipakai untuk data company lain, dapat %d", resp.StatusCode)
	}
}

// Endpoint create tidak menyebut company di query; company-nya ada di body.
// Yang diuji sekalian: body harus sampai UTUH ke service tujuan walau sudah
// dibaca gateway untuk mencari company_id.
func TestCompanyIsReadFromTheBodyWithoutConsumingIt(t *testing.T) {
	backend := newBackend(t)
	rbac := newFakeRBAC(t, map[string]map[string]any{
		testUser + "|" + testCompany: grants(map[string]map[string]bool{
			"/warehouse/products": {"view": true, "create": true},
		}),
	})
	srv := newGateway(t, backend.URL, enforcing(rbac.URL))
	token := userToken(t)

	body := `{"company_id":"` + testCompany + `","code":"PRD-1","name":"Barang Uji"}`
	resp := authzRequest(t, srv, http.MethodPost, "/api/warehouse/products", token, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("company_id di body seharusnya cukup, dapat %d", resp.StatusCode)
	}

	_, _, _, _, gotBody := backend.last(t)
	if gotBody != body {
		t.Fatalf("body sampai ke service dalam keadaan berubah:\n  kirim: %s\n  terima: %s", body, gotBody)
	}
}

func TestNonMemberOfACompanyIsRejected(t *testing.T) {
	backend := newBackend(t)
	rbac := newFakeRBAC(t, nil) // semua jawaban: member=false
	srv := newGateway(t, backend.URL, enforcing(rbac.URL))

	resp := authzRequest(t, srv, http.MethodGet, "/api/finance/invoices?company_id="+testCompany, userToken(t), "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bukan anggota company seharusnya 403, dapat %d", resp.StatusCode)
	}
}

// Hal yang memang milik setiap user tidak boleh ikut terkunci: tanpa ini,
// user yang haknya dicabut habis tidak bisa lagi mengganti passwordnya sendiri
// maupun memuat sidebar.
func TestSelfServiceEndpointsOnlyNeedAValidToken(t *testing.T) {
	backend := newBackend(t)
	rbac := newFakeRBAC(t, nil)
	srv := newGateway(t, backend.URL, enforcing(rbac.URL))
	token := userToken(t)

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodPost, "/api/auth/change-password"},
		{http.MethodGet, "/api/rbac/menu-tree"},
		{http.MethodGet, "/api/company/companies"},
		{http.MethodGet, "/api/rbac/user-permissions?user_id=" + testUser + "&company_id=" + testCompany},
	} {
		resp := authzRequest(t, srv, tc.method, tc.path, token, "")
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s %s seharusnya lolos dengan token valid saja, dapat %d", tc.method, tc.path, resp.StatusCode)
		}
	}

	// Tapi menanyakan hak ORANG LAIN tetap butuh hak User Management.
	resp := authzRequest(t, srv, http.MethodGet,
		"/api/rbac/user-permissions?user_id=orang-lain&company_id="+testCompany, token, "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("membaca hak user lain seharusnya 403, dapat %d", resp.StatusCode)
	}
}

// rbac-service mati BUKAN alasan meneruskan request tanpa pemeriksaan. 503
// dipilih supaya gangguannya terbaca sebagai gangguan, bukan sebagai hak yang
// dicabut (403) yang akan membuat orang mencari masalahnya di tempat salah.
func TestUnreachableRBACBecomesServiceUnavailableNotAFreePass(t *testing.T) {
	backend := newBackend(t)
	rbac := newFakeRBAC(t, nil)
	rbac.Close() // sengaja dimatikan sebelum dipakai
	srv := newGateway(t, backend.URL, enforcing(rbac.URL))

	resp := authzRequest(t, srv, http.MethodGet, "/api/finance/invoices?company_id="+testCompany, userToken(t), "")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("rbac tidak bisa dihubungi seharusnya 503, dapat %d", resp.StatusCode)
	}
}

func TestRequestWithoutAnyCompanyIsRejectedWithABadRequest(t *testing.T) {
	backend := newBackend(t)
	rbac := newFakeRBAC(t, nil)
	srv := newGateway(t, backend.URL, enforcing(rbac.URL))

	resp := authzRequest(t, srv, http.MethodGet, "/api/finance/invoices", userToken(t), "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("request tanpa company seharusnya 400, dapat %d", resp.StatusCode)
	}
}
