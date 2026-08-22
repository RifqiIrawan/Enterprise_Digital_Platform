package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Client mengambil hak efektif user dari rbac-service (GET /access) dan
// menyimpannya sebentar. Tanpa cache, SATU halaman yang memuat 4-5 daftar
// sekaligus akan menghasilkan 4-5 query RBAC tambahan -- rbac-service berubah
// dari layanan administrasi yang jarang dipanggil menjadi jalur terpanas di
// seluruh platform.
//
// Dua batas waktu, bukan satu:
//   - ttl: selama ini jawaban dipakai apa adanya. Konsekuensinya, pencabutan
//     hak baru berlaku paling lama setelah ttl -- itulah harga yang dibayar,
//     dan itulah kenapa ttl-nya pendek (30 detik), bukan hitungan menit.
//   - staleGrace: kalau rbac-service TIDAK BISA dihubungi, jawaban lama masih
//     dipakai sampai sebatas ini. Alternatifnya adalah seluruh platform ikut
//     mati begitu rbac-service mati, dan itu jauh lebih buruk daripada hak yang
//     tertinggal beberapa menit. Setelah lewat, request ditolak (503) -- bukan
//     diteruskan tanpa pemeriksaan.
type Client struct {
	baseURL    string
	http       *http.Client
	ttl        time.Duration
	staleGrace time.Duration
	now        func() time.Time

	mu      sync.Mutex
	entries map[string]*entry
}

// entry.mu juga berfungsi sebagai penggabung request: saat satu request sedang
// mengambil hak seorang user, request lain untuk user yang sama menunggu di
// sini lalu ikut memakai hasilnya, bukan menembak rbac-service lagi.
type entry struct {
	mu        sync.Mutex
	access    Access
	fetchedAt time.Time
	filled    bool
}

// Actions mencerminkan kolom hak di rbac-service.
type Actions struct {
	CanView    bool `json:"can_view"`
	CanCreate  bool `json:"can_create"`
	CanUpdate  bool `json:"can_update"`
	CanDelete  bool `json:"can_delete"`
	CanApprove bool `json:"can_approve"`
	CanExport  bool `json:"can_export"`
}

func (a Actions) Allows(action Action) bool {
	switch action {
	case View:
		return a.CanView
	case Create:
		return a.CanCreate
	case Update:
		return a.CanUpdate
	case Delete:
		return a.CanDelete
	case Approve:
		return a.CanApprove
	case Export:
		return a.CanExport
	}
	return false
}

// Access adalah jawaban GET /access rbac-service: apakah user anggota company
// ini, dan hak apa yang dimilikinya per PATH menu.
type Access struct {
	Member      bool               `json:"member"`
	Permissions map[string]Actions `json:"permissions"`
}

func NewClient(rbacBaseURL string, ttl, staleGrace time.Duration) *Client {
	return &Client{
		baseURL:    rbacBaseURL,
		http:       &http.Client{Timeout: 5 * time.Second},
		ttl:        ttl,
		staleGrace: staleGrace,
		now:        time.Now,
		entries:    map[string]*entry{},
	}
}

func (c *Client) Access(ctx context.Context, userID, companyID string) (Access, error) {
	key := userID + "|" + companyID

	c.mu.Lock()
	e, ok := c.entries[key]
	if !ok {
		e = &entry{}
		c.entries[key] = e
	}
	c.mu.Unlock()

	e.mu.Lock()
	defer e.mu.Unlock()

	age := c.now().Sub(e.fetchedAt)
	if e.filled && age < c.ttl {
		return e.access, nil
	}

	access, err := c.fetch(ctx, userID, companyID)
	if err != nil {
		if e.filled && age < c.ttl+c.staleGrace {
			log.Printf("api-gateway: rbac-service tidak bisa dihubungi (%v), memakai hak akses lama umur %s untuk user=%s company=%s", err, age.Round(time.Second), userID, companyID)
			return e.access, nil
		}
		return Access{}, err
	}

	e.access, e.fetchedAt, e.filled = access, c.now(), true
	return access, nil
}

func (c *Client) fetch(ctx context.Context, userID, companyID string) (Access, error) {
	endpoint := fmt.Sprintf("%s/access?user_id=%s&company_id=%s",
		c.baseURL, url.QueryEscape(userID), url.QueryEscape(companyID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Access{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Access{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Access{}, fmt.Errorf("rbac-service menjawab %d", resp.StatusCode)
	}
	var access Access
	if err := json.NewDecoder(resp.Body).Decode(&access); err != nil {
		return Access{}, err
	}
	if access.Permissions == nil {
		access.Permissions = map[string]Actions{}
	}
	return access, nil
}
