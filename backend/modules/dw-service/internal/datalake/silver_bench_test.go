package datalake

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSilverScale_IncrementalVsFull mengukur waktu dan puncak memori heap
// build Silver penuh vs inkremental pada N baris. Hanya jalan kalau BENCH_ROWS
// diisi (mis. BENCH_ROWS=250000), karena menulis ratusan MB ke MinIO:
//
//	BENCH_ROWS=250000 go test ./internal/datalake -run SilverScale -v -timeout 30m
//
// Umur objek Bronze dipalsukan lewat nama key (build membaca umur dari nama),
// jadi tidak perlu menunggu 10 menit pengendapan. Yang diukur hanya sisi
// lake: ekstraksi sumber (Postgres) tidak ikut, kecuali LiveKeys sintetis yang
// membangun peta kunci seukuran N di dalam build, meniru biaya "lantai" itu.
func TestSilverScale_IncrementalVsFull(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("BENCH_ROWS"))
	if n == 0 {
		t.Skip("set BENCH_ROWS untuk menjalankan uji skala")
	}
	ctx := context.Background()
	fact := uniqueFact(t)
	t.Cleanup(func() { cleanup(t, fact) })

	const perObject = 10000
	company := uid(1)
	pad := strings.Repeat("x", 120) // baris ~250 B, mendekati sales_order_lines
	rowOf := func(i, version int) string {
		return `{"LineID":"` + uid(i) + `","CompanyID":"` + company + `","OrderID":"` + uid(i/5) +
			`","ProductID":"` + uid(i%997) + `","Quantity":"3","UnitPrice":"15000.00","Note":"` + pad + `","V":"` + strconv.Itoa(version) + `"}`
	}
	put := func(at time.Time, lines []string) {
		key := fmt.Sprintf("%s/%04d/%02d/%02d/%d.jsonl", fact, at.Year(), at.Month(), at.Day(), at.UnixNano())
		if err := testClient.Put(ctx, key, []byte(strings.Join(lines, "\n")+"\n")); err != nil {
			t.Fatal(err)
		}
	}

	t0 := time.Now()
	base := time.Now().Add(-4 * time.Hour)
	for start, o := 0, 0; start < n; start, o = start+perObject, o+1 {
		end := min(start+perObject, n)
		lines := make([]string, 0, end-start)
		for i := start; i < end; i++ {
			lines = append(lines, rowOf(i, 0))
		}
		put(base.Add(time.Duration(o)*time.Second), lines)
	}
	t.Logf("seed: %d baris, %s", n, time.Since(t0).Round(time.Millisecond))

	live := func(context.Context) (*KeySet, error) {
		m := NewKeySet(n)
		for i := 0; i < n; i++ {
			m.Add(company + "/" + uid(i))
		}
		return m, nil
	}

	measure := func(label string, liveFn LiveKeys, full bool) SilverStats {
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		baseline := ms.HeapAlloc
		var mu sync.Mutex
		var peak uint64
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				var s runtime.MemStats
				runtime.ReadMemStats(&s)
				mu.Lock()
				peak = max(peak, s.HeapAlloc)
				mu.Unlock()
				select {
				case <-stop:
					return
				case <-time.After(20 * time.Millisecond):
				}
			}
		}()
		start := time.Now()
		st, err := testClient.buildSilver(ctx, fact, "LineID", liveFn, full)
		el := time.Since(start)
		close(stop)
		<-done
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		mu.Lock()
		defer mu.Unlock()
		t.Logf("%-28s mode=%-11s %8s  peak heap +%4d MB  bronze=%d prev=%d silver=%d dup=%d",
			label, st.Mode, el.Round(time.Millisecond), (peak-baseline)>>20,
			st.BronzeRows, st.PreviousSilverRows, st.SilverRows, st.DuplicatesDropped)
		return st
	}
	silverHash := func() [32]byte {
		b, err := testClient.Get(ctx, silverKey(fact))
		if err != nil {
			t.Fatal(err)
		}
		return sha256.Sum256(b)
	}

	for _, withLive := range []bool{false, true} {
		var lf LiveKeys
		tag := "live=nil"
		if withLive {
			lf, tag = live, "live=map"
		}
		measure("full (first) "+tag, lf, true)
		measure("incremental, no new "+tag, lf, false)

		// Delta 1%: separuh pembaruan baris lama, separuh baris baru.
		delta := max(n/100, 2)
		lines := make([]string, 0, delta)
		for i := 0; i < delta/2; i++ {
			lines = append(lines, rowOf(i*(n/delta), 1))
		}
		for i := 0; i < delta-delta/2; i++ {
			lines = append(lines, rowOf(n+i, 0))
		}
		put(base.Add(2*time.Hour+time.Duration(withLiveInt(withLive))*time.Minute), lines)
		// peta kunci hidup harus ikut mencakup baris baru
		n += delta - delta/2
		measure("incremental, delta 1% "+tag, lf, false)
		inc := silverHash()
		measure("full (forced) "+tag, lf, true)
		if full := silverHash(); !bytes.Equal(inc[:], full[:]) {
			t.Errorf("%s: Silver inkremental != Silver penuh", tag)
		}
	}
}

func withLiveInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
