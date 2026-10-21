package etl

import (
	"context"
	"strings"
	"testing"

	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
)

// File ini sengaja berawalan "zz_": Go menjalankan test menurut urutan nama
// berkas, dan perbandingan di bawah baru bermakna kalau test fact lain sudah
// menanam barisnya di database uji bersama.

func sorted(set *datalake.KeySet) map[string]struct{} {
	out := make(map[string]struct{}, set.Len())
	set.Each(func(k string) { out[k] = struct{}{} })
	return out
}

// TestKeysOnly_MatchesStreamedExtract adalah jaring pengaman query kunci-saja.
// Untuk SETIAP fact, himpunan kunci dari query dua-kolom harus sama persis
// dengan himpunan dari ekstrak penuh (cara lama). Kalau filternya menyimpang,
// atau indeks company_id salah, Silver akan memangkas baris yang masih ada
// tanpa galat apa pun -- test ini adalah satu-satunya yang menangkapnya.
func TestKeysOnly_MatchesStreamedExtract(t *testing.T) {
	ctx := context.Background()
	for _, c := range liveKeysCases {
		t.Run(c.name, func(t *testing.T) {
			want, err := c.streamed(ctx, sourcePool)
			if err != nil {
				t.Fatalf("streamed: %v", err)
			}
			got, err := c.keysOnly(ctx, sourcePool)
			if err != nil {
				t.Fatalf("keys-only: %v", err)
			}
			// Bukan perbandingan kosong-dengan-kosong: tanpa baris, query yang
			// salah pun lolos. Kalau gagal di sini, fact ini belum ditanami
			// data oleh test lain.
			if want.Len() == 0 {
				t.Fatalf("tidak ada baris sumber untuk %s di database uji; perbandingan tidak bermakna", c.name)
			}
			if got.Len() != want.Len() {
				t.Errorf("jumlah kunci: keys-only %d, ekstrak penuh %d", got.Len(), want.Len())
			}
			wantSet, gotSet := sorted(want), sorted(got)
			for k := range wantSet {
				if _, ok := gotSet[k]; !ok {
					t.Errorf("kunci %q ada di ekstrak penuh tapi hilang di keys-only (baris akan salah dipangkas)", k)
					break
				}
			}
			for k := range gotSet {
				if _, ok := wantSet[k]; !ok {
					t.Errorf("kunci %q ada di keys-only tapi bukan baris yang disalin sync (baris hantu tak terpangkas)", k)
					break
				}
			}
		})
	}
}

func TestKeysOnlySQL_KeepsFromAndWhereVerbatim(t *testing.T) {
	cases := []struct {
		name       string
		sql        string
		companyCol int
		want       []string // potongan yang HARUS ada
		forbid     []string // potongan yang TIDAK boleh ada
	}{
		{
			name: "iot: join + created_at", sql: iotExtractSQL, companyCol: 1,
			want:   []string{"SELECT r.id, r.company_id", "FROM readings r", "JOIN devices d ON d.id = r.device_id", "WHERE r.created_at >= $1"},
			forbid: []string{"ORDER BY", "device_code", "value_text"},
		},
		{
			name: "finance: company di kolom ke-3, watermark COALESCE", sql: financeExtractSQL, companyCol: 2,
			want:   []string{"SELECT jl.id, je.company_id", "WHERE COALESCE(je.posted_at, je.created_at) >= $1"},
			forbid: []string{"ORDER BY", "entry_number", "AS watermark"},
		},
		{
			name: "oee: LATERAL dan filter CLOSED tetap", sql: productionOEEExtractSQL, companyCol: 1,
			want:   []string{"SELECT pr.id, pr.company_id", "LEFT JOIN LATERAL", "FROM downtime_logs WHERE production_run_id = pr.id", "WHERE pr.status = 'CLOSED' AND pr.updated_at >= $1"},
			forbid: []string{"ORDER BY pr.updated_at", "ideal_cycle_time"},
		},
		{
			name: "alias kolom dibuang", sql: "SELECT t.id AS x, t.company_id AS y, t.name FROM t WHERE t.u >= $1 ORDER BY t.u", companyCol: 1,
			want:   []string{"SELECT t.id, t.company_id", "FROM t WHERE t.u >= $1"},
			forbid: []string{" AS ", "ORDER BY", "t.name"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := keysOnlySQL(c.sql, c.companyCol)
			if err != nil {
				t.Fatal(err)
			}
			flat := strings.Join(strings.Fields(got), " ")
			for _, w := range c.want {
				if !strings.Contains(flat, strings.Join(strings.Fields(w), " ")) {
					t.Errorf("hasil tidak memuat %q:\n%s", w, got)
				}
			}
			for _, f := range c.forbid {
				if strings.Contains(flat, f) {
					t.Errorf("hasil masih memuat %q:\n%s", f, got)
				}
			}
		})
	}

	if _, err := keysOnlySQL(iotExtractSQL, 99); err == nil {
		t.Error("companyCol di luar daftar kolom harus galat")
	}
	if _, err := keysOnlySQL(iotExtractSQL, 0); err == nil {
		t.Error("companyCol 0 (kolom id) harus galat")
	}
}

// Fact baru yang lupa didaftarkan di liveKeysCases lolos dari perbandingan di
// atas -- dan justru fact baru yang paling mungkin salah indeks company_id-nya.
func TestKeysOnly_CoversEveryFact(t *testing.T) {
	covered := map[string]bool{}
	for _, c := range liveKeysCases {
		covered[c.name] = true
	}
	for _, f := range Facts {
		if !covered[f.Name] {
			t.Errorf("fact %q belum ada di liveKeysCases (zz_livekeys_equiv_cases_test.go)", f.Name)
		}
	}
	if len(liveKeysCases) != len(Facts) {
		t.Errorf("liveKeysCases punya %d entri, Facts punya %d", len(liveKeysCases), len(Facts))
	}
}
