# 03 — Microservices Architecture
## Enterprise Data Center Simulator (EDCS)

---

## 📦 Service Catalog

| # | Service Name | Domain | Port | DB | Language |
|---|-------------|--------|------|----|----------|
| 1 | auth-service | Platform | 3001 | PostgreSQL | Node.js |
| 2 | erp-core-service | ERP | 3002 | PostgreSQL | Node.js |
| 3 | hris-service | HR | 3003 | PostgreSQL | Node.js |
| 4 | payroll-service | HR | 3004 | PostgreSQL | Python |
| 5 | crm-service | CRM | 3005 | PostgreSQL | Node.js |
| 6 | sales-service | Sales | 3006 | PostgreSQL | Node.js |
| 7 | wms-service | Warehouse | 3007 | PostgreSQL | Node.js |
| 8 | inventory-service | Warehouse | 3008 | PostgreSQL + Redis | Node.js |
| 9 | mes-service | Manufacturing | 3009 | PostgreSQL | Python |
| 10 | quality-service | Manufacturing | 3010 | PostgreSQL | Python |
| 11 | finance-service | Finance | 3011 | PostgreSQL | Node.js |
| 12 | accounting-service | Finance | 3012 | PostgreSQL | Node.js |
| 13 | procurement-service | Procurement | 3013 | PostgreSQL | Node.js |
| 14 | vendor-service | Procurement | 3014 | PostgreSQL | Node.js |
| 15 | asset-service | Asset | 3015 | PostgreSQL | Node.js |
| 16 | iot-gateway | IoT | 3016 | TimescaleDB | Python |
| 17 | device-simulator | IoT | 3017 | Redis | Python |
| 18 | ml-serving | AI/ML | 3018 | — | Python |
| 19 | ai-assistant | AI | 3019 | Qdrant | Python |
| 20 | notification-service | Platform | 3020 | Redis | Node.js |
| 21 | file-service | Platform | 3021 | MinIO | Node.js |
| 22 | report-service | Platform | 3022 | — | Python |
| 23 | audit-service | Platform | 3023 | PostgreSQL | Node.js |
| 24 | search-service | Platform | 3024 | Elasticsearch | Python |

---

## 🏗️ Service Template Standard

Setiap service mengikuti struktur berikut:

```
service-name/
├── src/
│   ├── api/              # Controllers / Route handlers
│   │   ├── v1/
│   │   └── health.ts
│   ├── domain/           # Business logic (pure, no infra dependency)
│   │   ├── entities/
│   │   ├── use-cases/
│   │   └── repositories/ # Interfaces
│   ├── infrastructure/   # Adapters (DB, Kafka, external APIs)
│   │   ├── database/
│   │   ├── messaging/
│   │   └── external/
│   ├── config/           # Env vars, constants
│   └── main.ts           # Entry point
├── test/
│   ├── unit/
│   ├── integration/
│   └── e2e/
├── migrations/           # DB migrations (Flyway/Knex)
├── Dockerfile
├── docker-compose.dev.yml
├── openapi.yaml          # API spec
└── package.json / pyproject.toml
```

---

## 🔗 Inter-Service Communication

### Synchronous (REST)
```yaml
# Digunakan untuk:
# - Query data real-time
# - User-facing requests
# - Validasi cross-domain

Pattern: HTTP/REST via API Gateway
Auth: JWT Bearer Token
Format: JSON
Timeout: 5s (default), 30s (long-running)
Retry: 3x dengan exponential backoff
Circuit Breaker: Threshold 50% error rate / 10s window
```

### Asynchronous (Kafka Events)
```yaml
# Digunakan untuk:
# - Domain events (state changes)
# - Notifikasi antar domain
# - Data sync ke Data Platform

Broker: Apache Kafka
Schema: Avro (Schema Registry)
Pattern: Publish-Subscribe
Partitioning: by entity_id
Retention: 7 hari
DLQ: setiap topic punya .DLQ counterpart
```

---

## 📨 Kafka Topic Registry

