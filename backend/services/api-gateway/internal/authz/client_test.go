package authz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeAccessServer menjawab /access dan menghitung berapa kali benar-benar
// dipanggil -- itulah yang membedakan "cache bekerja" dari "kebetulan hasilnya
// sama".
func fakeAccessServer(t *testing.T, calls *atomic.Int64, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

const grantsInvoiceView = `{"member":true,"permissions":{"/finance/invoices":{"can_view":true}}}`

func TestAccessIsCachedForTheTTLThenRefetched(t *testing.T) {
	var calls atomic.Int64
	srv := fakeAccessServer(t, &calls, grantsInvoiceView)

	now := time.Now()
	c := NewClient(srv.URL, 30*time.Second, 5*time.Minute)
	c.now = func() time.Time { return now }

	for range 5 {
		if _, err := c.Access(context.Background(), "u1", "c1"); err != nil {
			t.Fatalf("Access: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("5 request dalam TTL seharusnya 1 panggilan ke rbac, dapat %d", calls.Load())
	}

	// User lain tidak boleh ikut memakai jawaban user pertama.
	if _, err := c.Access(context.Background(), "u2", "c1"); err != nil {
		t.Fatalf("Access u2: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("user berbeda seharusnya menambah 1 panggilan, total jadi %d", calls.Load())
	}

	now = now.Add(31 * time.Second)
	if _, err := c.Access(context.Background(), "u1", "c1"); err != nil {
		t.Fatalf("Access setelah TTL: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("setelah TTL lewat seharusnya diambil ulang, total panggilan %d", calls.Load())
	}
}

// Saat rbac-service mati, jawaban lama masih dipakai sampai batas staleGrace --
// alternatifnya seluruh platform ikut mati. Setelah batas itu lewat, request
// gagal (dan gateway menjawab 503), BUKAN diloloskan.
func TestStaleAnswersSurviveAnOutageButNotForever(t *testing.T) {
	var calls atomic.Int64
	srv := fakeAccessServer(t, &calls, grantsInvoiceView)

	now := time.Now()
	c := NewClient(srv.URL, 30*time.Second, 5*time.Minute)
	c.now = func() time.Time { return now }

	access, err := c.Access(context.Background(), "u1", "c1")
	if err != nil || !access.Member {
		t.Fatalf("pengambilan pertama gagal: %+v %v", access, err)
	}

	srv.Close() // rbac-service mati

	now = now.Add(2 * time.Minute) // lewat TTL, masih di dalam staleGrace
	access, err = c.Access(context.Background(), "u1", "c1")
	if err != nil {
		t.Fatalf("jawaban lama seharusnya masih dipakai selama gangguan: %v", err)
	}
	if !access.Permissions["/finance/invoices"].CanView {
		t.Fatal("jawaban lama yang dipakai kehilangan isinya")
	}

	now = now.Add(10 * time.Minute) // lewat staleGrace
	if _, err := c.Access(context.Background(), "u1", "c1"); err == nil {
		t.Fatal("setelah staleGrace lewat, kegagalan harus dilaporkan -- bukan diloloskan diam-diam")
	}
}

// Beberapa request bersamaan untuk user yang sama tidak boleh menembak
// rbac-service sebanyak jumlah request: satu halaman saja bisa memuat 4-5
// daftar sekaligus.
func TestConcurrentRequestsForOneUserCollapseIntoOneFetch(t *testing.T) {
	var calls atomic.Int64
	srv := fakeAccessServer(t, &calls, grantsInvoiceView)

	c := NewClient(srv.URL, 30*time.Second, 5*time.Minute)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Access(context.Background(), "u1", "c1"); err != nil {
				t.Errorf("Access: %v", err)
			}
		}()
	}
	wg.Wait()

	if calls.Load() != 1 {
		t.Fatalf("20 request bersamaan seharusnya menghasilkan 1 panggilan, dapat %d", calls.Load())
	}
}

func TestNonJSONOrErrorResponsesAreTreatedAsFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, 30*time.Second, 5*time.Minute)
	if _, err := c.Access(context.Background(), "u1", "c1"); err == nil {
		t.Fatal("rbac-service yang menjawab 500 seharusnya jadi kegagalan, bukan hak kosong yang terlihat sah")
	}
}
