# 02 — Enterprise Architecture
## Enterprise Digital Platform (EDP)

---

## Arsitektur Overview

EDP menggunakan **microservices berbasis Go** dengan komunikasi HTTP sinkron (lewat API Gateway) dan event asinkron (lewat Kafka). Tidak ada service mesh, tidak ada schema registry, tidak ada API versioning — arsitektur sengaja dibuat sesederhana mungkin sambil tetap production-grade.

```
┌─────────────────────────────────────────────────────────┐
│                  BROWSER (React SPA)                    │
│           http://localhost:3000 (Vite dev)             │
└───────────────────────────┬─────────────────────────────┘
                            │ HTTP
┌───────────────────────────▼─────────────────────────────┐
│              API GATEWAY (Go, port 8079)                │
│  JWT validation │ Reverse proxy │ Route prefix mapping  │
└──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──────────┘
   │  │  │  │  │  │  │  │  │  │  │  │  │  │  │
  Auth Comp RBAC Aud Fin HR  Sal Pur War Pro QC  Ast AI  IoT DW
  8081 8082 8083 8084 8085 8086 8087 8088 8089 8090 8091 8092 8093 8094 8095

┌─────────────────────────────────────────────────────────┐
│              APACHE KAFKA (single broker)               │
│  65+ topics: auth.*, company.*, finance.*, sales.*      │
│  hr.*, purchasing.*, warehouse.*, production.*, qc.*    │
│  asset.*, iot.*                                         │
│  Consumer groups: audit-service, dw-service-streaming   │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│                POSTGRESQL 18 (native)                   │
│  13 databases (1 per service yang butuh persistence)   │
│  Role: platform / platform                              │
└─────────────────────────────────────────────────────────┘

┌──────────────────┐  ┌──────────────────┐  ┌────────────┐
│   CLICKHOUSE     │  │     MINIO        │  │   REDIS    │
│  (dw-service     │  │  (dw-service,    │  │ (api-gw,   │
│   OLAP facts)    │  │   data lake)     │  │  auth)     │
└──────────────────┘  └──────────────────┘  └────────────┘

┌──────────────────────────────────────────────────────────┐
│               MOSQUITTO (MQTT broker)                    │
│  iot-service simulator → publish → subscribe → ingest   │
└──────────────────────────────────────────────────────────┘
```

---

## Pola Komunikasi

### HTTP Sinkron (service-to-service)
Dipakai untuk operasi yang butuh hasil langsung:
- `hr-service` → `finance-service`: posting payroll ke GL
- `sales-service` → `finance-service`: posting invoice AR
- `sales-service` → `warehouse-service`: batch stock movement
- `purchasing-service` → `finance-service`: posting invoice AP
- `purchasing-service` → `warehouse-service`: stock in dari PO
- `production-service` → `warehouse-service`: konsumsi komponen + stock hasil produksi
- `ai-bi-service` → semua 8 service: agregasi data untuk dashboard

Semua panggilan service-to-service langsung (tidak lewat api-gateway), menggunakan env var `*_SERVICE_URL`.

### Event Asinkron (Kafka)
Semua service bisnis publish audit event ke Kafka setelah operasi berhasil. Format: `{domain}.{entity}.{action}` (contoh: `sales.order.fulfilled`).

**Consumers:**
- `audit-service` — subscribe semua 65+ topic, simpan ke `audit_logs` (Postgres)
- `dw-service` streaming consumer — subscribe 12 topic, trigger single-row re-query ke Postgres lalu insert ke ClickHouse

### MQTT (IoT only)
`iot-service` simulator publish readings ke Mosquitto → service `ingest` handler subscribe dan simpan ke Postgres + publish 5 Kafka topics (`iot.*`).

---

## Keputusan Arsitektur Kunci

| Keputusan | Pilihan | Alasan |
|-----------|---------|--------|
| Bahasa backend | Go (semua service) | Simplicity, performance, single binary |
| HTTP routing | `net/http` stdlib (Go 1.22+) | Pattern routing built-in, tanpa framework |
| DB driver | `pgx/v5` | Performa, type safety |
| Kafka client | `segmentio/kafka-go` | Ringan, no Zookeeper dependency |
| Auth | JWT custom (bukan Keycloak) | Tidak overengineering untuk skala ini |
| API Gateway | Custom Go (bukan Kong) | Kontrol penuh, tidak ada black box |
| K8s config | Kustomize (bukan Helm) | Cukup untuk project ini, YAML transparan |
| BI/Analytics | Go service aggregation + linear regression | Tidak butuh Python/MLflow untuk use case ini |
