package streaming

import (
	"context"
	"log"
	"strings"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
	"github.com/enterprise-digital-platform/dw-service/internal/sourcedb"
)

// handlerFn adalah tipe handler per-topic: terima raw JSON → lookup Postgres
// → insert ClickHouse (+lake best-effort).
type handlerFn func(context.Context, []byte, *sourcedb.Pools, *ch.Client, *datalake.Client) error

// topicHandlers memetakan setiap Kafka topic ke handler-nya.
//
// IoT readings SENGAJA tidak ada di sini — per infra/kafka/topics.md,
// iot.reading.* tidak dipublikasikan ke Kafka (telemetri frekuensi tinggi
// langsung ke Postgres via MQTT, bukan lewat Kafka). Batch ETL di
// internal/etl/iot.go tetap menangani IoT readings setiap 5 menit.
//
// 12 topic → 8 domain handler (sales/purchasing/asset masing-masing punya
// 2 trigger berbeda tapi handler yang sama, karena keduanya cuma mengubah
// status entitas dan kita extract ulang seluruh data entitas itu).
var topicHandlers = map[string]handlerFn{
	// Finance: journal entry di-post → extract semua lines-nya
	"finance.journal.posted": handleFinanceJournalPosted,

	// Sales: SO di-fulfill atau di-invoice → status berubah, extract ulang lines
	"sales.order.fulfilled": handleSalesOrderEvent,
	"sales.order.invoiced":  handleSalesOrderEvent,

	// Warehouse: single movement atau batch movement (per reference_id)
	"warehouse.stock.moved":       handleStockMoved,
	"warehouse.stock.batch_moved": handleStockBatchMoved,

	// HR: payroll run di-post → extract semua details-nya
	"hr.payroll.posted": handleHRPayrollPosted,

	// Purchasing: PO diterima atau di-invoice → status berubah, extract ulang lines
	"purchasing.order.received": handlePurchasingOrderEvent,
	"purchasing.order.invoiced": handlePurchasingOrderEvent,

	// Production: WO selesai → update fact (quantity_produced terisi)
	"production.work_order.completed": handleProductionWOCompleted,

	// QC: inspeksi dibuat → langsung final (tidak ada status DRAFT di QC)
	"qc.inspection.created": handleQCInspectionCreated,

	// Asset: maintenance selesai atau dibatalkan → status berubah
	"asset.maintenance.completed": handleAssetMaintenanceEvent,
	"asset.maintenance.cancelled": handleAssetMaintenanceEvent,
}

// Start menjalankan satu goroutine konsumer per topic. Setiap goroutine:
//  1. Menerima Kafka event (JSON envelope dengan entity_id)
//  2. Memanggil handler yang tepat untuk domain itu
//  3. Handler lookup ke Postgres (single-row JOIN query) lalu insert ke ClickHouse
//
// Streaming berjalan PARALEL dengan batch ETL di cmd/server/main.go —
// keduanya menulis ke tabel ClickHouse yang sama. ReplacingMergeTree
// (dengan version column synced_at) meng-upsert baris yang sama secara
// otomatis, sehingga tidak ada duplikat permanen.
//
// dest dan lake boleh nil (pola best-effort yang sama dengan service lain
// di project ini) — Start akan log warning dan tidak spawn goroutine kalau
// dest=nil (tidak ada gunanya consume Kafka kalau tidak bisa nulis ke mana-
// mana). lake=nil tetap jalan normal, hanya data lake write yang di-skip.
func Start(ctx context.Context, brokers, groupID string, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) {
	if dest == nil {
		log.Printf("dw-streaming: ClickHouse tidak tersedia, streaming consumer tidak dijalankan")
		return
	}

	brokerList := strings.Split(brokers, ",")

	for topic, handler := range topicHandlers {
		h := handler // capture untuk goroutine
		t := topic
		go consumeTopic(ctx, brokerList, groupID, t, func(raw []byte) {
			if err := h(ctx, raw, sources, dest, lake); err != nil {
				log.Printf("dw-streaming[%s]: handler error: %v", t, err)
			}
		})
	}

	log.Printf("dw-streaming: started %d topic consumers (brokers: %s, group: %s)",
		len(topicHandlers), brokers, groupID)
}
