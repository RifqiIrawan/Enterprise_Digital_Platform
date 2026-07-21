# 06 — Data Lake Architecture
## Enterprise Digital Platform (EDP)

---

## Overview

Data lake EDP adalah **bronze layer sederhana** di atas MinIO — raw dump dari setiap batch sync dan streaming event sebelum data masuk ke ClickHouse (curated). Tidak ada Silver/Gold layer, tidak ada Delta Lake, tidak ada Apache Spark.

---

## Implementasi Aktual

**Object Storage**: MinIO (`infra/docker-compose.yml`, port host 9004, container port 9000)  
**Bucket**: `dw-lake`  
**Format**: JSON Lines (`.jsonl`) — satu baris JSON per record  
**Layer**: Bronze only (raw, no transformation)

---

## Struktur Path

```
dw-lake/
└── {fact_name}/
    └── {YYYY}/
        └── {MM}/
            └── {DD}/
                └── {synced_at_unix_nano}.jsonl
```

Contoh:
```
dw-lake/finance_journal_lines/2026/07/21/1753084800000000000.jsonl
dw-lake/sales_order_lines/2026/07/21/1753084800000000001.jsonl
```

---

## Kapan Data Ditulis ke Lake

**Dari batch ETL**: setelah `InsertXxx` ke ClickHouse sukses, `WriteJSONLines` dipanggil untuk semua baris batch.  
**Dari streaming ETL**: setelah insert ClickHouse per-event sukses, `WriteJSONLines` untuk baris entity itu.

**Penting**: lake write adalah **best-effort** — kegagalan MinIO tidak menggagalkan insert ClickHouse yang sudah berhasil. `lake *datalake.Client` boleh `nil` di seluruh codebase (nil = no-op).

---

## Format JSON Lines

Field name menggunakan nama field Go (CamelCase, bukan snake_case):
```json
{"LineID":"uuid","JournalID":"uuid","CompanyID":"uuid","BranchID":null,"EntryNumber":"JE-001","EntryDate":"2026-07-21T00:00:00Z","DebitAmount":1000000,"CreditAmount":0,"PostedAt":"2026-07-21T10:00:00Z"}
{"LineID":"uuid","JournalID":"uuid","CompanyID":"uuid","BranchID":null,"EntryNumber":"JE-001","EntryDate":"2026-07-21T00:00:00Z","DebitAmount":0,"CreditAmount":1000000,"PostedAt":"2026-07-21T10:00:00Z"}
```

CamelCase dipilih secara sadar — bronze layer untuk durability/reprocessability, bukan konsumsi langsung. Tidak worth effort nambah `json:""` tags ke ~120 field di 9 row struct.

---

## Package `internal/datalake`

```go
// Connect membuka koneksi MinIO + EnsureBucket (create-if-not-exists)
func Connect(ctx, endpoint, accessKey, secretKey, bucket, useSSL) (*Client, error)

// WriteJSONLines menerima slice bertipe apa pun (pakai reflection sekali di sini)
// dan menulis ke MinIO sebagai JSON Lines file.
// Nil client = no-op, tidak panic.
func (c *Client) WriteJSONLines(ctx, fact string, rows any, syncedAt time.Time) error

// ListKeys, Get — untuk verifikasi di test
```

Satu-satunya pemakaian reflection di seluruh codebase — trade-off yang disengaja daripada 9 method identik per domain.

---

## Environment Variables (dw-service)

| Var | Default | Keterangan |
|-----|---------|------------|
| `MINIO_ENDPOINT` | `localhost:9004` | Host:port MinIO API |
| `MINIO_ACCESS_KEY` | `minioadmin` | Dev-only |
| `MINIO_SECRET_KEY` | `minioadmin` | Dev-only |
| `MINIO_BUCKET` | `dw-lake` | Nama bucket |
| `MINIO_USE_SSL` | `false` | TLS untuk prod |

---

## Kenapa Hanya Bronze Layer

- Scope awal: durability dan audit trail data mentah sebelum ClickHouse processing
- Silver (cleansed) dan Gold (aggregated) bisa ditambahkan nanti kalau ada kebutuhan konkret
- ClickHouse sudah berperan sebagai "gold layer" de facto untuk query analitik
- Menambah Spark/Delta hanya untuk Silver→Gold transformation di skala ini tidak justified
