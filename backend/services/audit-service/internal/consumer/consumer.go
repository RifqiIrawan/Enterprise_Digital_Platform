package consumer

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

// Topics yang dikonsumsi audit-service, sesuai konvensi <domain>.<entity>.<action>
// di infra/kafka/topics.md dan event yang benar-benar dipublikasikan tiap
// service (lihat h.events.Publish("...") di masing-masing internal/httpapi).
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
	"finance.account.created",
	"finance.account.updated",
	"finance.invoice.created",
	"finance.invoice.posted",
	"finance.journal.created",
	"finance.journal.posted",
	"hr.employee.created",
	"hr.employee.updated",
	"hr.attendance.created",
	"hr.attendance.updated",
	"hr.payroll.processed",
	"hr.payroll.posted",
	"sales.customer.created",
	"sales.customer.updated",
	"sales.quotation.created",
	"sales.quotation.sent",
	"sales.quotation.accepted",
	"sales.quotation.rejected",
	"sales.quotation.converted",
	"sales.order.created",
	"sales.order.confirmed",
	"sales.order.fulfilled",
	"sales.order.invoiced",
	"purchasing.supplier.created",
	"purchasing.supplier.updated",
	"purchasing.requisition.created",
	"purchasing.requisition.submitted",
	"purchasing.requisition.approved",
	"purchasing.requisition.rejected",
	"purchasing.requisition.converted",
	"purchasing.order.created",
	"purchasing.order.confirmed",
	"purchasing.order.received",
	"purchasing.order.invoiced",
	"warehouse.product.created",
	"warehouse.product.updated",
	"warehouse.warehouse.created",
	"warehouse.warehouse.updated",
	"warehouse.stock.moved",
	"warehouse.stock.batch_moved",
	"warehouse.transfer.created",
	"warehouse.transfer.confirmed",
	"warehouse.opname.created",
	"warehouse.opname.posted",
	"production.bom.created",
	"production.bom.updated",
	"production.work_order.created",
	"production.work_order.started",
	"production.work_order.completed",
	"qc.standard.created",
	"qc.standard.updated",
	"qc.inspection.created",
	"asset.asset.created",
	"asset.asset.updated",
	"asset.maintenance.scheduled",
	"asset.maintenance.completed",
	"asset.maintenance.cancelled",
	"iot.device.registered",
	"iot.device.updated",
	"iot.alert.triggered",
	"iot.alert.acknowledged",
	"iot.alert.resolved",
}

const (
	retryBaseDelay = 3 * time.Second
	retryMaxDelay  = 30 * time.Second
)

// Start menjalankan satu goroutine per topic. Setiap goroutine membuat
// kafka.Reader BARU saat error — ini adalah perbedaan kunci dari implementasi
// lama yang hanya me-retry ReadMessage pada reader yang sama.
//
// Kenapa recreate Reader (bukan cuma retry ReadMessage)?
// Saat audit-service start sebelum sebuah topic pernah ada di Kafka, reader
// melakukan JoinGroup/SyncGroup consumer-group pada topic yang belum ada.
// Kafka biasanya auto-create topic kosong saat itu, tapi reader bisa masuk
// state internal yang korup (partition assignment kosong, offset stale) karena
// metadata topic belum sepenuhnya terpropagasi. Retry ReadMessage pada reader
// yang sama tidak menyembuhkan state ini — reader terus stuck sampai proses
// di-restart manual (Known Issue #2 di NEXT_SESSION.md).
//
// Dengan recreate Reader, setiap error memaksa fresh JoinGroup baru. Begitu
// topic sudah benar-benar ada (event pertama dipublikasikan oleh service lain),
// iterasi berikutnya akan berhasil mendapatkan partition assignment yang valid.
func Start(ctx context.Context, brokers, groupID string, handler func(topic string, value []byte)) {
	brokerList := strings.Split(brokers, ",")
	for _, topic := range Topics {
		go consumeTopic(ctx, brokerList, groupID, topic, handler)
	}
}

// consumeTopic membuat Reader baru di setiap iterasi retry. Delay antar retry
// menggunakan exponential backoff (3s → 6s → 12s → ... → 30s maks) dan
// di-reset ke base delay setiap kali Reader berhasil menerima minimal satu
// pesan (artinya koneksi & assignment pernah valid, error berikutnya lebih
// mungkin transient daripada structural).
func consumeTopic(ctx context.Context, brokers []string, groupID, topic string, handler func(topic string, value []byte)) {
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
			// MaxWait: batas tunggu server-side fetch sebelum broker
			// kembalikan batch kosong. Nilai pendek (1s) membuat
			// ReadMessage lebih responsif terhadap ctx.Done() saat idle
			// dan mempercepat recovery loop kalau ada error.
			MaxWait: 1 * time.Second,
		})

		gotMsg := drainReader(ctx, reader, topic, handler)
		reader.Close()

		if ctx.Err() != nil {
			return
		}

		if gotMsg {
			// Pernah dapat pesan → error ini kemungkinan transient
			// (broker restart, network blip). Reset ke base delay.
			delay = retryBaseDelay
			log.Printf("consumer[%s]: reader stopped after receiving messages, recreating in %s", topic, delay)
		} else {
			// Belum pernah dapat pesan sama sekali → kemungkinan
			// topic belum ada, atau broker belum reachable.
			// Pakai exponential backoff supaya tidak spam log.
			log.Printf("consumer[%s]: reader stopped without receiving any message, recreating in %s", topic, delay)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		// Exponential backoff, cap di retryMaxDelay.
		if delay < retryMaxDelay {
			delay *= 2
			if delay > retryMaxDelay {
				delay = retryMaxDelay
			}
		}
	}
}

// drainReader membaca pesan dari reader sampai error atau ctx selesai.
// Mengembalikan true kalau minimal satu pesan berhasil diproses — dipakai
// oleh consumeTopic untuk memutuskan apakah perlu reset backoff delay.
func drainReader(ctx context.Context, reader *kafka.Reader, topic string, handler func(string, []byte)) (gotMsg bool) {
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() == nil {
				// Hanya log kalau bukan shutdown normal.
				log.Printf("consumer[%s]: read error: %v", topic, err)
			}
			return gotMsg
		}
		if !gotMsg {
			log.Printf("consumer[%s]: connected, first message received (offset %d)", topic, msg.Offset)
		}
		gotMsg = true
		handler(topic, msg.Value)
	}
}
