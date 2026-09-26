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
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/rag-service/internal/httpapi"
	"github.com/enterprise-digital-platform/rag-service/internal/llm"
	"github.com/enterprise-digital-platform/rag-service/internal/store"
	"github.com/enterprise-digital-platform/rag-service/migrations"
)

var pool *pgxpool.Pool

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/rag_service_test?sslmode=disable"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	adminURL := getEnv("RAG_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: rag-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: rag-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE rag_service_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create rag_service_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("RAG_TEST_DATABASE_URL", testDatabaseURL)
	pool, err = store.Connect(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to rag_service_test: %v\n", err)
		os.Exit(0)
	}
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Printf("FAIL: migration of rag_service_test failed: %v\n", err)
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

// resetCorpus mengosongkan index sebelum tiap test yang menanam korpusnya
// sendiri. rag_chunks ikut terhapus lewat CASCADE.
func resetCorpus(t *testing.T) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `DELETE FROM rag_documents`); err != nil {
		t.Fatalf("reset corpus: %v", err)
	}
}

// stubAnswerer menggantikan Claude di test. Tidak ada test di berkas ini yang
// menyentuh jaringan: yang diuji adalah perilaku service di sekitar model
// (kutipan mana yang dikirim, apa yang terjadi saat model gagal, apa yang
// dicatat), bukan model itu sendiri.
type stubAnswerer struct {
	answer     string
	err        error
	refused    bool
	lastPrompt []llm.Passage
	calls      int
}

func (s *stubAnswerer) Answer(ctx context.Context, question string, passages []llm.Passage) (llm.Result, error) {
	s.calls++
	s.lastPrompt = passages
	if s.err != nil {
		return llm.Result{}, s.err
	}
	if s.refused {
		return llm.Result{Model: s.Model(), Refused: true, RefusalReason: "cyber"}, nil
	}
	return llm.Result{
		Text: s.answer, Model: s.Model(), InputTokens: 1200, OutputTokens: 240,
	}, nil
}

func (s *stubAnswerer) Model() string { return "stub-model" }

// writeCorpus menulis berkas markdown ke folder sementara dan mengembalikan
// path-nya, supaya test ingest bekerja terhadap berkas sungguhan (termasuk
// jalur baca/hashnya), bukan terhadap struktur Go yang sudah jadi.
func writeCorpus(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir korpus: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("tulis korpus %s: %v", name, err)
		}
	}
	return dir
}

func newServer(t *testing.T, corpusDir string, answerer llm.Answerer) *httptest.Server {
	t.Helper()
	handler := httpapi.NewHandler(pool, answerer, corpusDir, 5)
	mux := http.NewServeMux()
	handler.Register(mux)
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
	client := &http.Client{Timeout: 30 * time.Second}
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
	return doRequest(t, http.MethodPost, url, payload)
}

func getJSON(t *testing.T, url string) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodGet, url, nil)
}

func requireStatus(t *testing.T, resp apiResponse, want int) {
	t.Helper()
	if resp.status != want {
		t.Fatalf("expected status %d, got %d (body: %s)", want, resp.status, resp.body)
	}
}

// Korpus contoh yang dipakai sebagian besar test: dua dokumen berbahasa
// Indonesia dengan istilah yang benar-benar dipakai platform ini.
func sampleCorpus() map[string]string {
	return map[string]string{
		"01_produksi.md": `# Modul Produksi

## Rumus OEE

OEE dihitung sebagai Availability dikali Performance dikali Quality. Availability adalah waktu jalan dibagi waktu rencana, Performance membandingkan waktu ideal dengan waktu jalan, dan Quality adalah unit bagus dibagi total unit yang dihasilkan mesin pada shift itu.

## Work Order

Work order adalah rencana produksi yang mengambil komponen dari bill of material. Stok hanya bermutasi ketika work order berstatus COMPLETED, tidak pada saat production run dicatat oleh operator lantai produksi.
`,
		"02_keuangan.md": `# Modul Keuangan

## Jurnal Umum

Jurnal umum mencatat transaksi keuangan dengan pasangan debit dan kredit yang wajib seimbang. Jurnal berstatus DRAFT bisa diubah, sedangkan jurnal yang sudah diposting tidak bisa dihapus karena menjadi dasar laporan keuangan periode tersebut.

## Penyusutan Aset

Penyusutan dihitung setiap bulan lalu diposting ke buku besar sebagai satu jurnal, mendebit beban penyusutan dan mengkredit akumulasi penyusutan pada akhir periode.
`,
	}
}
