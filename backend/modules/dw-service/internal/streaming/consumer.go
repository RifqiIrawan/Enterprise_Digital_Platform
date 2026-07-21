// Package streaming mengimplementasikan Kafka Streaming ETL untuk dw-service:
// konsumsi event bisnis → single-row Postgres lookup → insert ClickHouse.
// Ini melengkapi (bukan menggantikan) batch ETL di internal/etl yang masih
// berjalan sebagai backfill/recovery setiap 5 menit.
//
// Pola recreate-reader identik dengan audit-service consumer (fix Known Issue
// #2, commit c925b0f): Reader baru dibuat tiap iterasi retry supaya fresh
// JoinGroup + metadata fetch — bukan retry ReadMessage pada reader yang sama
// yang bisa stuck kalau topic belum ada saat reader pertama kali start.
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

// consumeTopic membuat kafka.Reader baru di setiap iterasi retry.
// Exponential backoff (3s→30s), di-reset ke base delay kalau gotMsg=true.
func consumeTopic(ctx context.Context, brokers []string, groupID, topic string, handler func([]byte)) {
	delay := retryBaseDelay

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

		gotMsg := drainReader(ctx, reader, topic, handler)
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

// drainReader membaca pesan sampai error atau ctx selesai.
// Return gotMsg=true kalau minimal satu pesan berhasil diproses.
func drainReader(ctx context.Context, reader *kafka.Reader, topic string, handler func([]byte)) (gotMsg bool) {
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("dw-streaming[%s]: read error: %v", topic, err)
			}
			return gotMsg
		}
		if !gotMsg {
			log.Printf("dw-streaming[%s]: connected, first message received (offset %d)", topic, msg.Offset)
		}
		gotMsg = true
		handler(msg.Value)
	}
}
