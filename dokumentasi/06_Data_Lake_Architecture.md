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

**Build Silver inkremental.** Silver disimpan terurut menurut kunci `(CompanyID, id)`, ditemani `silver/<fact>/_state.json` berisi penanda objek Bronze terakhir yang sudah "mengendap" (umur > 10 menit, `settleLag`). Build berikutnya hanya membaca objek Bronze yang lebih baru dari penanda dan menggabungkannya ke Silver lama sebagai **aliran** (merge dua daftar terurut, baris demi baris; versi delta menang pada kunci yang sama), dengan pemangkasan baris yang dihapus di sumber dilakukan saat baris lewat. Silver lama tidak pernah dimuat utuh ke memori; Gold dan rekonsiliasi juga membaca Silver secara streaming (`EachSilver`/`CountSilver`). Objek yang lebih muda dari `settleLag` dibaca ulang tiap build (idempoten), karena batch ETL dan streaming ETL bisa menulis objek dengan key yang lebih lama sesudah build membaca daftar objek. Build **penuh** dari seluruh Bronze dijalankan kalau tidak ada state yang valid atau Silver lama hilang, tiap 24 jam (`fullRebuildEvery`), dengan `?full=true`, dan otomatis sekali kalau Silver lama ternyata rusak atau tidak terurut. `SilverStats` menambah `mode` (`full`/`incremental`) dan `previous_silver_rows`; invarian menjadi `previous + bronze = silver + duplikat + rejected + pruned`, dan `rejected.jsonl` bertambah tanpa duplikat. Ticker memakai build inkremental.

**Endpoint** (lewat gateway): `POST /api/dw/lake/build` (Create pada menu Sync Status) membangun Silver (inkremental) lalu Gold, dan `?full=true` memaksa Silver dibangun penuh dari seluruh Bronze; `GET /api/dw/lake/gold/{finance-monthly|sales-monthly}?company_id=` membaca Gold. Selain lewat endpoint, dw-service menjalankan build + rekonsiliasi **otomatis** tiap `DW_LAKE_BUILD_INTERVAL_SECONDS` (default 3600; 0 = mati; di Kubernetes dan kedua env example). Interval-nya terpisah dari sync 5 menit karena build membaca seluruh Bronze, dan putaran pertama menunggu satu interval penuh setelah start. Log hanya mencatat **perubahan** status per fact (selisih baru, pulih, atau tidak bisa diperiksa) plus ringkasan "N/17 facts match"; keadaan tetap seperti `EXTRA_IN_LAKE` permanen tidak memenuhi log tiap jam, sedangkan galat build dicatat setiap putaran. Backfill sengaja TIDAK otomatis: log hanya menunjuk ke `POST /api/dw/lake/backfill`.

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


**Production:** pakai akun least-privilege (`infra/minio/dw-service-policy.json`), bukan root. **Jangan pasang expiry pada Bronze**: build penuh Silver membaca seluruh Bronze, jadi objek yang kedaluwarsa membuat baris hilang dari Silver (`MISSING_FROM_LAKE`). Aturan lifecycle `AbortIncompleteMultipartUpload` ditolak MinIO; upload yatim dibersihkan MinIO sendiri (`api stale_uploads_expiry`, default 24 jam). Detail dan hasil verifikasi: `infra/minio/README.md`.
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

**Pembaruan: Silver inkremental sudah dibuat (perbaikan (b) di atas); hasil ukurnya ada di bawah.** Angka pada tabel di atas diambil SEBELUM perubahan itu. Pengukuran ulang pada 500 rb baris (dengan jeda 10,5 menit supaya objek Bronze mengendap) dihentikan sebelum build inkremental sempat berjalan, jadi TIDAK ada angka penghematan. Yang terverifikasi adalah **kebenarannya**: hasil inkremental identik byte-per-byte dengan build penuh setelah tiap dari 25 ronde acak (baris baru, pembaruan, baris berulang antar objek), dan merusak urutan merge (versi lama menang pada kunci yang sama) langsung tertangkap di ronde pertama.