| Topic | Producer | Consumers | Payload |
|-------|----------|-----------|---------|
| `erp.master-data.updated` | erp-core | hris, crm, wms, finance | MasterDataEvent |
| `hris.employee.created` | hris | auth, payroll, crm | EmployeeCreatedEvent |
| `hris.payroll.processed` | payroll | finance, audit | PayrollProcessedEvent |
| `crm.lead.converted` | crm | sales, finance | LeadConvertedEvent |
| `sales.order.created` | sales | wms, finance, inventory | SalesOrderEvent |
| `wms.shipment.dispatched` | wms | crm, sales | ShipmentEvent |
| `wms.stock.below-threshold` | inventory | procurement, mes | StockAlertEvent |
| `mes.workorder.completed` | mes | wms, quality, finance | WorkOrderEvent |
| `mes.quality.failed` | quality | mes, crm, procurement | QualityFailedEvent |
| `finance.invoice.created` | finance | procurement, crm | InvoiceEvent |
| `finance.payment.processed` | finance | vendor, crm | PaymentEvent |
| `procurement.po.approved` | procurement | vendor, wms, finance | POApprovedEvent |
| `asset.maintenance.due` | asset | notification | MaintenanceDueEvent |
| `iot.sensor.reading` | iot-gateway | mes, asset, data-lake | SensorReadingEvent |
| `iot.device.alert` | iot-gateway | notification, mes | DeviceAlertEvent |

---

## 🛡️ Service Resilience Patterns

### Circuit Breaker (per service)
```typescript
// Konfigurasi circuit breaker
const cbConfig = {
  threshold: 0.5,        // 50% error rate membuka circuit
  windowSize: 10,        // Evaluasi per 10 detik
  timeout: 5000,         // Timeout per request (ms)
  resetTimeout: 30000,   // Tunggu 30s sebelum retry (half-open)
};
```

### Bulkhead Pattern
```yaml
# Isolasi thread pool per downstream service
bulkhead:
  hris → payroll: maxConcurrent: 10
  crm  → sales:   maxConcurrent: 20
  wms  → inventory: maxConcurrent: 30
```

### Retry dengan Backoff
```typescript
const retryConfig = {
  attempts: 3,
  delay: 1000,       // 1 detik awal
  factor: 2,         // Exponential: 1s, 2s, 4s
  maxDelay: 10000,   // Maksimum 10 detik
  retryOn: [503, 429, 408],
};
```

---

## 🔍 Service Discovery

```yaml
# Kubernetes DNS-based discovery
# Service A memanggil Service B via:
http://hris-service.edcs-business.svc.cluster.local:3003

# Di luar cluster (via API Gateway):
https://api.edcs.internal/hris/v1/employees

# Environment variables per service:
HRIS_SERVICE_URL=http://hris-service:3003
KAFKA_BROKERS=kafka-0.kafka:9092,kafka-1.kafka:9092
REDIS_URL=redis://redis-master:6379
DB_URL=postgresql://user:pass@postgres-hris:5432/hris_db
```

---

## 📊 Service Health & Metrics

### Health Check Endpoint (wajib semua service)
```json
GET /health

{
  "status": "healthy",
  "service": "hris-service",
  "version": "1.2.3",
  "timestamp": "2026-07-09T10:00:00Z",
  "checks": {
    "database": "healthy",
    "kafka": "healthy",
    "redis": "healthy",
    "memory_mb": 256,
    "uptime_seconds": 86400
  }
}
```

### Prometheus Metrics (wajib semua service)
```
# Metrics yang harus di-expose di /metrics:
http_requests_total{method, route, status}
http_request_duration_seconds{method, route}
kafka_messages_produced_total{topic}
kafka_messages_consumed_total{topic, consumer_group}
db_query_duration_seconds{query_type}
business_events_total{event_type}   # domain-specific
```

---

## 🚀 Deployment Spec (K8s)

```yaml
# Template Kubernetes Deployment (setiap service)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hris-service
  namespace: edcs-business
spec:
  replicas: 2
  selector:
    matchLabels:
      app: hris-service
  template:
    spec:
      containers:
      - name: hris-service
        image: edcs/hris-service:1.0.0
        ports:
        - containerPort: 3003
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 3003
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 3003
          initialDelaySeconds: 10
          periodSeconds: 5
        env:
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: hris-secrets
              key: database-url
```
