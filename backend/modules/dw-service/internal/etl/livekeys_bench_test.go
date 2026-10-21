package etl

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"

	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
)

// peakHeap menjalankan fn dan mengembalikan kenaikan puncak heap Go selama itu.
func peakHeap(fn func()) (uint64, time.Duration) {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	baseline := ms.HeapAlloc
	var mu sync.Mutex
	var peak uint64
	stop, done := make(chan struct{}), make(chan struct{})
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
			case <-time.After(10 * time.Millisecond):
			}
		}
	}()
	start := time.Now()
	fn()
	el := time.Since(start)
	close(stop)
	<-done
	mu.Lock()
	defer mu.Unlock()
	return peak - baseline, el
}

// TestLiveKeys_StreamingVsMaterialized membandingkan cara lama (ekstrak semua
// baris ke slice, lalu hitung kunci) dengan cara baru (alirkan baris, simpan
// hanya kuncinya) pada N baris iot readings di Postgres uji. Hanya jalan kalau
// BENCH_ROWS diisi:
//
//	BENCH_ROWS=300000 go test ./internal/etl -run LiveKeys_StreamingVsMaterialized -v
func TestLiveKeys_StreamingVsMaterialized(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("BENCH_ROWS"))
	if n == 0 {
		t.Skip("set BENCH_ROWS untuk menjalankan uji skala")
	}
	ctx := context.Background()
	company := uuid.New()
	device := uuid.New()
	if _, err := sourcePool.Exec(ctx, `INSERT INTO devices (id, device_code, device_type) VALUES ($1, $2, 'TEMPERATURE')`,
		device, "BENCH-"+device.String()[:8]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sourcePool.Exec(ctx, `DELETE FROM readings WHERE company_id = $1`, company)
		sourcePool.Exec(ctx, `DELETE FROM devices WHERE id = $1`, device)
	})
	if _, err := sourcePool.Exec(ctx, `
		INSERT INTO readings (device_id, company_id, reading_type, value_numeric, value_text, recorded_at)
		SELECT $1, $2, 'TEMPERATURE', 20 + (g % 10), 'sensor-bench-reading-text-padding', now() - (g || ' seconds')::interval
		FROM generate_series(1, $3) g`, device, company, n); err != nil {
		t.Fatal(err)
	}
	// Baris uji lain di database yang sama ikut terhitung; yang dibandingkan
	// adalah kedua cara pada data yang SAMA.

	// Pecah biaya: ekstrak saja (query + scan) vs hitung kunci saja.
	var extracted []ch.IoTReadingRow
	_, extractOnly := peakHeap(func() {
		var err error
		if extracted, _, err = extractIoT(ctx, sourcePool, time.Time{}); err != nil {
			t.Fatal(err)
		}
	})
	_, keyOnly := peakHeap(func() {
		for _, r := range extracted {
			datalake.RowKey(iotSourceTable, r)
		}
	})
	t.Logf("pecahan waktu: extract saja %v, RowKey saja %v (%d baris)", extractOnly.Round(time.Millisecond), keyOnly.Round(time.Millisecond), len(extracted))
	extracted = nil

	var oldKeys map[string]struct{}
	oldPeak, oldTime := peakHeap(func() {
		rows, _, err := extractIoT(ctx, sourcePool, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		oldKeys = make(map[string]struct{}, len(rows))
		for _, r := range rows {
			k, ok := datalake.RowKey(iotSourceTable, r)
			if !ok {
				t.Fatal("no key")
			}
			oldKeys[k] = struct{}{}
		}
	})
	oldLen := len(oldKeys)
	oldKeys = nil

	var newKeys *datalake.KeySet
	newPeak, newTime := peakHeap(func() {
		var err error
		if newKeys, err = LiveKeysIoT(ctx, sourcePool); err != nil {
			t.Fatal(err)
		}
	})
	t.Logf("rows=%d  old(materialize+string map): %v, +%d MB   new(stream+KeySet): %v, +%d MB   keys old=%d new=%d",
		n, oldTime.Round(time.Millisecond), oldPeak>>20, newTime.Round(time.Millisecond), newPeak>>20, oldLen, newKeys.Len())
	if newKeys.Len() != oldLen {
		t.Errorf("jumlah kunci berbeda: old=%d new=%d", oldLen, newKeys.Len())
	}
}
