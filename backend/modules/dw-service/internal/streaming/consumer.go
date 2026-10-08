// Package streaming mengimplementasikan Kafka Streaming ETL untuk dw-service:
// konsumsi event bisnis → single-row Postgres lookup → insert ClickHouse.
// Ini melengkapi (bukan menggantikan) batch ETL di internal/etl yang masih
// berjalan sebagai backfill/recovery setiap 5 menit.
//
// Pola recreate-reader identik dengan audit-service consumer (fix Known Issue
// #2, commit c925b0f): Reader baru dibuat tiap iterasi retry supaya fresh
// JoinGroup + metadata fetch — bukan retry ReadMessage pada reader yang sama
// yang bisa stuck kalau topic belum ada saat reader pertama kali start.
//
// Delivery: at-least-once. Offset di-commit HANYA setelah handler sukses
// (atau event berhasil dipindah ke dead-letter topic <topic>.dlq setelah
// maxAttempts gagal). Handler idempoten karena ReplacingMergeTree.
package streaming

import (
	"context"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	retryBaseDelay = 3 * time.Second
	retryMaxDelay  = 30 * time.Second
)

// Variabel (bukan const) supaya test bisa mempercepatnya.
var (
	maxAttempts      = 3
	handlerRetryWait = 1 * time.Second // dikali 2 tiap attempt
)

// dlqSuffix: event yang gagal permanen dikirim ke "<topic>.dlq".
const dlqSuffix = ".dlq"

type deadLetterFn func(ctx context.Context, value []byte, cause error) error

// processMessage menjalankan handler dengan retry. Return true kalau pesan
// boleh di-commit (sukses, atau sudah aman di dead-letter). Return false kalau
// pesan TIDAK boleh di-commit (ctx dibatalkan, atau dead-letter gagal) —
// caller harus berhenti supaya pesan dikirim ulang oleh Kafka.
func processMessage(ctx context.Context, topic string, value []byte, sent time.Time,
	handler func([]byte) error, dlq deadLetterFn) bool {

	var lastErr error
	wait := handlerRetryWait
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		start := time.Now()
		lastErr = handler(value)
		handlerDuration.WithLabelValues(topic).Observe(time.Since(start).Seconds())
		if lastErr == nil {
			eventsTotal.WithLabelValues(topic, "processed").Inc()
			if !sent.IsZero() {
				eventLatency.WithLabelValues(topic).Observe(time.Since(sent).Seconds())
			}
			return true
		}

		log.Printf("dw-streaming[%s]: handler error (attempt %d/%d): %v", topic, attempt, maxAttempts, lastErr)
		if attempt == maxAttempts {
			break
		}
		retriesTotal.WithLabelValues(topic).Inc()
		select {
		case <-ctx.Done():
			return false
		case <-time.After(wait):
		}
		wait *= 2
	}

	if err := dlq(ctx, value, lastErr); err != nil {
		log.Printf("dw-streaming[%s]: dead-letter publish failed, message will be redelivered: %v", topic, err)
		return false
	}
	eventsTotal.WithLabelValues(topic, "dead_lettered").Inc()
	log.Printf("dw-streaming[%s]: event moved to %s%s after %d attempts", topic, topic, dlqSuffix, maxAttempts)
	return true
}

// consumeTopic membuat kafka.Reader baru di setiap iterasi retry.
// Exponential backoff (3s→30s), di-reset ke base delay kalau gotMsg=true.
func consumeTopic(ctx context.Context, brokers []string, groupID, topic string, handler func([]byte) error) {
	delay := retryBaseDelay

	writer := &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  topic + dlqSuffix,
		AllowAutoTopicCreation: true,
		RequiredAcks:           kafka.RequireAll,
	}
	defer writer.Close()
	dlq := func(ctx context.Context, value []byte, cause error) error {
		return writer.WriteMessages(ctx, kafka.Message{
			Value:   value,
			Headers: []kafka.Header{{Key: "dw-error", Value: []byte(cause.Error())}},
		})
	}

	for {
		if ctx.Err() != nil {
			return
		}

		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:  brokers,
			GroupID:  groupID,
			Topic:    topic,
			MinBytes: 1,
			MaxBytes: 10e6,
			MaxWait:  1 * time.Second,
		})

		gotMsg := drainReader(ctx, reader, topic, handler, dlq)
		reader.Close()

		if ctx.Err() != nil {
			return
		}

		if gotMsg {
			delay = retryBaseDelay
			log.Printf("dw-streaming[%s]: reader stopped after receiving messages, recreating in %s", topic, delay)
		} else {
			log.Printf("dw-streaming[%s]: reader stopped without messages, recreating in %s", topic, delay)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		if delay < retryMaxDelay {
			delay *= 2
			if delay > retryMaxDelay {
				delay = retryMaxDelay
			}
		}
	}
}

// drainReader membaca pesan sampai error atau ctx selesai. FetchMessage +
// CommitMessages (bukan ReadMessage, yang auto-commit sebelum handler jalan
// sehingga event hilang kalau handler gagal).
// Return gotMsg=true kalau minimal satu pesan berhasil diproses.
func drainReader(ctx context.Context, reader *kafka.Reader, topic string, handler func([]byte) error, dlq deadLetterFn) (gotMsg bool) {
	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("dw-streaming[%s]: read error: %v", topic, err)
			}
			return gotMsg
		}
		if !gotMsg {
			log.Printf("dw-streaming[%s]: connected, first message received (offset %d)", topic, msg.Offset)
		}

		if !processMessage(ctx, topic, msg.Value, msg.Time, handler, dlq) {
			return gotMsg
		}
		gotMsg = true

		if err := reader.CommitMessages(ctx, msg); err != nil {
			if ctx.Err() == nil {
				log.Printf("dw-streaming[%s]: commit offset %d: %v", topic, msg.Offset, err)
			}
			return gotMsg
		}
	}
}
