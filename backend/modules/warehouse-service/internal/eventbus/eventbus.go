package eventbus

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

// Publisher mempublikasikan event ke Kafka secara best-effort: kegagalan
// (mis. Kafka belum jalan di lokal) hanya dicatat ke log, tidak pernah
// menggagalkan request HTTP yang memicunya.
type Publisher struct {
	writer *kafka.Writer
}

func NewPublisher(brokers string) *Publisher {
	return &Publisher{
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(strings.Split(brokers, ",")...),
			Balancer:               &kafka.LeastBytes{},
			AllowAutoTopicCreation: true,
			WriteTimeout:           3 * time.Second,
			BatchTimeout:           50 * time.Millisecond,
		},
	}
}

func (p *Publisher) Publish(topic string, event any) {
	if p == nil || p.writer == nil {
		return
	}
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("eventbus: marshal error for topic %s: %v", topic, err)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := p.writer.WriteMessages(ctx, kafka.Message{Topic: topic, Value: data}); err != nil {
			log.Printf("eventbus: publish to %s failed (continuing without it): %v", topic, err)
		}
	}()
}

func (p *Publisher) Close() {
	if p != nil && p.writer != nil {
		_ = p.writer.Close()
	}
}
