package consumer

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

// Topics yang dikonsumsi audit-service, sesuai konvensi di
// infra/kafka/topics.md dan event yang dipublikasikan auth-service,
// company-service, dan rbac-service.
var Topics = []string{
	"auth.user.registered",
	"auth.user.logged_in",
	"company.company.created",
	"company.company.updated",
	"company.branch.created",
	"company.department.created",
	"rbac.role.created",
	"rbac.role.updated",
	"rbac.role.deleted",
	"rbac.role.permissions_updated",
	"rbac.role.assigned",
	"rbac.role.revoked",
	"hr.employee.created",
	"hr.employee.updated",
	"hr.attendance.created",
	"hr.attendance.updated",
	"hr.payroll.processed",
	"hr.payroll.posted",
}

// Start menjalankan satu Reader goroutine per topic. Setiap reader retry
// otomatis (dengan jeda) bila broker belum tersedia, tanpa membuat proses
// audit-service gagal start hanya karena Kafka belum jalan.
func Start(ctx context.Context, brokers, groupID string, handler func(topic string, value []byte)) {
	brokerList := strings.Split(brokers, ",")
	for _, topic := range Topics {
		go consumeTopic(ctx, brokerList, groupID, topic, handler)
	}
}

func consumeTopic(ctx context.Context, brokers []string, groupID, topic string, handler func(topic string, value []byte)) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		GroupID:  groupID,
		Topic:    topic,
		MinBytes: 1,
		MaxBytes: 10e6,
	})
	defer reader.Close()

	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("consumer[%s]: read error, retrying in 3s: %v", topic, err)
			time.Sleep(3 * time.Second)
			continue
		}
		handler(topic, msg.Value)
	}
}
