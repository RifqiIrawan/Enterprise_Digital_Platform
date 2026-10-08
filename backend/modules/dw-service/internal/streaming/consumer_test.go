package streaming

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func fastRetry(t *testing.T) {
	t.Helper()
	oldA, oldW := maxAttempts, handlerRetryWait
	maxAttempts, handlerRetryWait = 3, time.Millisecond
	t.Cleanup(func() { maxAttempts, handlerRetryWait = oldA, oldW })
}

func neverDLQ(t *testing.T) deadLetterFn {
	return func(context.Context, []byte, error) error {
		t.Error("dead-letter tidak boleh dipanggil")
		return nil
	}
}

func TestProcessMessage_SuccessAfterRetry(t *testing.T) {
	fastRetry(t)
	calls := 0
	h := func([]byte) error {
		calls++
		if calls < 3 {
			return errors.New("clickhouse sementara down")
		}
		return nil
	}
	before := testutil.ToFloat64(eventsTotal.WithLabelValues("t.ok", "processed"))
	if !processMessage(context.Background(), "t.ok", []byte(`{}`), time.Now(), h, neverDLQ(t)) {
		t.Fatal("pesan harus boleh di-commit setelah retry sukses")
	}
	if calls != 3 {
		t.Fatalf("handler dipanggil %d kali, mau 3", calls)
	}
	if got := testutil.ToFloat64(eventsTotal.WithLabelValues("t.ok", "processed")) - before; got != 1 {
		t.Fatalf("processed counter naik %v, mau 1", got)
	}
}

func TestProcessMessage_DeadLettersAfterMaxAttempts(t *testing.T) {
	fastRetry(t)
	calls := 0
	var dlqValue []byte
	var dlqCause error
	dlq := func(_ context.Context, v []byte, cause error) error {
		dlqValue, dlqCause = v, cause
		return nil
	}
	h := func([]byte) error { calls++; return errors.New("boom") }
	if !processMessage(context.Background(), "t.dlq", []byte(`{"entity_id":"x"}`), time.Time{}, h, dlq) {
		t.Fatal("pesan yang sudah aman di DLQ harus boleh di-commit")
	}
	if calls != 3 {
		t.Fatalf("handler dipanggil %d kali, mau 3", calls)
	}
	if string(dlqValue) != `{"entity_id":"x"}` || dlqCause == nil || dlqCause.Error() != "boom" {
		t.Fatalf("DLQ menerima value=%q cause=%v", dlqValue, dlqCause)
	}
	if testutil.ToFloat64(eventsTotal.WithLabelValues("t.dlq", "dead_lettered")) != 1 {
		t.Fatal("dead_lettered counter harus 1")
	}
}

func TestProcessMessage_DLQFailureDoesNotCommit(t *testing.T) {
	fastRetry(t)
	h := func([]byte) error { return errors.New("boom") }
	dlq := func(context.Context, []byte, error) error { return errors.New("kafka down") }
	if processMessage(context.Background(), "t.dlqfail", []byte(`{}`), time.Time{}, h, dlq) {
		t.Fatal("pesan TIDAK boleh di-commit kalau DLQ juga gagal (event akan hilang)")
	}
}

func TestProcessMessage_CancelledContextDoesNotCommit(t *testing.T) {
	fastRetry(t)
	handlerRetryWait = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	h := func([]byte) error { cancel(); return errors.New("boom") }
	if processMessage(ctx, "t.cancel", []byte(`{}`), time.Time{}, h, neverDLQ(t)) {
		t.Fatal("pesan TIDAK boleh di-commit saat shutdown di tengah retry")
	}
}
