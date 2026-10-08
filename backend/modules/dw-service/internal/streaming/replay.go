package streaming

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

// replayGroupID dipisah dari group konsumen utama supaya offset DLQ punya
// jejaknya sendiri: event yang sudah di-replay tidak diputar ulang di panggilan
// berikutnya.
const replayGroupID = "dw-service-dlq-replay"

// replayIdle: berapa lama menunggu pesan berikutnya sebelum DLQ dianggap habis.
// Cukup panjang untuk JoinGroup pertama kali.
var replayIdle = 8 * time.Second

// KnownTopic melaporkan apakah topic punya handler streaming.
func KnownTopic(topic string) bool {
	_, ok := topicHandlers[topic]
	return ok
}

// Replay memindahkan sampai limit event dari "<topic>.dlq" kembali ke topic
// aslinya supaya diproses ulang oleh consumer biasa (jalankan setelah penyebab
// kegagalan diperbaiki). Offset DLQ di-commit setelah publish sukses, jadi
// event yang gagal dipublikasikan tetap ada di DLQ. Event yang gagal lagi
// akan kembali ke DLQ lewat jalur retry normal.
func Replay(ctx context.Context, brokers, topic string, limit int) (int, error) {
	if !KnownTopic(topic) {
		return 0, fmt.Errorf("topic %q tidak punya handler streaming", topic)
	}
	if limit <= 0 {
		return 0, errors.New("limit harus > 0")
	}
	brokerList := strings.Split(brokers, ",")

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokerList,
		GroupID:     replayGroupID,
		Topic:       topic + dlqSuffix,
		MinBytes:    1,
		MaxBytes:    10e6,
		MaxWait:     time.Second,
		StartOffset: kafka.FirstOffset,
	})
	defer reader.Close()
	writer := &kafka.Writer{Addr: kafka.TCP(brokerList...), Topic: topic, RequiredAcks: kafka.RequireAll}
	defer writer.Close()

	replayed := 0
	for replayed < limit {
		fctx, cancel := context.WithTimeout(ctx, replayIdle)
		msg, err := reader.FetchMessage(fctx)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return replayed, ctx.Err()
			}
			return replayed, nil // idle: DLQ habis
		}
		if err := writer.WriteMessages(ctx, kafka.Message{Value: msg.Value}); err != nil {
			return replayed, fmt.Errorf("publish ulang ke %s: %w", topic, err)
		}
		if err := reader.CommitMessages(ctx, msg); err != nil {
			return replayed, fmt.Errorf("commit offset DLQ: %w", err)
		}
		replayed++
	}
	return replayed, nil
}
