# 05 — Data Warehouse Architecture
## Enterprise Digital Platform (EDP)

---

## Overview

`dw-service` (port 8095) adalah modul dedicated untuk analytical data pipeline. Service ini membaca langsung dari database Postgres 9 service lain (bukan lewat HTTP API mereka) dan menulis ke ClickHouse sebagai destinasi OLAP, dengan MinIO sebagai raw data lake (bronze layer).

**Tidak menggunakan**: dbt, Airbyte, Debezium, Airflow, star schema dengan dimension tables terpisah.

---

## Arsitektur Dua-Lapis ETL

```
┌──────────────────────────────────────────────────────────────────┐
│                      SOURCE SYSTEMS (Postgres)                   │
│  finance_service  hr_service  sales_service  warehouse_service   │
│  purchasing_service  production_service  qc_service  asset_service│
│  iot_service                                                      │
└──────────┬──────────────────────────────────────────────────────┘
           │
    ┌──────┴──────────────────┐
    │                         │
    ▼                         ▼
┌─────────────────┐    ┌─────────────────────────────────┐
│  BATCH ETL      │    │  KAFKA STREAMING ETL            │
│  (ticker 5 min) │    │  (near-realtime, <100ms)        │
│                 │    │                                  │
│  9 SyncX() fns  │    │  12 Kafka topics                │
│  watermark-based│    │  → single-row Postgres re-query  │
│  incremental    │    │  → insert ClickHouse             │
└────────┬────────┘    └────────────────┬────────────────┘
         │                              │
         └──────────────┬───────────────┘
                        ▼
         ┌──────────────────────────────┐
         │     CLICKHOUSE (dw database) │
         │  9 fact tables               │
         │  ReplacingMergeTree engine   │
         │  (upsert via synced_at)      │
         └──────────────────────────────┘
                        │ dual-write (best-effort)
                        ▼
         ┌──────────────────────────────┐
         │     MINIO (dw-lake bucket)  │
         │  JSON Lines, bronze only     │
         │  path: <fact>/<YYYY>/<MM>/   │
         │        <DD>/<ts>.jsonl       │
         └──────────────────────────────┘
```

---

## 9 Fact Tables di ClickHouse

| Fact Table | Source | Watermark Column | Engine |
|------------|--------|-----------------|--------|
| `fact_finance_journal_lines` | finance_service | COALESCE(posted_at, created_at) | ReplacingMergeTree(synced_at) |
| `fact_sales_order_lines` | sales_service | sales_orders.updated_at | ReplacingMergeTree(synced_at) |
| `fact_inventory_movements` | warehouse_service | stock_movements.created_at | ReplacingMergeTree(synced_at) |
| `fact_hr_payroll_details` | hr_service | COALESCE(posted_at, created_at) | ReplacingMergeTree(synced_at) |
| `fact_purchasing_order_lines` | purchasing_service | purchase_orders.updated_at | ReplacingMergeTree(synced_at) |
| `fact_production_work_orders` | production_service | work_orders.updated_at | ReplacingMergeTree(synced_at) |
| `fact_qc_inspections` | qc_service | quality_inspections.updated_at | ReplacingMergeTree(synced_at) |
| `fact_asset_maintenance` | asset_service | maintenance_schedules.updated_at | ReplacingMergeTree(synced_at) |
| `fact_iot_readings` | iot_service | readings.created_at | ReplacingMergeTree(synced_at) |

Semua tabel **denormalized** — JOIN dimensi (customer_name, account_code, dll) dilakukan saat extract dari Postgres, bukan saat query di ClickHouse. Ini sesuai best practice ClickHouse sebagai kolom-store OLAP.

---

## Batch ETL — Detail Implementasi

**Package**: `internal/etl/` (9 file: finance.go, sales.go, inventory.go, hr.go, purchasing.go, production.go, qc.go, asset.go, iot.go)

**Alur per SyncX:**
1. Baca watermark terakhir dari `etl_sync_state` di ClickHouse
2. Query Postgres dengan `WHERE <watermark_col> >= $1` (inklusif)
3. Scan baris ke struct Go
4. Batch insert ke ClickHouse via `PrepareBatch` + `Append` + `Send`
5. Best-effort write ke MinIO (JSON Lines)
6. Update watermark ke MAX timestamp batch ini

**Trigger**: Ticker tiap 300 detik (default) + `POST /sync` untuk manual trigger.

---

## Kafka Streaming ETL — Detail Implementasi

**Package**: `internal/streaming/` (consumer.go, handlers.go, streaming.go)

**12 Topic yang Dikonsumsi** (consumer group: `dw-service-streaming`):

| Kafka Topic | Entity ID | Target Fact |
|-------------|-----------|-------------|
| `finance.journal.posted` | journal_entry_id | fact_finance_journal_lines |
| `sales.order.fulfilled` | sales_order_id | fact_sales_order_lines |
| `sales.order.invoiced` | sales_order_id | fact_sales_order_lines |
| `warehouse.stock.moved` | movement_id | fact_inventory_movements |
| `warehouse.stock.batch_moved` | reference_id | fact_inventory_movements |
| `hr.payroll.posted` | payroll_run_id | fact_hr_payroll_details |
| `purchasing.order.received` | purchase_order_id | fact_purchasing_order_lines |
| `purchasing.order.invoiced` | purchase_order_id | fact_purchasing_order_lines |
| `production.work_order.completed` | work_order_id | fact_production_work_orders |
| `qc.inspection.created` | inspection_id | fact_qc_inspections |
| `asset.maintenance.completed` | schedule_id | fact_asset_maintenance |
| `asset.maintenance.cancelled` | schedule_id | fact_asset_maintenance |

IoT readings **tidak ada di Kafka** (telemetri frekuensi tinggi → Postgres langsung via MQTT). Batch ETL menangani IoT.

**Alur per event:**
1. Terima Kafka event JSON, parse `entity_id`
2. Single-row JOIN query ke Postgres (`WHERE id = $1`)
3. Insert ke ClickHouse + best-effort write ke MinIO

---

## ReplacingMergeTree & Deduplication

Kedua path (batch + streaming) bisa menulis baris yang sama ke ClickHouse — ini bukan bug. `ReplacingMergeTree(synced_at)` men-dedup berdasarkan ORDER BY key (`company_id, {entity_id}`), mempertahankan baris dengan `synced_at` terbesar. Dedup terjadi saat background merge atau via `SELECT ... FINAL`.

---

## HTTP Endpoints

| Endpoint | Fungsi |
|----------|--------|
| `GET /health` | Status service |
| `POST /sync` | Trigger manual batch ETL untuk semua 9 fact |
| `GET /sync/status` | Row count per fact + last watermark |