**Hasil ukur Silver inkremental (2026-10-21).** Diukur dengan `TestSilverScale_IncrementalVsFull` (`BENCH_ROWS=500000 go test ./internal/datalake -run SilverScale -v`), baris ±250 B, MinIO lokal, umur objek Bronze dipalsukan lewat nama key. Puncak heap Go selama build (bukan RSS proses), delta = 1% baris (separuh pembaruan, separuh baru). Hasil inkremental identik byte-per-byte dengan build penuh paksa pada tiap skenario.

| 500 rb baris | Waktu | Puncak heap |
|---|---|---|
| Build penuh | ±27 dtk | ±420 MB |
| Inkremental, tanpa data baru | ±8 dtk | ±34 MB |
| Inkremental, delta 1% | ±9,5 dtk | ±39 MB |
| Penuh + kunci hidup 500 rb | ±27 dtk | ±440–460 MB |
| Inkremental + kunci hidup 500 rb | ±7,5 dtk | ±145–155 MB |

Pada 100 rb baris: penuh ±6–8 dtk / 91 MB, inkremental ±2–3 dtk / 34 MB. Kesimpulan: inkremental ±3× lebih cepat dan memori ±12× lebih kecil (±3× dengan peta kunci hidup, karena peta itulah yang kini mendominasi), dan biayanya tumbuh dengan ukuran Silver, bukan dengan delta (tanpa data baru pun ±8 dtk, karena Silver lama tetap dibaca dan ditulis ulang sebagai aliran). Memori build penuh tetap linear (±0,85 KB/baris di heap Go; angka 3,5 KB/baris di atas mencakup seluruh proses). **Tidak diukur:** ekstraksi kunci hidup dari Postgres (peta sintetis dibangun di memori), MinIO di jaringan, dan di atas 500 rb baris. Kunci hidup 16 byte (perbaikan (a)) masih perbaikan berikutnya yang paling berdampak.

**Kunci hidup dipadatkan (2026-10-21).** Daftar kunci hidup kini `datalake.KeySet`: kunci `uuid/uuid` kanonik disimpan sebagai 32 byte tetap, selain itu jatuh ke peta string dengan semantik identik (dua ejaan UUID berbeda tetap dua kunci). Diukur ulang pada 500 rb baris: puncak heap inkremental + kunci hidup turun dari ±145–155 MB ke ±111–117 MB (build penuh tidak berubah, ±440–480 MB). Penghematannya lebih kecil dari yang diharapkan karena bagian terbesar yang tersisa adalah peta delta dan buffering, bukan peta kunci. **Ekstrak kunci hidup dari sumber (2026-10-21).** Sebelumnya `liveKeys` menampung SEMUA baris utuh lalu meng-marshal+unmarshal tiap baris hanya untuk dua kolom. Kini tiap `extractX` dibangun di atas `eachX` yang menyalurkan baris satu per satu (query dan filter TETAP yang sama dengan sync, jadi "masih ada" tetap berarti "masih akan disalin"), dan kunci dihitung lewat jalur cepat `fastKey` dengan jalur lambat sebagai cadangan (diuji sama hasilnya). Diukur pada 300 rb baris iot_readings di Postgres uji: puncak heap ±214 MB -> ±40 MB, hitung kunci ±17 dtk -> ±3,4 dtk; waktu ekstrak (query + scan, ±4–7 dtk, bervariasi antar run) tidak berubah karena semua kolom tetap dikirim dari Postgres. **Query kunci-saja (2026-10-21).** Kunci hidup kini diambil dengan query dua kolom (id + company_id), bukan seluruh baris. Query itu TIDAK ditulis tangan per fact: `keysOnlySQL` (`internal/etl/keysonly.go`) mengambil SQL ekstrak fact itu sendiri dan hanya mengganti daftar SELECT, jadi FROM/JOIN/WHERE dipakai apa adanya dan filternya tidak bisa menyimpang; ORDER BY dibuang. Satu-satunya hal yang ditulis tangan per fact adalah indeks kolom company_id (2 untuk ecommerce, finance, hr, purchasing, sales; 1 untuk sisanya). Risikonya (kunci yang salah memangkas baris yang masih ada, tanpa galat) dijaga `TestKeysOnly_MatchesStreamedExtract`: untuk SEMUA 18 fact, himpunan kunci dari query kunci-saja harus sama persis dengan himpunan dari ekstrak penuh (cara lama, kini hanya acuan di test), dan test menolak perbandingan kosong-dengan-kosong. Diuji dengan merusak indeks company_id sales: test gagal di kedua arah (baris hilang dan baris hantu). Diukur pada 300 rb baris iot_readings: waktu ±4,9 dtk -> ±1,3 dtk, puncak heap ±212 MB -> ±35 MB (sebagian besar sisanya adalah himpunan kunci itu sendiri). Fact baru wajib menambahkan entri di `liveKeysCases` dan memilih indeks company_id-nya; test itu yang akan memberi tahu kalau salah.

