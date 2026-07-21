# 09 — Kafka Streaming
## Enterprise Digital Platform (EDP)

---

## Overview

Apache Kafka dipakai sebagai **event bus** untuk audit trail dan near-realtime data warehousing. Semua service bisnis publish event setelah operasi selesai; dua consumer group membaca event ini untuk tujuan berbeda.

**Setup**: Single broker (bukan multi-broker cluster), KRaft mode (tidak butuh Zookeeper), tanpa Schema Registry, tanpa Kafka Connect.

---

## Infrastruktur

```yaml
# infra/docker-compose.yml
kafka:
  image: bitnamilegacy/kafka:3.7.1
  environment:
    KAFKA_CFG_NODE_ID: 0
    KAFKA_CFG_PROCESS_ROLES: controller,broker
    # PLAINTEXT: untuk koneksi dari host (go run), port 9092
    # INTERNAL: untuk koneksi container-to-container, port 29092
    KAFKA_CFG_LISTENERS: PLAINTEXT://:9092,INTERNAL://:29092,CONTROLLER://:9093
    KAFKA_CFG_ADVERTISED_LISTENERS: PLAINTEXT://localhost:9092,INTERNAL://kafka:29092
```

| Listener | Port | Dipakai oleh |
|----------|------|-------------|
| PLAINTEXT | 9092 | `go run` di host, test lokal |
| INTERNAL | 29092 | Container ke container (docker-compose, K8s) |

**Default di service config**: `KAFKA_BROKERS=localhost:9092`  
**Di docker-compose**: `KAFKA_BROKERS=kafka:29092`  
**Di K8s dev overlay**: `KAFKA_BROKERS=host.docker.internal:9092`

---

## Kafka UI

```
http://localhost:8099  (port 8099, bukan 8090 — diganti untuk menghindari konflik dengan production-service)
```

---

## Format Event

Semua service bisnis menggunakan format yang sama (didefinisikan lokal di setiap service, tidak ada shared package):

```go
type auditEvent struct {
    EventID       string    `json:"event_id"`       // UUID v4
    EventType     string    `json:"event_type"`      // "sales.order.fulfilled"
    SourceService string    `json:"source_service"`  // "sales-service"
    OccurredAt    time.Time `json:"occurred_at"`
    ActorUserID   *string   `json:"actor_user_id,omitempty"`
    CompanyID     *string   `json:"company_id,omitempty"`
    Action        string    `json:"action"`          // "create", "update", "delete"
    EntityType    string    `json:"entity_type"`     // "sales_order"
    EntityID      string    `json:"entity_id"`       // UUID entity yang diubah
    Payload       any       `json:"payload,omitempty"` // full struct entity
}
```

---

## Daftar Topics (65 Topics)

### Platform
```
auth.user.registered          auth.user.logged_in
company.company.created       company.company.updated
company.branch.created        company.department.created
rbac.role.created             rbac.role.updated         rbac.role.deleted
rbac.role.permissions_updated rbac.role.assigned        rbac.role.revoked
```

### Finance
```
finance.journal.created       finance.journal.posted
finance.invoice.created       finance.invoice.posted
```

### HR
```
hr.employee.created    hr.employee.updated
hr.attendance.created  hr.attendance.updated
hr.payroll.processed   hr.payroll.posted
```

### Sales
```
sales.customer.created    sales.customer.updated
sales.quotation.created   sales.quotation.sent
sales.quotation.accepted  sales.quotation.rejected  sales.quotation.converted
sales.order.created       sales.order.confirmed
sales.order.fulfilled     sales.order.invoiced
```

### Purchasing
```
purchasing.supplier.created     purchasing.supplier.updated
purchasing.requisition.created  purchasing.requisition.submitted
purchasing.requisition.approved purchasing.requisition.rejected purchasing.requisition.converted
purchasing.order.created        purchasing.order.confirmed
purchasing.order.received       purchasing.order.invoiced
```

### Warehouse
```
warehouse.product.created   warehouse.product.updated
warehouse.warehouse.created warehouse.warehouse.updated
warehouse.stock.moved       warehouse.stock.batch_moved
warehouse.transfer.created  warehouse.transfer.confirmed
warehouse.opname.created    warehouse.opname.posted
```

### Production, QC, Asset, IoT
```
production.bom.created         production.bom.updated
production.work_order.created  production.work_order.started  production.work_order.completed
qc.standard.created  qc.standard.updated  qc.inspection.created
asset.asset.created  asset.asset.updated
asset.maintenance.scheduled  asset.maintenance.completed  asset.maintenance.cancelled
iot.device.registered  iot.device.updated
iot.alert.triggered    iot.alert.acknowledged  iot.alert.resolved
```

**Tidak ada** `iot.reading.*` — readings adalah telemetri frekuensi tinggi, langsung ke Postgres via MQTT, tidak lewat Kafka.

---

## Consumer Groups

### `audit-service` — Subscribe semua 65 topics

**Package**: `backend/services/audit-service/internal/consumer/`

Pattern: 1 goroutine per topic, masing-masing dengan `kafka.Reader` yang **dibuat ulang** (bukan retry ReadMessage pada reader yang sama) — ini memastikan recovery kalau topic belum ada saat reader pertama kali start.

```go
// Setiap consumer goroutine:
for {
    reader := kafka.NewReader(config)       // fresh Reader setiap iterasi
    drainReader(ctx, reader, topic, handler) // baca sampai error
    reader.Close()
    time.Sleep(delay)  // exponential backoff 3s→30s
}
```

Handler: `func(topic string, value []byte)` → parse JSON → insert ke `audit_logs` (Postgres).

### `dw-service-streaming` — Subscribe 12 topics

**Package**: `backend/modules/dw-service/internal/streaming/`

Pattern identik dengan audit-service consumer. Handler: parse `entity_id` → query Postgres (1 baris, JOIN) → insert ClickHouse + lake.

---

## Publish Pattern (di setiap service)

```go
// internal/eventbus/eventbus.go — nil-safe, best-effort, non-blocking
func (p *Publisher) Publish(topic string, payload any) {
    if p == nil {
        return  // Kafka tidak tersedia → silently skip
    }
    go func() {  // non-blocking
        data, _ := json.Marshal(payload)
        p.writer.WriteMessages(ctx, kafka.Message{
            Topic: topic,
            Value: data,
        })
    }()
}
```

Kegagalan publish tidak menggagalkan operasi bisnis — Kafka adalah side-channel untuk audit trail dan DW, bukan bagian dari alur transaksional utama.

---

## Known Issue: Topic Auto-Creation

Bitnami Kafka default mengaktifkan `auto.create.topics.enable`. Kalau consumer start **sebelum** topic pernah ada, Kafka auto-create topic kosong dan reader join consumer group — tapi bisa masuk state korup. **Fix**: consumer menggunakan recreate-Reader-on-error pattern (bukan retry ReadMessage yang sama). Detail di commit `c925b0f`.
