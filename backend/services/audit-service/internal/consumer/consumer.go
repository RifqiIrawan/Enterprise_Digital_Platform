package consumer

import (
	"context"
	"errors"
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
	"company.branch.updated",
	"company.branch.deleted",
	"company.department.created",
	"company.department.updated",
	"company.department.deleted",
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
	"crm.lead.created",
	"crm.lead.updated",
	"crm.lead.converted",
	"crm.account.created",
	"crm.account.updated",
	"crm.contact.created",
	"crm.contact.updated",
	"crm.opportunity.created",
	"crm.opportunity.updated",
	"crm.opportunity.stage_changed",
	"crm.opportunity.won",
	"crm.opportunity.lost",
	"crm.activity.created",
	"crm.activity.updated",
	"ticketing.category.created",
	"ticketing.category.updated",
	"ticketing.ticket.created",
	"ticketing.ticket.updated",
	"ticketing.ticket.closed",
	"ticketing.ticket.reopened",
	"ticketing.comment.created",
	"ecommerce.order.created",
	"ecommerce.order.updated",
	"ecommerce.order.paid",
	"ecommerce.order.shipped",
	"ecommerce.order.delivered",
	"ecommerce.order.cancelled",
	"fleet.vehicle.created",
	"fleet.vehicle.updated",
	"fleet.driver.created",
	"fleet.driver.updated",
	"fleet.delivery.created",
	"fleet.delivery.updated",
	"fleet.delivery.dispatched",
	"fleet.delivery.delivered",
	"fleet.delivery.cancelled",
	"project.project.created",
	"project.project.updated",
	"project.project.activated",
	"project.project.held",
	"project.project.completed",
	"project.project.cancelled",
	"project.task.created",
	"project.task.updated",
	"project.timesheet.created",
	"project.timesheet.approved",
	"project.timesheet.rejected",
	"project.cost.posted",
}

const (
	retryBaseDelay = 3 * time.Second
	retryMaxDelay  = 30 * time.Second

	// topicPollInterval: jarak antar pengecekan metadata broker selagi masih
	// ada topic yang belum muncul. Satu panggilan metadata melayani SELURUH
	// daftar Topics, jadi ini tidak ikut membesar seiring bertambahnya modul.
	topicPollInterval = 15 * time.Second
	metadataDialTimeout = 5 * time.Second
)

// Start menunggu sebuah topic benar-benar ADA di broker sebelum menjalankan
// consumer untuknya, lalu satu goroutine per topic yang membuat kafka.Reader
// baru setiap kali error.
//
// Kenapa harus menunggu topic-nya ada dulu?
// Seluruh 100+ reader ini berbagi SATU consumer group. Kalau audit-service
// start saat Kafka masih kosong (mis. `docker compose up` menyalakan keduanya
// bersamaan), semua reader melakukan JoinGroup untuk topic yang belum ada
// sekaligus. Yang terjadi kemudian bukan error yang bisa ditangani: kafka-go
// memblokir di dalam ReadMessage sambil retry sendiri TANPA pernah
// mengembalikan error, jadi loop recreate di bawah — yang sepenuhnya digerakkan
// oleh error — tidak pernah terpicu. Gejalanya di lapangan: log consumer diam
// total, `kafka-consumer-groups --describe` menunjukkan partisi tanpa anggota
// aktif dan lag yang tidak pernah turun, dan satu-satunya jalan keluar adalah
// restart proses secara manual.
//
// Menunggu metadata lebih dulu menutup jalur itu di sumbernya: tidak ada
// anggota group yang pernah dibuat untuk topic yang belum ada, sehingga group-
// nya tidak pernah masuk keadaan tersebut. Loop recreate berbasis error tetap
// dipertahankan sebagai penanganan gangguan biasa (broker restart, network
// blip) setelah reader-nya berhasil jalan.
func Start(ctx context.Context, brokers, groupID string, handler func(topic string, value []byte)) {
	go superviseTopics(ctx, strings.Split(brokers, ","), groupID, handler)
}

// superviseTopics memeriksa metadata broker secara berkala dan menjalankan
// consumer untuk tiap topic BEGITU topic itu muncul. Berhenti sendiri setelah
// seluruh Topics punya consumer, jadi tidak ada polling yang berjalan selamanya
// pada deployment yang sehat.
func superviseTopics(ctx context.Context, brokers []string, groupID string, handler func(topic string, value []byte)) {
	started := make(map[string]bool, len(Topics))

	for {
		if ctx.Err() != nil {
			return
		}

		existing, err := listTopics(ctx, brokers)
		if err != nil {
			log.Printf("consumer: gagal membaca metadata topic dari broker, coba lagi dalam %s: %v", topicPollInterval, err)
		} else {
			for _, topic := range pendingTopics(started, existing) {
				started[topic] = true
				go consumeTopic(ctx, brokers, groupID, topic, handler)
			}
			if len(started) == len(Topics) {
				log.Printf("consumer: seluruh %d topic sudah ada dan punya consumer", len(Topics))
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(topicPollInterval):
		}
	}
}

// pendingTopics memilih topic yang sudah ada di broker tapi belum punya
// consumer. Dipisah sebagai fungsi murni supaya bisa diuji tanpa Kafka.
func pendingTopics(started, existing map[string]bool) []string {
	var out []string
	for _, topic := range Topics {
		if !started[topic] && existing[topic] {
			out = append(out, topic)
		}
	}
	return out
}

// listTopics mengembalikan himpunan topic yang diketahui broker.
//
// ReadPartitions dipanggil TANPA argumen dengan sengaja: memberi nama topic
// membuatnya jadi permintaan metadata untuk topic tersebut, dan broker dengan
// `auto.create.topics.enable=true` (default, termasuk di infra/docker-compose)
// akan membuat topic itu saat itu juga. Kalau itu terjadi, pengecekan ini
// selalu bernilai benar dan tidak menyaring apa pun — sekaligus mengotori
// broker dengan 100+ topic kosong yang dibuat oleh consumer, bukan producer.
func listTopics(ctx context.Context, brokers []string) (map[string]bool, error) {
	dialer := &kafka.Dialer{Timeout: metadataDialTimeout}
	var lastErr error

	for _, broker := range brokers {
		conn, err := dialer.DialContext(ctx, "tcp", strings.TrimSpace(broker))
		if err != nil {
			lastErr = err
			continue
		}
		partitions, err := conn.ReadPartitions()
		conn.Close()
		if err != nil {
			lastErr = err
			continue
		}

		topics := make(map[string]bool, len(partitions))
		for _, p := range partitions {
			topics[p.Topic] = true
		}
		return topics, nil
	}

	if lastErr == nil {
		lastErr = errNoBrokers
	}
	return nil, lastErr
}

var errNoBrokers = errors.New("tidak ada broker Kafka yang bisa dihubungi")

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
