# 09 — Kafka Streaming
## Enterprise Data Center Simulator (EDCS)

---

## 📡 Overview

Apache Kafka berfungsi sebagai **sistem saraf pusat** EDCS — semua domain event mengalir melalui Kafka, memungkinkan decoupling total antar microservice dan menjadi fondasi untuk real-time analytics, audit trail, dan event sourcing.

---

## 🏗️ Kafka Cluster Topology

```
┌──────────────────────────────────────────────────────────┐
│                  KAFKA CLUSTER (KRaft)                   │
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │ Broker 0 │  │ Broker 1 │  │ Broker 2 │              │
│  │(Controller)│ │          │  │          │              │
│  │ Port 9092│  │ Port 9092│  │ Port 9092│              │
│  └──────────┘  └──────────┘  └──────────┘              │
│                                                          │
│  ┌────────────────────────────────────────┐             │
│  │         Schema Registry                │             │
│  │         Port 8081                      │             │
│  └────────────────────────────────────────┘             │
│                                                          │
│  ┌────────────────────────────────────────┐             │
│  │         Kafka Connect                  │             │
│  │         Port 8083 (REST API)           │             │
│  └────────────────────────────────────────┘             │
│                                                          │
│  ┌────────────────────────────────────────┐             │
│  │         Kafka UI (Provectus)           │             │
│  │         Port 8080                      │             │
│  └────────────────────────────────────────┘             │
└──────────────────────────────────────────────────────────┘
```

---

## 📋 Topic Registry (Lengkap)

### Domain: ERP
| Topic | Partitions | Replication | Retention | Producer | Schema |
|-------|-----------|-------------|-----------|----------|--------|
| `erp.master-data.product.updated` | 6 | 3 | 30d | erp-core | Avro |
| `erp.master-data.customer.updated` | 6 | 3 | 30d | erp-core | Avro |
| `erp.master-data.vendor.updated` | 6 | 3 | 30d | erp-core | Avro |

### Domain: HRIS
| Topic | Partitions | Replication | Retention | Producer |
|-------|-----------|-------------|-----------|----------|
| `hris.employee.created` | 6 | 3 | 90d | hris |
| `hris.employee.updated` | 6 | 3 | 90d | hris |
| `hris.employee.terminated` | 3 | 3 | 365d | hris |
| `hris.attendance.logged` | 12 | 3 | 30d | hris |
| `hris.leave.approved` | 3 | 3 | 90d | hris |
| `hris.payroll.processed` | 3 | 3 | 365d | payroll |
| `hris.payroll.disbursed` | 3 | 3 | 365d | payroll |

### Domain: CRM & Sales
| Topic | Partitions | Replication | Retention |
|-------|-----------|-------------|-----------|
| `crm.lead.created` | 6 | 3 | 60d |
| `crm.lead.converted` | 6 | 3 | 180d |
| `crm.opportunity.stage-changed` | 6 | 3 | 90d |
| `crm.ticket.created` | 6 | 3 | 60d |
| `crm.ticket.resolved` | 6 | 3 | 60d |
| `sales.order.created` | 12 | 3 | 90d |
| `sales.order.confirmed` | 12 | 3 | 90d |
| `sales.order.cancelled` | 6 | 3 | 90d |
| `sales.invoice.generated` | 6 | 3 | 365d |

### Domain: WMS & MES
| Topic | Partitions | Replication | Retention |
|-------|-----------|-------------|-----------|
| `wms.stock.receipt` | 12 | 3 | 90d |
| `wms.stock.issue` | 12 | 3 | 90d |
| `wms.stock.below-reorder` | 6 | 3 | 30d |
| `wms.shipment.dispatched` | 12 | 3 | 90d |
| `mes.workorder.started` | 6 | 3 | 60d |
| `mes.workorder.completed` | 6 | 3 | 60d |
| `mes.quality.passed` | 6 | 3 | 60d |
| `mes.quality.failed` | 6 | 3 | 180d |
| `mes.machine.downtime` | 6 | 3 | 90d |

### Domain: Finance & Procurement
| Topic | Partitions | Replication | Retention |
|-------|-----------|-------------|-----------|
| `finance.journal.posted` | 6 | 3 | 365d |
| `finance.payment.processed` | 6 | 3 | 365d |
| `finance.budget.exceeded` | 3 | 3 | 90d |
| `procurement.pr.approved` | 6 | 3 | 90d |
| `procurement.po.created` | 6 | 3 | 90d |
| `procurement.po.approved` | 6 | 3 | 90d |
| `procurement.gr.completed` | 6 | 3 | 90d |

### Domain: IoT
| Topic | Partitions | Replication | Retention |
|-------|-----------|-------------|-----------|
| `iot.sensor.reading` | 24 | 3 | 7d |
| `iot.sensor.anomaly` | 12 | 3 | 30d |
| `iot.device.connected` | 6 | 3 | 30d |
| `iot.device.alert` | 12 | 3 | 30d |
| `iot.device.firmware-updated` | 3 | 3 | 60d |

