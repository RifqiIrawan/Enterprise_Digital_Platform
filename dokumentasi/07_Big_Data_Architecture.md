# 07 — Big Data Architecture
## Enterprise Digital Platform (EDP)

---

## Overview

"Big Data" di EDP berarti **ClickHouse sebagai OLAP engine** untuk analytical queries di atas 9 fact tables. Tidak ada Apache Spark, Airflow, Trino, atau Flink — semua data processing dilakukan oleh dw-service (Go) dan query langsung ke ClickHouse.

---

## Stack Analitik Aktual

```
┌──────────────────────────────────────────────────────────────────┐
│                    ANALYTICAL QUERIES                            │
│  Frontend (React) → api-gateway → ai-bi-service → ClickHouse    │
│  atau langsung dw-service → ClickHouse                           │
└──────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────┐
│                 CLICKHOUSE 24.3 (OLAP Engine)                   │
│                                                                  │
│  Database: dw                                                    │
│  Port native: 9000 (container), 9101 (host remap)              │
│  Port HTTP: 8123                                                 │
│                                                                  │
│  9 Fact Tables (ReplacingMergeTree)                             │
│  + etl_sync_state (watermark tracking)                          │
│                                                                  │
│  Auth: CLICKHOUSE_USER=default, CLICKHOUSE_PASSWORD=clickhouse   │
│  (image resmi mematikan akses network kalau tidak diset)        │
└──────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────┐
│                  DATA INGESTION (Go)                             │
│                                                                  │
│  Batch ETL (5 menit)    Kafka Streaming (<100ms)                │
│  └── 9 SyncX() via      └── 12 topic consumers                  │
│      direct Postgres         └── single-row Postgres query       │
│      read + batch insert          └── insert ClickHouse          │
└──────────────────────────────────────────────────────────────────┘
```

---

## ClickHouse — Pilihan Desain

### Kenapa ReplacingMergeTree (bukan append-only)
Batch ETL dan streaming ETL bisa menulis baris yang sama. ReplacingMergeTree dengan `version = synced_at` men-dedup berdasarkan ORDER BY key, mempertahankan baris paling baru. Tidak perlu truncate-reload atau idempotency manual.

### Kenapa Denormalized (bukan star schema)
ClickHouse kolom-store paling optimal untuk tabel lebar yang sudah di-JOIN saat load, bukan JOIN saat query. Tiap fact table sudah berisi semua field dimensi yang dibutuhkan (customer_name, account_code, dll) — hasil JOIN dari Postgres saat extract.

### Watermark Incremental
```sql
-- etl_sync_state melacak last_synced_at per fact table
CREATE TABLE etl_sync_state (
    source_table String,
    last_synced_at DateTime
) ENGINE = ReplacingMergeTree(last_synced_at) ORDER BY source_table;

-- Query watermark dengan FINAL untuk dapat nilai terbaru
SELECT last_synced_at FROM etl_sync_state FINAL WHERE source_table = 'finance_journal_lines'
```

### Tipe Data ClickHouse vs Go
- `Decimal(18,2)` di ClickHouse → `decimal.Decimal` (`github.com/shopspring/decimal`) di Go — **bukan** `float64`
- `Nullable(Decimal(15,4))` → `*decimal.Decimal`
- `Nullable(UUID)` → `*uuid.UUID`

---

## Query Pattern dari ai-bi-service

ai-bi-service (port 8093) tidak query ClickHouse langsung — dia agregasi data via HTTP ke service-service bisnis (Postgres) dan melakukan kalkulasi di Go. Untuk query analitik ClickHouse, gunakan dw-service `GET /sync/status` atau query langsung ke ClickHouse:

```sql
-- Revenue total per bulan (dari fact_sales_order_lines)
SELECT
    toYYYYMM(order_date) as period,
    sum(amount) as total_revenue,
    count(distinct sales_order_id) as order_count
FROM fact_sales_order_lines FINAL
WHERE company_id = 'uuid-here'
GROUP BY period
ORDER BY period;

-- Stock movement summary per warehouse
SELECT
    warehouse_name,
    movement_type,
    sum(quantity) as total_qty,
    count(*) as movement_count
FROM fact_inventory_movements FINAL
WHERE company_id = 'uuid-here'
GROUP BY warehouse_name, movement_type;
```

---

## Partisi

Semua fact table dipartisi per bulan:
```sql
PARTITION BY toYYYYMM(entry_date)  -- atau order_date, movement_date, dll
```

Query dengan `WHERE entry_date BETWEEN ... AND ...` otomatis melakukan partition pruning di ClickHouse.

---

## Keterbatasan Saat Ini

- Tidak ada materialized view untuk pre-aggregation (bisa ditambahkan nanti)
- Tidak ada compression policy kustom (pakai default ClickHouse LZ4)
- Query Kafka Streams/Flink tidak diimplementasikan — semua via batch atau event-triggered re-query
- Tidak ada Trino/federated query — ClickHouse dan Postgres di-query terpisah
