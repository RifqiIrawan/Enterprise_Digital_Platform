# 08 — ETL/ELT Pipeline
## Enterprise Digital Platform (EDP)

---

## Overview

EDP memiliki **dua pipeline ETL yang berjalan paralel** di `dw-service`, keduanya membaca dari Postgres dan menulis ke ClickHouse (+ MinIO lake best-effort):

1. **Batch ETL** — polling berkala (default 5 menit), watermark-based incremental
2. **Kafka Streaming ETL** — event-triggered, near-realtime (<100ms setelah event)

Tidak ada Airbyte, Debezium, dbt, atau Airflow.

---

## Batch ETL

### Implementasi
**Package**: `internal/etl/`  
**9 fungsi**: `SyncFinance`, `SyncSales`, `SyncInventory`, `SyncHR`, `SyncPurchasing`, `SyncProduction`, `SyncQC`, `SyncAsset`, `SyncIoT`

**Alur setiap SyncX:**
```
1. GetWatermark(ctx, source_table) dari ClickHouse etl_sync_state
   └── Kalau belum pernah sync: watermark = zero time → ambil semua data

2. Query Postgres dengan JOIN (denormalized):
   WHERE <watermark_column> >= $1   (inklusif, bukan >)
   ORDER BY <watermark_column>

3. Scan rows ke []ch.{Domain}Row

4. InsertXxx(ctx, rows, syncedAt) ke ClickHouse (PrepareBatch + Append + Send)

5. WriteJSONLines(ctx, fact, rows, syncedAt) ke MinIO (best-effort, error = log only)

6. SetWatermark(ctx, source_table, maxWatermark) ke ClickHouse
```

**Kenapa watermark inklusif (`>=` bukan `>`)**: Mencegah off-by-one edge case di batch boundary. Baris batas bisa ter-extract ulang di run berikutnya — ReplacingMergeTree men-dedup otomatis.

### Trigger
- **Ticker background**: tiap `DW_SYNC_INTERVAL_SECONDS` (default 300s)
- **Manual**: `POST /sync` ke dw-service HTTP API
- **Frontend**: tombol "Sync Now" di halaman DW Sync Status

### Watermark per Fact

| Fact | Kolom Watermark | Alasan |
|------|-----------------|--------|
| finance_journal_lines | `COALESCE(je.posted_at, je.created_at)` | journal_entries tidak punya updated_at |
| sales_order_lines | `sales_orders.updated_at` | status SO berubah → updated_at berubah → extract ulang semua lines |
| inventory_movements | `stock_movements.created_at` | append-only, tidak pernah di-UPDATE |
| hr_payroll_details | `COALESCE(pr.posted_at, pd.created_at)` | payroll tidak punya updated_at |
| purchasing_order_lines | `purchase_orders.updated_at` | sama dengan sales |
| production_work_orders | `work_orders.updated_at` | status WO berubah saat completed |
| qc_inspections | `quality_inspections.updated_at` | ada updated_at |
| asset_maintenance | `maintenance_schedules.updated_at` | status berubah saat complete/cancel |
| iot_readings | `readings.created_at` | append-only telemetry |

---

## Kafka Streaming ETL

### Implementasi
**Package**: `internal/streaming/`  
**Consumer group**: `dw-service-streaming` (terpisah dari `audit-service`)

### Alur per Event

```
Kafka event diterima (JSON)
    │
    ▼
parse entity_id dari event envelope
    │
    ▼
Single-row JOIN query ke Postgres (WHERE id = $1)
  └── Sama dengan query batch ETL tapi WHERE entity_id = $1
      bukan WHERE watermark >= $1
    │
    ▼
Insert ke ClickHouse (InsertXxx yang sama dengan batch ETL)
    │
    ▼
Best-effort write ke MinIO
```

### 12 Topic → 8 Handler

```
finance.journal.posted         → handleFinanceJournalPosted (query semua lines je)
sales.order.fulfilled          → handleSalesOrderEvent (query semua lines SO)
sales.order.invoiced           ↗
warehouse.stock.moved          → handleStockMoved (query 1 movement by id)
warehouse.stock.batch_moved    → handleStockBatchMoved (query by reference_id)
hr.payroll.posted              → handleHRPayrollPosted (query semua details)
purchasing.order.received      → handlePurchasingOrderEvent (query semua lines PO)
purchasing.order.invoiced      ↗
production.work_order.completed → handleProductionWOCompleted (query 1 WO)
qc.inspection.created          → handleQCInspectionCreated (query 1 inspeksi)
asset.maintenance.completed    → handleAssetMaintenanceEvent (query 1 schedule)
asset.maintenance.cancelled    ↗
```

IoT readings: tidak ada Kafka topic untuk readings (high-frequency telemetry) — batch ETL handles it.

### Consumer Resilience (Recreate Reader Pattern)

Setiap goroutine consumer menggunakan pattern **recreate Reader on error** (bukan retry ReadMessage pada reader yang sama):

```go
for {
    reader := kafka.NewReader(...)  // fresh reader setiap iterasi
    drainReader(ctx, reader, topic, handler)
    reader.Close()

    // Exponential backoff: 3s → 6s → 12s → ... → 30s max
    // Reset ke 3s kalau reader berhasil menerima pesan (error transient)
    time.Sleep(delay)
}
```

Ini memastikan audit-service dan dw-service streaming consumer tidak stuck kalau topic belum ada saat service pertama kali start.

---

## Koeksistensi Batch + Streaming

Kedua path menulis ke fact table ClickHouse yang sama. Ini bukan bug:
- `ReplacingMergeTree(synced_at)` men-dedup berdasarkan ORDER BY key (`company_id, entity_id`)
- Baris dengan `synced_at` lebih baru menggantikan yang lama saat background merge
- Query dengan `SELECT ... FINAL` selalu dapat data terbaru

Manfaat:
- **Batch ETL**: backfill data lama, recovery kalau streaming lag, IoT readings
- **Streaming ETL**: latency <100ms untuk event bisnis kritis (sales, finance, purchasing)
