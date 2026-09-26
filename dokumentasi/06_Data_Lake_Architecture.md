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

Urutan setelah menambah/memasang lake: `backfill` → `build` → `reconcile`. Batas pemeriksaan ini: ia membandingkan **hitungan**, bukan isi — selisih yang saling meniadakan lolos sebagai MATCH — dan Silver yang usang terbaca sebagai selisih, jadi `build` dulu.

**Penghapusan di sumber.** Bronze append-only tidak pernah tahu bahwa sebuah baris dihapus, jadi tanpa penanganan Silver menyimpan baris hantu selamanya (di dev: 2 baris `hr_kpi_reviews` yang sudah tidak ada di Postgres). Tiap build Silver sekarang menanyakan ke sumber baris mana yang MASIH ADA — lewat ekstrak yang sama persis dengan sync, jadi "masih ada" berarti "masih akan disalin sync" (mis. `production_oee` hanya run CLOSED) — dan memangkas dari Silver kunci `(CompanyID, id)` yang sudah tidak ada. `SilverStats.pruned` melaporkan jumlahnya, dan invarian menjadi `bronze = silver + duplikat + rejected + pruned`. Bronze tetap menyimpan baris itu sebagai riwayat; membangun Silver tanpa pemangkasan menghidupkannya lagi.

Aturannya konservatif: Bronze dibaca DULU, kunci hidup diambil SESUDAHNYA (kebalikannya bisa memangkas baris yang baru dibuat di antara keduanya), dan kalau sumber tidak bisa ditanya build fact itu GAGAL dan Silver lama dibiarkan — lebih baik usang daripada menerbitkan baris hantu atau menghapus baris yang masih ada. Aturan kunci (`rowKey`) hanya satu dan dipakai Silver maupun daftar kunci hidup. Biayanya: tiap build mengekstrak seluruh tabel sumber (sama beratnya dengan backfill), jadi interval build 1 jam, bukan 5 menit.

Catatan: ClickHouse sendiri juga upsert-only dan tidak menghapus baris yang dihapus di sumber, jadi `MISSING_FROM_LAKE` bisa berarti backfill dibutuhkan **atau** ClickHouse masih memegang baris yang sudah dihapus. Menangani penghapusan di ClickHouse di luar cakupan ini.

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

## Batas Skala (hasil uji, 2026-09-27)

Diukur pada satu fact (`sales_order_lines`, baris ±350 byte, 5 baris per order) dengan instance dw-service terpisah, database Postgres sumber sementara, database ClickHouse dan bucket MinIO per tahap; data dev tidak disentuh dan semuanya dihapus setelahnya. Mesin: 15 GB RAM, 12 core, Postgres/ClickHouse/MinIO lokal. Angka "selisih" = waktu dikurangi baseline 0 baris (sync 4,7 dtk, build 3,2 dtk, backfill 3,0 dtk: biaya tetap 16 fact lain dan koneksi).

| Baris sales | Sync awal (selisih) | Puncak memori | Build Silver+Gold (selisih) | Puncak memori | Backfill (selisih) |
|---:|---:|---:|---:|---:|---:|
| 100 rb | 5,7 dtk (+1,0) | 227 MB | 7,5 dtk (+4,3) | 462 MB | 4,0 dtk (+1,0) |
| 250 rb | 8,3 dtk (+3,6) | 460 MB | 14,4 dtk (+11,2) | 941 MB | 5,9 dtk (+2,9) |
| 500 rb | 12,0 dtk (+7,3) | 1.025 MB | 25,7 dtk (+22,5) | 1.786 MB | 7,7 dtk (+4,7) |

Semuanya **linear**: sync ±15 µs dan ±2 KB per baris (±68 rb baris/dtk), build ±45 µs dan ±3,5 KB per baris, backfill ±9 µs dan ±3,3 KB per baris. Jumlah baris di ClickHouse cocok persis di tiap tahap. Build kedua (setelah Bronze berisi hasil sync DAN backfill, jadi dua kali lipat baris) lebih lama 28% pada 500 rb (32,9 dtk): biaya build mengikuti jumlah baris **Bronze**, bukan baris unik.

**Proyeksi linear ke target roadmap** (Sales 5 jt, Purchase 2 jt, Inventory 10 jt, HRIS 2 jt, Manufacturing 15 jt, IoT 50 jt = ±84 jt baris fact; log sistem 100 jt tidak masuk lake):

- Satu fact 5 jt baris: sync awal ±75 dtk dan ±10 GB; build ±225 dtk dan **±17,5 GB** — melebihi RAM mesin ini.
- Semua fact berurutan: build ±63 menit, **lebih lama dari interval 1 jam** ticker; fact terbesar (IoT 50 jt) butuh ±175 GB.
- Di mesin 15 GB dengan layanan lain berjalan, batas praktisnya sekitar **2–3 jt baris per fact**.

**Penyebab (dari kode, bukan dugaan):**

1. `BuildSilver` membaca tiap objek Bronze utuh ke memori lalu menyimpan salinan setiap baris unik di map `latest`.
2. Daftar kunci hidup (pemangkasan) meng-ekstrak SELURUH baris bertipe ke memori hanya untuk mengambil dua kolom kunci.
3. Sync dan backfill menaruh seluruh hasil ekstrak dalam satu slice, satu batch insert ClickHouse, dan satu objek JSONL — muatan awal tabel besar jadi satu objek raksasa.
4. Build membaca ulang SELURUH Bronze tiap putaran, dan Bronze terus bertambah (baris batas watermark ditulis ulang tiap sync, plus backfill).

**Perbaikan yang bisa dilakukan sebelum pindah ke Spark/dbt** (belum dikerjakan; urut menurut dampak): (a) daftar kunci hidup hanya menyimpan kunci sebagai pasangan UUID 16 byte, bukan baris utuh; (b) Silver inkremental — pertahankan Silver sebelumnya dan gabungkan hanya objek Bronze yang lebih baru dari penanda build terakhir, sehingga biaya mengikuti data baru, bukan seluruh riwayat; (c) sync/backfill berpotongan (mis. 100 rb baris per objek dan per batch insert) dengan memori terbatas; (d) pemadatan Bronze untuk objek yang sudah sepenuhnya tertimpa. Di atas puluhan juta baris per fact, Spark/dbt seperti yang diantisipasi di awal menjadi masuk akal, dan kontrak yang sudah ada (aturan kunci, invarian `bronze = silver + duplikat + rejected + pruned`, rekonsiliasi) bisa dipindahkan sebagai test.

**Yang TIDAK diukur:** fact lain (diasumsikan orde yang sama; baris IoT lebih sempit sehingga memori per baris lebih kecil), waktu `count(*) ... FINAL` untuk rekonsiliasi dan query analitik ClickHouse pada puluhan juta baris, biaya sort ekstrak Postgres tanpa indeks `updated_at`, dan ruang disk. Tahap tertinggi yang dijalankan adalah 500 rb baris — 1 jt akan butuh ±3,5 GB sementara hanya ±3,4 GB RAM yang bebas — sehingga angka di atasnya adalah ekstrapolasi.
