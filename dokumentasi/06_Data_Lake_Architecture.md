# 06 — Data Lake Architecture
## Enterprise Digital Platform (EDP)

---

## Overview

Data lake EDP adalah **medallion tiga lapis (Bronze → Silver → Gold)** di atas MinIO, seluruhnya ditulis dalam Go di dw-service. Bronze adalah raw dump dari setiap batch sync dan streaming event; Silver adalah keadaan terkini tiap baris tanpa duplikat; Gold adalah agregat bulanan yang dihitung dari Silver. Tidak ada Delta Lake dan tidak ada Apache Spark/dbt.

---

## Implementasi Aktual

**Object Storage**: MinIO (`infra/docker-compose.yml`, port host 9004, container port 9000)  
**Bucket**: `dw-lake`  
**Format**: JSON Lines (`.jsonl`) — satu baris JSON per record  
**Layer**:

| Layer | Lokasi | Isi |
|-------|--------|-----|
| Bronze | `<fact>/YYYY/MM/DD/<unix-nano>.jsonl` | Raw, append-only; baris yang berubah muncul berkali-kali |
| Silver | `silver/<fact>/current.jsonl` (+ `rejected.jsonl` bila ada) | Versi terbaru per `(CompanyID, id)`, terurut, deterministik |
| Gold | `gold/finance_monthly.jsonl`, `gold/sales_monthly.jsonl` | Revenue/expense dan nilai penjualan per company per bulan |

**Silver** memakai aturan yang sama dengan `ReplacingMergeTree(synced_at)` di ClickHouse: kunci `(CompanyID, id)`, `synced_at` terbesar menang. Baris tanpa id/company, dengan UUID kosong, atau bukan JSON tidak dibuang diam-diam — dihitung dan disimpan di `rejected.jsonl`. Selalu berlaku `bronze_rows = silver_rows + duplicates_dropped + rejected`. Build-nya full rebuild dan idempotent (dua build berturut-turut menghasilkan berkas identik).

**Gold** memakai aturan bisnis yang identik dengan `MonthlyFinanceSummary` (hanya jurnal POSTED; revenue = kredit akun REVENUE; expense = debit akun EXPENSE) dan `MonthlySalesSummary` (DRAFT dan CANCELLED dikecualikan). `TestGold_AgreesWithClickHouse` membandingkan keduanya angka demi angka terhadap ClickHouse dan MinIO sungguhan, dengan data yang sengaja memuat baris yang berubah status.

**Endpoint** (lewat gateway): `POST /api/dw/lake/build` (Create pada menu Sync Status) membangun ulang seluruh Silver lalu Gold; `GET /api/dw/lake/gold/{finance-monthly|sales-monthly}?company_id=` membaca Gold. Selain lewat endpoint, dw-service menjalankan build + rekonsiliasi **otomatis** tiap `DW_LAKE_BUILD_INTERVAL_SECONDS` (default 3600; 0 = mati; di Kubernetes dan kedua env example). Interval-nya terpisah dari sync 5 menit karena build membaca seluruh Bronze, dan putaran pertama menunggu satu interval penuh setelah start. Log hanya mencatat **perubahan** status per fact (selisih baru, pulih, atau tidak bisa diperiksa) plus ringkasan "N/17 facts match"; keadaan tetap seperti `EXTRA_IN_LAKE` permanen tidak memenuhi log tiap jam, sedangkan galat build dicatat setiap putaran. Backfill sengaja TIDAK otomatis: log hanya menunjuk ke `POST /api/dw/lake/backfill`.

**Backfill dan rekonsiliasi.** ETL biasa hanya membaca baris di atas watermark, jadi baris yang sudah tersalin ke ClickHouse *sebelum* lake dipasang (atau saat penulisan ke lake gagal) tidak pernah masuk Bronze — dan Gold yang dibangun di atasnya kekurangan angka. (Watermark disimpan sebagai `DateTime` detik penuh dan ekstraknya `>=`, jadi baris terbaru dibaca ulang tiap sync; itu sebabnya Bronze penuh duplikat, dan celahnya hanya menimpa baris yang sudah *di bawah* watermark.) Dua endpoint menutupnya:

- `POST /api/dw/lake/backfill` (Create pada menu Sync Status) menulis SELURUH isi tiap tabel sumber ke Bronze dengan SQL ekstrak yang sama persis dengan sync biasa, tanpa menyentuh ClickHouse maupun watermark. Tumpang tindih dengan Bronze lama aman: di Silver versi terbaru menang. Kegagalan menulis ke lake di sini adalah galat, bukan sekadar log.
- `GET /api/dw/lake/reconcile` (View) membandingkan jumlah baris Silver dengan `count(*) ... FINAL` ClickHouse per fact: `MATCH`, `MISSING_FROM_LAKE` (jalankan backfill), atau `EXTRA_IN_LAKE` (selidiki).

Urutan setelah menambah/memasang lake: `backfill` → `build` → `reconcile`. Batas pemeriksaan ini: ia membandingkan **hitungan**, bukan isi — selisih yang saling meniadakan lolos sebagai MATCH — dan Silver yang usang terbaca sebagai selisih, jadi `build` dulu. `EXTRA_IN_LAKE` yang wajar: Bronze append-only tidak tahu baris sumber yang dihapus (di dev: 2 baris `hr_kpi_reviews` yang sudah dihapus dari Postgres dan ClickHouse masih ada di lake).

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

## Kenapa Silver/Gold di Go, bukan Spark atau dbt

- Skalanya belum membenarkan Spark: satu fact dibaca penuh ke memori saat build. Kalau Bronze tumbuh sampai itu jadi masalah, itulah saatnya berpindah ke Spark/dbt — Silver dan Gold sudah punya kontrak (kunci, invarian, angka pembanding) yang bisa dipindahkan.
- Tanpa dependency baru: tidak ada JVM/Python di image maupun CI.
- ClickHouse tetap menjadi tempat query analitik interaktif; Gold di MinIO adalah salinan agregat yang bisa dikonsumsi alat lain tanpa menyentuh ClickHouse.

**Batas yang perlu diketahui**: Gold baru dua dataset (yang punya padanan ClickHouse untuk dibandingkan). Dataset lain ditambahkan dengan pola yang sama, bersama padanannya. Kalau Silver satu fact gagal dibangun, Gold tetap dihitung dari Silver lama — baca `errors` di respons build sebelum mempercayai angka Gold.
