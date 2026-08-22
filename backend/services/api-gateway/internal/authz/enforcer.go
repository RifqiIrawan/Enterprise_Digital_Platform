package authz

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// Identity adalah siapa yang meminta, hasil pembacaan JWT di gateway.
type Identity struct {
	UserID       string
	IsSuperAdmin bool
}

// Result kosong (Status 0) berarti boleh diteruskan.
type Result struct {
	Status  int
	Message string
}

func (r Result) Allowed() bool { return r.Status == 0 }

type Enforcer struct {
	client *Client
}

func NewEnforcer(c *Client) *Enforcer { return &Enforcer{client: c} }

// maxBodyPeek membatasi berapa banyak body yang dibaca hanya untuk mencari
// company_id. Payload terbesar di platform ini (Work Order dengan puluhan
// komponen, PO dengan puluhan baris) masih jauh di bawah ini; body yang lebih
// besar dari ini tetap diteruskan utuh, company-nya saja yang jatuh ke header.
const maxBodyPeek = 1 << 20

// Authorize memutuskan apakah request boleh diteruskan. Request dimodifikasi
// hanya dalam satu hal: body-nya bisa dibaca lalu dikembalikan seperti semula
// (lihat companyID), sehingga proxy di belakangnya tetap menerima isi yang sama.
func (e *Enforcer) Authorize(r *http.Request, id Identity) Result {
	rule, found := Lookup(r.Method, r.URL.Path)
	if !found {
		// Aturan 3 di policy.go: endpoint yang tidak dikenali ditolak, bukan
		// diteruskan. Pesannya sengaja menyebut sebabnya supaya endpoint baru
		// yang lupa didaftarkan langsung ketahuan sebabnya, bukan terlihat
		// seperti hak akses yang kurang.
		return Result{http.StatusForbidden, "Endpoint ini belum terdaftar di kebijakan hak akses gateway"}
	}

	// Diperiksa SEBELUM super admin: endpoint internal bukan soal seberapa
	// tinggi hak pemanggil, tapi soal jalur mana yang boleh memakainya.
	if rule.Req.kind == kindInternal {
		return Result{http.StatusForbidden, "Endpoint internal, tidak dapat dipanggil lewat gateway"}
	}

	if id.IsSuperAdmin {
		return Result{}
	}

	if rule.Req.kind == kindAuthenticated {
		return Result{}
	}

	if rule.Req.selfParam != "" && r.URL.Query().Get(rule.Req.selfParam) == id.UserID && id.UserID != "" {
		return Result{}
	}

	company := companyID(r)
	if company == "" {
		return Result{http.StatusBadRequest, "Request tidak menyebut company_id, hak akses tidak dapat ditentukan"}
	}

	access, err := e.client.Access(r.Context(), id.UserID, company)
	if err != nil {
		// 503, bukan 403: yang gagal adalah pemeriksaannya, bukan haknya.
		// Menjawab 403 di sini membuat gangguan rbac-service terlihat seperti
		// hak yang dicabut, dan orang akan mencari masalahnya di tempat salah.
		return Result{http.StatusServiceUnavailable, "Hak akses tidak dapat diperiksa saat ini"}
	}
	if !access.Member {
		return Result{http.StatusForbidden, "Anda tidak punya akses di company ini"}
	}

	for _, n := range rule.Req.needs {
		if access.Permissions[n.Menu].Allows(n.Action) {
			return Result{}
		}
	}
	return Result{http.StatusForbidden, "Hak akses Anda tidak mencakup tindakan ini"}
}

// companyID mencari company yang BENAR-BENAR DITUJU request, bukan sekadar
// company yang sedang dipilih di layar. Urutannya penting:
//
//  1. query `company_id` -- dipakai hampir seluruh endpoint list & filter;
//  2. field `company_id` di body JSON -- dipakai seluruh endpoint create;
//  3. header `X-Company-Id` -- terakhir, karena inilah satu-satunya yang tidak
//     ikut menentukan data mana yang disentuh. Endpoint seperti DELETE
//     /api/hr/holidays/{id} memang tidak menyebut company di mana pun, jadi
//     header tetap dibutuhkan; tapi selama request menyebutnya sendiri, itulah
//     yang dipakai -- kalau tidak, user yang punya akses penuh di company A
//     bisa memakai hak company A untuk menyentuh data company B.
func companyID(r *http.Request) string {
	if v := r.URL.Query().Get("company_id"); v != "" {
		return v
	}
	if v := companyIDFromBody(r); v != "" {
		return v
	}
	return r.Header.Get("X-Company-Id")
}

func companyIDFromBody(r *http.Request) string {
	if r.Body == nil || r.Body == http.NoBody {
		return ""
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return ""
	}
	peek, err := io.ReadAll(io.LimitReader(r.Body, maxBodyPeek))
	if err != nil {
		return ""
	}
	// Body dikembalikan utuh: bagian yang sudah terbaca disambung kembali di
	// depan sisanya, jadi payload yang lebih besar dari maxBodyPeek pun tetap
	// sampai ke service tujuan tanpa terpotong.
	r.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(peek), r.Body), r.Body}

	var payload struct {
		CompanyID string `json:"company_id"`
	}
	if err := json.Unmarshal(peek, &payload); err != nil {
		return ""
	}
	return payload.CompanyID
}