Yang perlu diketahui sebelum mengharapkan hasilnya:

- **Memori hanya turun pada build inkremental.** Build penuh (pertama, tiap 24 jam, `?full=true`) masih menampung semua baris unik di map delta, jadi puncak memori build penuh sama seperti sebelumnya dan batas ±2–3 jt baris per fact belum terangkat selama build penuh harian ada.
- **Lantai biaya waktu belum turun.** Tiap build, termasuk inkremental, masih menanyakan kunci hidup ke sumber lewat ekstrak seluruh tabel (penyebab 2) untuk memangkas baris yang dihapus, dan itu tetap O(seluruh baris) dalam waktu dan memori. Penghematan inkremental adalah membaca ulang Bronze (penyebab 4 dan sebagian 1); perbaikan (a), kunci 16 byte tanpa baris utuh, tetap belum dikerjakan dan kini menjadi langkah berikutnya yang paling berdampak.
- **Asumsi urutan.** Objek Bronze yang muncul belakangan dengan key lebih tua dari penanda tidak terlihat oleh build inkremental sampai build penuh berikutnya (paling lama 24 jam); ini terjadi hanya kalau penulis menahan objek lebih dari 10 menit sejak `synced_at`-nya (mis. insert ClickHouse yang sangat lambat). Rekonsiliasi akan melihatnya sebagai selisih, dan `?full=true` memulihkannya (diuji).

**Yang TIDAK diukur:** fact lain (diasumsikan orde yang sama; baris IoT lebih sempit sehingga memori per baris lebih kecil), waktu `count(*) ... FINAL` untuk rekonsiliasi dan query analitik ClickHouse pada puluhan juta baris, biaya sort ekstrak Postgres tanpa indeks `updated_at`, dan ruang disk. Tahap tertinggi yang dijalankan adalah 500 rb baris — 1 jt akan butuh ±3,5 GB sementara hanya ±3,4 GB RAM yang bebas — sehingga angka di atasnya adalah ekstrapolasi.

## Fact tambahan: `production_downtime` (2026-10-21)

`fact_production_oee` hanya menyimpan TOTAL menit downtime per run, jadi alasan berhentinya tidak bisa ditanyakan dari gudang data. `fact_production_downtime` (fact ke-18, Silver id `DowntimeID`) menyimpan satu baris per catatan downtime dari run yang sudah ditutup, dengan `reason_code` dan menitnya. Dua keputusan yang perlu diketahui:

- **Watermark memakai `production_runs.updated_at`, bukan `downtime_logs.created_at`.** Catatan downtime dibuat selagi run masih berjalan; watermark `created_at` sudah lewat saat run ditutup dan catatannya tidak akan pernah terambil. Menutup run menaikkan `updated_at`, jadi semua catatan run itu terambil saat itu (diuji).
- **Batasnya sama dengan `fact_production_oee`** (hanya run CLOSED), sehingga jumlah menit downtime di kedua fact selalu sama untuk periode yang sama (diuji).

Endpoint: `GET /api/dw/analytics/production-downtime-pareto?company_id=...&from=YYYY-MM-DD&to=YYYY-MM-DD` (`from`/`to` opsional, tanggal run, inklusif) mengembalikan alasan dari yang terbesar dengan `minutes`, `share`, dan `cumulative_share` (pecahan 0-1, baris terakhir tepat 1). Dashboard Manufaktur menampilkannya sebagai bar menit per alasan; garis kumulatif tidak digambar karena chart-nya bersumbu tunggal.
