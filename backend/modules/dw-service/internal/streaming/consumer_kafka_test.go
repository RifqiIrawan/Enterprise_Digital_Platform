package streaming

// Tes integrasi terhadap Kafka sungguhan (KAFKA_BROKERS, default localhost:9092).
// Di-skip kalau broker tidak tersedia.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

func kafkaBrokers(t *testing.T) []string {
	t.Helper()
	addr := getEnvStr("KAFKA_BROKERS", "localhost:9092")
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Skipf("Kafka tidak tersedia di %s: %v", addr, err)
	}
	c.Close()
	return []string{addr}
}

func createTopic(t *testing.T, brokers []string, topic string) {
	t.Helper()
	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	ctrl, err := conn.Controller()
	if err != nil {
		t.Fatalf("controller: %v", err)
	}
	cc, err := kafka.Dial("tcp", net.JoinHostPort(ctrl.Host, fmt.Sprint(ctrl.Port)))
	if err != nil {
		t.Fatalf("dial controller: %v", err)
	}
	defer cc.Close()
	if err := cc.CreateTopics(kafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1}); err != nil {
		t.Fatalf("create topic: %v", err)
	}
}

func publish(t *testing.T, brokers []string, topic, value string) {
	t.Helper()
	createTopic(t, brokers, topic)
	w := &kafka.Writer{Addr: kafka.TCP(brokers...), Topic: topic, AllowAutoTopicCreation: true, RequiredAcks: kafka.RequireAll}
	defer w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Topic baru butuh waktu untuk leader election; WriteMessages me-retry.
	if err := w.WriteMessages(ctx, kafka.Message{Value: []byte(value)}); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func TestConsume_FailingHandlerEndsInDeadLetterTopic(t *testing.T) {
	brokers := kafkaBrokers(t)
	fastRetry(t)
	topic := "dwtest.fail." + uuid.NewString()[:8]
	publish(t, brokers, topic, `{"entity_id":"abc"}`)

	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go consumeTopic(ctx, brokers, "dwtest-"+topic, topic, func([]byte) error {
		calls.Add(1)
		return errors.New("clickhouse down")
	})

	r := kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, Topic: topic + dlqSuffix, Partition: 0, MinBytes: 1, MaxBytes: 1e6})
	defer r.Close()
	rctx, rcancel := context.WithTimeout(ctx, 60*time.Second)
	defer rcancel()
	msg, err := r.ReadMessage(rctx)
	if err != nil {
		t.Fatalf("tidak ada pesan di %s%s: %v (handler dipanggil %d kali)", topic, dlqSuffix, err, calls.Load())
	}
	if string(msg.Value) != `{"entity_id":"abc"}` {
		t.Fatalf("payload DLQ = %q", msg.Value)
	}
	if len(msg.Headers) == 0 || msg.Headers[0].Key != "dw-error" || string(msg.Headers[0].Value) != "clickhouse down" {
		t.Fatalf("header dw-error salah: %+v", msg.Headers)
	}
	if calls.Load() != int32(maxAttempts) {
		t.Fatalf("handler dipanggil %d kali, mau %d", calls.Load(), maxAttempts)
	}
}

func TestConsume_CommitsOffsetOnlyAfterSuccess(t *testing.T) {
	brokers := kafkaBrokers(t)
	fastRetry(t)
	topic := "dwtest.ok." + uuid.NewString()[:8]
	group := "dwtest-" + topic
	publish(t, brokers, topic, `{"entity_id":"one"}`)

	// Consumer 1: sukses → offset ter-commit.
	got := make(chan string, 4)
	ctx1, cancel1 := context.WithCancel(context.Background())
	go consumeTopic(ctx1, brokers, group, topic, func(b []byte) error { got <- string(b); return nil })
	select {
	case <-got:
	case <-time.After(60 * time.Second):
		t.Fatal("event tidak pernah diproses")
	}
	time.Sleep(2 * time.Second) // beri waktu CommitMessages selesai
	cancel1()
	time.Sleep(time.Second)

	// Consumer 2 (group sama): tidak boleh menerima event yang sama lagi.
	var redelivered atomic.Int32
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go consumeTopic(ctx2, brokers, group, topic, func([]byte) error { redelivered.Add(1); return nil })
	time.Sleep(10 * time.Second)
	if n := redelivered.Load(); n != 0 {
		t.Fatalf("event dikirim ulang %d kali padahal sudah di-commit", n)
	}
}