### Platform
| Topic | Partitions | Replication | Retention |
|-------|-----------|-------------|-----------|
| `platform.notification.send` | 6 | 3 | 7d |
| `platform.audit.event` | 12 | 3 | 365d |
| `{any-topic}.DLQ` | 3 | 3 | 30d |

---

## 🏗️ Event Schema Standard

### Base Event Structure (Avro)
```json
{
  "type": "record",
  "name": "BaseEvent",
  "namespace": "com.edcs",
  "fields": [
    {"name": "event_id",       "type": "string",  "doc": "UUID v4"},
    {"name": "event_type",     "type": "string",  "doc": "SNAKE_CASE nama event"},
    {"name": "event_version",  "type": "string",  "default": "1.0"},
    {"name": "occurred_at",    "type": "long",    "logicalType": "timestamp-millis"},
    {"name": "source_service", "type": "string"},
    {"name": "correlation_id", "type": ["null","string"], "default": null},
    {"name": "causation_id",   "type": ["null","string"], "default": null},
    {"name": "tenant_id",      "type": ["null","string"], "default": null},
    {"name": "payload",        "type": "string",  "doc": "JSON string payload"}
  ]
}
```

### Contoh Domain Event
```json
// hris.employee.created
{
  "event_id": "550e8400-e29b-41d4-a716-446655440000",
  "event_type": "EMPLOYEE_CREATED",
  "event_version": "1.0",
  "occurred_at": 1720512000000,
  "source_service": "hris-service",
  "correlation_id": "req-abc-123",
  "causation_id": null,
  "tenant_id": "edcs-demo",
  "payload": "{\"employee_id\":\"...\",\"employee_code\":\"EMP001\",\"full_name\":\"John Doe\"}"
}
```

---

## ⚡ Kafka Streams: Real-time Processing

### 1. Stock Level Aggregator
```java
// Realtime aggregasi stock level per product
StreamsBuilder builder = new StreamsBuilder();

KStream<String, StockMovementEvent> movements = builder
    .stream("wms.stock.movement", Consumed.with(Serdes.String(), stockMovementSerde));

KTable<String, StockSummary> stockLevels = movements
    .groupByKey()
    .aggregate(
        StockSummary::new,
        (productId, event, summary) -> {
            if (event.getType() == RECEIPT) {
                summary.addQty(event.getQuantity());
            } else if (event.getType() == ISSUE) {
                summary.subtractQty(event.getQuantity());
            }
            return summary;
        },
        Materialized.<String, StockSummary, KeyValueStore<Bytes, byte[]>>
            as("stock-levels-store")
            .withValueSerde(stockSummarySerde)
    );

// Output ke topic baru
stockLevels.toStream().to("wms.stock.current-levels");
```

### 2. Real-time Sales Aggregator
```java
KStream<String, SalesOrderEvent> orders = builder
    .stream("sales.order.confirmed");

KTable<Windowed<String>, Double> hourlySales = orders
    .selectKey((k, v) -> v.getProductCategory())
    .groupByKey()
    .windowedBy(TimeWindows.ofSizeWithNoGrace(Duration.ofHours(1)))
    .aggregate(
        () -> 0.0,
        (key, event, total) -> total + event.getTotalAmount(),
        Materialized.as("hourly-sales-store")
    );

hourlySales.toStream()
    .map((k, v) -> KeyValue.pair(k.key(),
        new SalesAggregate(k.key(), k.window().startTime(), v)))
    .to("sales.analytics.hourly-aggregates");
```

---

## 🛡️ Error Handling & DLQ Pattern

```java
// Producer dengan retry & DLQ
@Bean
public KafkaTemplate<String, Object> kafkaTemplate() {
    Map<String, Object> props = new HashMap<>();
    props.put(ProducerConfig.BOOTSTRAP_SERVERS_CONFIG, kafkaBrokers);
    props.put(ProducerConfig.RETRIES_CONFIG, 3);
    props.put(ProducerConfig.RETRY_BACKOFF_MS_CONFIG, 1000);
    props.put(ProducerConfig.ACKS_CONFIG, "all");
    props.put(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, true);
    return new KafkaTemplate<>(new DefaultKafkaProducerFactory<>(props));
}

// Consumer dengan DLQ
@KafkaListener(topics = "sales.order.created")
public void processOrder(SalesOrderEvent event, Acknowledgment ack) {
    try {
        orderService.process(event);
        ack.acknowledge();
    } catch (RetryableException e) {
        // Akan di-retry oleh Spring Retry
        throw e;
    } catch (Exception e) {
        // Non-retryable: kirim ke DLQ
        kafkaTemplate.send("sales.order.created.DLQ",
            event.getEventId(), event);
        log.error("Sent to DLQ: {}", event.getEventId(), e);
        ack.acknowledge();
    }
}
```

---

## 📊 Kafka Monitoring (JMX Metrics)

| Metric | Alert Threshold |
|--------|----------------|
| Consumer lag (business topics) | > 10.000 messages |
| Consumer lag (IoT topics) | > 100.000 messages |
| Broker disk usage | > 80% |
| Under-replicated partitions | > 0 |
| Producer error rate | > 1% |
| Kafka Connect task status | != RUNNING |
| Schema Registry errors | > 0/menit |
