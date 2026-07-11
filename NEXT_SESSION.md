# Handoff — Lanjutan Sesi Berikutnya

Terakhir dikerjakan: **2026-07-11**. Dokumen ini ringkasan supaya sesi besok bisa langsung lanjut tanpa re-explore dari nol.

---

## Status Ringkas

| Bagian | Status |
|---|---|
| **Fase 1** — auth, company, rbac, audit, api-gateway | ✅ Selesai & diverifikasi end-to-end |
| **Fase 2 — Finance module** | ✅ Selesai & diverifikasi end-to-end |
| **Fase 2 — HR module** (employees, attendance, payroll + posting ke GL) | ✅ Selesai & diverifikasi end-to-end di browser |
| **Fase 2 — Sales module** (customers, quotations, sales orders + invoice AR ke GL) | ✅ Selesai & diverifikasi end-to-end di browser |
| **Fase 2 — Purchasing module** (suppliers, requisitions, purchase orders + invoice AP ke GL) | ✅ Selesai & diverifikasi end-to-end di browser |
| **Fase 2 — Warehouse module** (products, warehouses, stock balance/movement, stock transfer, stock opname; PO RECEIVED → stock in, SO FULFILLED → stock out) | ✅ Selesai & diverifikasi end-to-end di browser (Playwright) |
| **Fase 2 — Production module** (Bill of Material, Work Order draft→in_progress→completed, jadwal produksi; WO COMPLETED → konsumsi komponen & tambah produk jadi di stock) | ✅ Selesai & diverifikasi end-to-end di browser (Playwright) |
| **Fase 2 — QC module** (Standar Mutu per produk, Inspeksi Kualitas dengan hasil PASS/FAIL/PARTIAL otomatis, opsional terhubung ke PO/Work Order) | ✅ Selesai & diverifikasi end-to-end di browser (Playwright) |
| **Fase 2 — Asset module** (Pendataan Aset, Maintenance Schedule dengan overdue indicator, complete/cancel) | ✅ Selesai & diverifikasi end-to-end di browser (Playwright) |
| **Fase 2 — AI & BI** | ⏳ Belum dikerjakan (masih placeholder README di `backend/modules/`). **Ini modul terakhir Fase 2** — lihat catatan khusus di "Next Steps" sebelum mulai, kemungkinan besar butuh pendekatan beda dari modul CRUD transaksional lainnya. |
| Frontend — DataTable (search+sort+pagination) di semua halaman list | ✅ Selesai |
| Kafka/Redis/MinIO/ClickHouse (docker-compose) | ✅ Sudah jalan & diverifikasi sesi ini (lihat detail di bawah) — Docker Desktop ternyata sehat sesi ini, bukan gagal permanen seperti diduga sesi-sesi sebelumnya |
| Git | ✅ 7 commit di branch `master`: `a796b1b` (checkpoint Fase 1+Finance+HR), `1fa8c1f` (Sales), `8ee2ef6` (update dok), `6440e11` (Purchasing), `f9e58a0` (update dok), `7590cd5` (Warehouse), `c0e600b` (Production), `b08a005` (QC), `359d6dc` (Asset). Docker fix + audit topics belum di-commit (lihat bawah). Belum ada remote. |

---

## Cara Menjalankan (urutan)

1. **Postgres** — jalan native sebagai Windows service (`postgresql-x64-18`), tidak perlu langkah apa pun kalau service Windows-nya sudah `Running`. Role `platform`/`platform` dan database `auth_service`, `company_service`, `rbac_service`, `audit_service`, `finance_service` sudah dibuat.
2. **Infra Kafka/Redis/MinIO/ClickHouse** — coba dulu, JANGAN asumsikan Docker pasti rusak (lihat "Known Issues" #1, sesi 2026-07-12 ternyata sehat):
   ```
   cd infra
   docker compose up -d
   ```
   Kalau Docker Desktop error (`pipe error 500` / `request returned 500 Internal Server Error`), restart Docker Desktop dulu. Semua service Go **tetap bisa jalan tanpa ini** — publish/consume Kafka didesain best-effort (gagal → log warning, tidak crash). Kalau image `bitnami/kafka:*` gagal pull ("not found"), itu karena Bitnami memindahkan image gratisnya ke `bitnamilegacy/*` (lihat komentar di `infra/docker-compose.yml`, sudah di-fix ke `bitnamilegacy/kafka:3.7.1`).
   **Penting**: kalau restart `audit-service` setelah Kafka baru naik, restart SEKALI LAGI kalau consumer group-nya baru saja subscribe topic yang baru saja auto-created (lihat Known Issues #4) — assignment pertama bisa miss topic yang belum ada saat join.
3. **Backend — 13 service Go**, masing-masing `go run ./cmd/server` di foldernya:
   | Service | Path | Port |
   |---|---|---|
   | api-gateway | `backend/services/api-gateway` | 8079 |
   | auth-service | `backend/services/auth-service` | 8081 |
   | company-service | `backend/services/company-service` | 8082 |
   | rbac-service | `backend/services/rbac-service` | 8083 |
   | audit-service | `backend/services/audit-service` | 8084 |
   | finance-service | `backend/modules/finance-service` | 8085 |
   | hr-service | `backend/modules/hr-service` | 8086 |
   | sales-service | `backend/modules/sales-service` | 8087 |
   | purchasing-service | `backend/modules/purchasing-service` | 8088 |
   | warehouse-service | `backend/modules/warehouse-service` | 8089 |
   | production-service | `backend/modules/production-service` | 8090 |
   | qc-service | `backend/modules/qc-service` | 8091 |
   | asset-service | `backend/modules/asset-service` | 8092 |

   Migrasi jalan otomatis saat startup (embed FS + tabel `schema_migrations`, aman dijalankan berkali-kali). Databasenya (`hr_service`, `sales_service`, `purchasing_service`, `warehouse_service`, `production_service`, `qc_service`, `asset_service`) perlu dibuat dulu kalau belum ada (`CREATE DATABASE hr_service;` dst lewat psql, role `platform`).
4. **Frontend**:
   ```
   cd frontend/web
   npm install   # kalau belum
   npm run dev   # default port 3000, auto-naik ke port lain kalau kepakai
   ```
5. **Login**: `admin@edp.local` / `Admin@12345` (Super Admin). User lain hasil seed demo pakai password `Password123`.

Cek cepat semua service hidup:
```bash
for port in 8079 8081 8082 8083 8084 8085 8086 8087 8088 8089 8090 8091 8092; do curl -s http://localhost:$port/health; echo; done
```

---

## ⚠️ Known Issues / Perlu Perhatian

1. ~~Docker Desktop error di mesin ini~~ — **update 2026-07-12: Docker ternyata sehat sesi ini**, `docker compose up -d` di `infra/` jalan bersih (Kafka, Redis, MinIO, ClickHouse, Kafka UI semua Up & reachable). Histori error `pipe error 500` di sesi-sesi sebelumnya kemungkinan besar karena beban memori dari project Docker lain di mesin ini (lihat memory `docker_instability_native_postgres`), bukan kerusakan permanen — coba dulu sebelum asumsi rusak. Dua bug nyata yang ditemukan & sudah di-fix di sesi ini:
   - `bitnami/kafka:3.7` sudah tidak bisa di-pull gratis (Bitnami memindahkannya ke langganan berbayar per 2025) → diganti `bitnamilegacy/kafka:3.7.1` di `infra/docker-compose.yml`.
   - `kafka-ui` sebelumnya mapped ke host port **8090**, sekarang bentrok dengan `production-service` milik project ini sendiri (ditambahkan sesi setelah docker-compose ini terakhir diedit) → dipindah ke port **8099**.
2. **Audit-service consumer group vs topic yang baru auto-created** — kalau `audit-service` di-restart lalu langsung dites dengan event dari topic yang BELUM PERNAH ADA sebelumnya di Kafka (baru auto-create saat consumer join), assignment generasi pertama bisa skip topic itu (event tersimpan di Kafka tapi tidak ke-assign ke consumer manapun, `kafka-consumer-groups.sh --describe` tidak menunjukkan baris untuk topic itu). Fix-nya cukup restart `audit-service` SEKALI LAGI setelah topic-nya sudah pasti ada — assignment generasi kedua akan meng-cover semua topic dengan benar. Ini murni race condition startup, bukan bug di kode `internal/consumer/consumer.go`.
   Terkait ini, sesi ini juga menambahkan topic modul Warehouse/Production/QC/Asset (`warehouse.*`, `production.*`, `qc.*`, `asset.*`) ke `Topics` list di `backend/services/audit-service/internal/consumer/consumer.go` — sebelumnya cuma topic dari Fase 1 + HR/Sales/Purchasing yang terdaftar, karena belum pernah bisa dites sampai Docker beneran jalan sesi ini.
3. **Sempat terdeteksi proses lain yang aktif menulis file & menjalankan service** di awal sesi ini (pola `internal/handler`+`repository`+`router`+`service`, beda dari pola yang dipakai sesi ini yaitu `internal/httpapi`+`store`+`eventbus`). User mengonfirmasi untuk lanjut pakai implementasi sesi ini; implementasi paralel tsb di-backup (bukan dihapus permanen dari disk manapun kecuali working tree) ke:
   ```
   C:\Users\rifqi\AppData\Local\Temp\claude\C--xampp7-4-htdocs-Enterprise-Digital-Platform\3c0fee6f-b631-4b9e-aee7-e6df0ff3786f\scratchpad\parallel-impl-backup\
   ```
   **Ini folder scratchpad sesi (temporary), bisa hilang.** Kalau ternyata itu kerjaan penting dari sesi lain yang belum di-commit, cek folder itu SEGERA di awal sesi besok sebelum scratchpad dibersihkan sistem, atau tanyakan ke user apakah masih relevan.
4. ~~Belum ada commit git~~ — sudah tidak relevan, semua pekerjaan sudah di-commit bertahap per modul (lihat tabel Git di atas).

---

## Data Demo yang Sudah Di-seed (Session Ini)

Supaya pagination DataTable kelihatan, ditambahkan data dummy lewat API (bukan migration seed — jadi kalau database di-reset dari nol, data ini hilang dan perlu diulang manual kalau mau demo):
- 16 Chart of Accounts (company `DEFAULT`, id `89e53684-a8e4-4e3e-be37-15f00eef232e`)
- 11 Journal Entries (2 dari posting invoice real, 9 dummy "Transaksi demo #")
- 11 Invoices (5 AR + 5 AP dummy partner, 1 invoice AR asli yang sudah di-post lengkap ke GL)
- 14 Users (termasuk `budi@edp.local` dan 10 user dummy nama Indonesia)
- 14 Roles (13 role sistem bawaan + 1 custom "Finance Viewer" hasil testing)
- Warehouse module (dari verifikasi end-to-end Playwright sesi ini): 2 gudang (`WH-A` Gudang Utama A, `WH-B` Gudang Cabang B), 1 produk (`SKU-TEST-01` Produk Uji Coba), 1 PO diterima (10 pcs masuk ke WH-A), 1 SO di-fulfill (3 pcs keluar dari WH-A), 1 stock transfer WH-A→WH-B (2 pcs, CONFIRMED), 1 stock opname di WH-A (disesuaikan ke 100 pcs, POSTED). Saldo akhir WH-A = 100 pcs setelah opname (bukan hasil hitung fisik asli, cuma angka tes) — jangan bingung kalau demo ke user, cukup jelaskan ini data uji coba.
- Production module (dari verifikasi end-to-end Playwright sesi ini): 2 produk tambahan (`SKU-RAW-01` Bahan Baku X, `SKU-FG-01` Barang Jadi Y), 1 BOM (`BOM-01` Resep Barang Jadi Y, 2x Bahan Baku X per unit), 1 work order (`WO-202607-0001`, rencana 10 pcs, COMPLETED) — hasil akhir di WH-A: Bahan Baku X 100→80 pcs (terpakai 20), Barang Jadi Y 0→10 pcs (hasil produksi).
- QC module (dari verifikasi end-to-end Playwright sesi ini): 1 standar mutu (`QS-01` Standar Barang Jadi Y), 3 inspeksi — `INS-202607-0001` terhubung ke `WO-202607-0001` (10 diperiksa, 8 lolos, 2 gagal → PARTIAL), `INS-202607-0002` terhubung ke `PO-202607-0001` (5/5/0 → PASS), `INS-202607-0003` manual (3/0/3 → FAIL). Tidak ada mutasi stok otomatis dari inspeksi ini (sesuai keputusan produk).
- Asset module (dari verifikasi end-to-end Playwright sesi ini): 1 aset (`AST-01` Forklift Toyota 3 Ton, lokasi WH-A, status ACTIVE), 2 jadwal maintenance — satu tanggal mundur (diselesaikan → COMPLETED, `completed_date` terisi otomatis), satu tanggal maju (dibatalkan → CANCELLED, untuk uji alur cancel).
- Sanity check audit trail Docker (lewat curl langsung, bukan browser): 1 gudang (`WH-TEST-AUDIT`), 1 produk (`SKU-AUDIT-TEST`), 1 aset (`AST-AUDIT-TEST`) — cuma dipakai untuk verifikasi event Kafka masuk ke `audit_logs`, aman dihapus atau dibiarkan.

Semua ini valid & aman untuk terus dipakai / didemokan, bukan data korup.

---

## Pola Arsitektur yang Sudah Established (ikuti pola ini untuk modul Fase 2 berikutnya)

Setiap service baru (lihat `backend/modules/finance-service` sebagai contoh lengkap):
```
{service}/
├── cmd/server/main.go          # wiring: config -> store.Connect -> store.Migrate -> eventbus -> httpapi -> ListenAndServe
├── internal/config/config.go   # getEnv pattern, DATABASE_URL default ke Postgres lokal
├── internal/model/*.go         # struct + json/db tags
├── internal/store/store.go     # copy persis dari service lain (pgxpool.Connect + Migrate embed-FS)
├── internal/eventbus/eventbus.go  # copy persis (kafka-go Writer, best-effort, non-blocking via goroutine)
├── internal/httpapi/
│   ├── handler.go              # Handler struct, Register(mux), auditEvent{} + newAuditEvent()
│   └── {domain}.go             # handler per resource, pakai http.ServeMux Go 1.22+ pattern routing
├── migrations/
│   ├── embed.go                # //go:embed *.sql
│   └── 001_init.sql
└── go.mod
```
- Tambahkan ke `backend/go.work` (`use (...)`).
- Tambahkan route baru di `backend/services/api-gateway/internal/gateway/gateway.go` (`{PREFIX}ServiceURL` di config + entry di `routes`).
- Kalau butuh menu baru di sidebar: tambah migration baru (bukan edit yang lama) di `backend/services/rbac-service/migrations/`, isi `menus` + `role_menu_permissions` (minimal untuk `super_admin`, `auditor` view-only, role fungsional terkait, `company_admin`/`branch_manager`).
- Event ke Kafka: definisikan `auditEvent` struct lokal (jangan shared package, sesuai prinsip microservices di dokumen), publish topic `{domain}.{entity}.{action}` sesuai `infra/kafka/topics.md`, lalu daftarkan topic itu di `backend/services/audit-service/internal/consumer/consumer.go` (var `Topics`).

Frontend, per halaman list baru:
- Pakai `components/common/DataTable.jsx` (search + sort + pagination built-in) — lihat `pages/finance/*.jsx` sebagai contoh kolom dengan `render`, `maxWidth` (truncate + hover tooltip via `TruncatedText`), `sortValue`.
- Pola fetch company: `apiClient.get('/api/company/companies')` lalu pakai `data[0].id` sebagai company_id (belum ada company switcher UI).
- Modal lewat `components/common/Modal.jsx`, form pakai `<form id="..." onSubmit={...}>` + tombol submit di `footer` Modal yang reference `form="..."`.

---

## Next Steps (rekomendasi)

HR, Sales, Purchasing, Warehouse, Production, QC, dan Asset sudah selesai (lihat tabel status di atas). **Seluruh modul Fase 2 yang bersifat CRUD transaksional sudah lengkap.** Sisa satu modul: **AI & BI**.

⚠️ **PENTING — jangan langsung scaffold AI/BI dengan pola yang sama seperti 8 modul sebelumnya.** Menu yang sudah di-seed (`dashboards` /ai-bi/dashboards, `forecasting` /ai-bi/forecasting, `anomaly_detection` /ai-bi/anomaly-detection — lihat 003_seed_business_menus.sql) itu indikatif dari awal proyek, belum tentu representasi scope yang benar. AI/BI secara sifat beda dari modul lain:
- Modul lain masing-masing "memiliki" datanya sendiri (tabel transaksi + master di database sendiri). AI/BI sebaliknya *tidak* punya data transaksi sendiri — tugasnya membaca/agregasi data dari 8 service lain (finance, hr, sales, purchasing, warehouse, production, qc, asset) untuk ditampilkan sebagai dashboard/forecast/anomaly.
- Pertanyaan yang perlu dijawab dulu bersama user sebelum coding: apakah ai-bi-service query langsung ke database service lain (melanggar "no cross-database FK/query" tapi read-only mungkin bisa diterima?), atau lewat HTTP call ke tiap service (lambat kalau agregasi berat), atau butuh semacam data warehouse/materialized view terpisah? Forecasting & anomaly detection juga menyiratkan komputasi (statistik/ML) yang jauh beda dari CRUD+state-transition yang selama ini dipakai.
- Rekomendasi: mulai sesi berikutnya dengan **bertanya ke user** scope realistis untuk sesi ini (mis. "BI Dashboards" saja dulu sebagai read-only aggregation view, tunda forecasting/anomaly yang butuh ML), sebelum menulis kode apa pun.

Sebelum mulai modul baru, cek dulu apakah proses lain (lihat Known Issues #2) sudah/sedang mengerjakan modul yang sama — hindari tabrakan file/port seperti awal sesi lalu.

### Pola cross-service posting (HR, Sales, Purchasing, Warehouse, & Production pakai ini; QC & Asset sengaja TIDAK)

Kalau modul baru butuh membuat journal entry / invoice di finance-service, ikuti pola `internal/financeclient` di `hr-service` (posting payroll ke journal entry), `sales-service` (posting sales order ke invoice AR), atau `purchasing-service` (posting purchase order ke invoice AP):
- Panggilan HTTP langsung ke `FINANCE_SERVICE_URL` (bukan lewat api-gateway), karena finance-service tidak validasi JWT.
- Header `X-User-Id` diteruskan manual dari actor pemanggil supaya tercatat sebagai `posted_by`/`actor_user_id` yang benar (harus UUID valid, bukan sembarang string).
- Urutan: panggil finance-service dulu, baru update status lokal setelah sukses (tidak ada distributed transaction, jadi finance-service adalah sumber kebenaran duluan).

Pola yang sama persis dipakai untuk stock movement lewat `internal/warehouseclient` di purchasing-service (PO RECEIVED, movement_type IN) dan sales-service (SO FULFILLED, movement_type OUT), panggil `WAREHOUSE_SERVICE_URL` (default `http://localhost:8089`), endpoint `POST /stock-movements/batch` di warehouse-service. Modul baru yang butuh menggerakkan stok (mis. Production) tinggal copy package `internal/warehouseclient` ini.

### Warehouse module — detail implementasi (untuk konteks kalau ada bug/lanjutan)

- Product master baru pertama kali ada di sesi ini, hidup di `warehouse-service` (tabel `products`), BUKAN shared package. Sales/Purchasing tetap menyimpan `product_name` teks bebas di baris order mereka (tidak diubah) — saat PO RECEIVED/SO FULFILLED, warehouse-service mencocokkan produk lewat `(company_id, name)` dan auto-create produk baru (SKU `AUTO-XXXXXXXX`) kalau belum ada.
- `stock_balances` adalah saldo ter-materialisasi (bukan SUM dari ledger), di-update transaksional bersamaan tiap insert `stock_movements` lewat helper `applyStockMovement` (`internal/httpapi/stock_movements.go`).
- Stock transfer & stock opname masing-masing punya baris DRAFT dulu, baru menggerakkan stok beneran saat aksi confirm/post (pola sama seperti PO/SO: draft → transisi status yang baru benar-benar berefek pada data lain).
- Endpoint `POST /stock-movements/batch` dipanggil service-to-service langsung (bukan lewat api-gateway) oleh purchasing-service/sales-service — sama seperti pola financeclient.
- Menu RBAC: `stock`, `stock_transfer`, `stock_opname` sudah ada dari seed lama (003-006); menu `products` & `warehouses` baru ditambah migration `008_seed_warehouse_master_menus.sql` sesi ini.
- Sudah diverifikasi end-to-end pakai Playwright (login → buat gudang/produk → PO Confirm→Terima Barang→cek stok masuk → SO Confirm→Fulfill→cek stok keluar → stock transfer→confirm → stock opname→post) — semua jalan tanpa error, saldo akhir sesuai perhitungan manual.

### Production module — detail implementasi (untuk konteks kalau ada bug/lanjutan)

- `bill_of_materials`/`bom_lines` dan `work_orders`/`work_order_lines` hidup di `production-service` (baru, port 8090, db `production_service`). `product_id` (produk jadi) dan `component_product_id` (bahan baku) menunjuk ke tabel `products` milik warehouse-service TANPA FK fisik (beda database) — production-service percaya `product_id` yang dikirim frontend (yang ambil daftar produk langsung dari `GET /api/warehouse/products`, sama seperti pola dropdown produk di halaman Warehouse).
- `work_order_lines` men-snapshot `bom_lines.quantity_per_unit * quantity_planned` saat work order dibuat (bukan re-lookup BOM saat completed), supaya perubahan BOM belakangan tidak mengubah kebutuhan komponen WO yang sudah berjalan.
- Alur status: `DRAFT` → `POST .../start` → `IN_PROGRESS` → `POST .../complete` (body `{quantity_produced}`) → `COMPLETED`. Saat complete, `internal/warehouseclient` (package baru, mirip punya purchasing/sales-service) memanggil warehouse-service `POST /stock-movements/batch` **dua kali**: sekali `movement_type=OUT` untuk semua `work_order_lines` (konsumsi komponen), sekali lagi `movement_type=IN` untuk `product_id` sebanyak `quantity_produced` (hasil produksi) — baru setelah keduanya sukses, status lokal diupdate ke COMPLETED (pola sama seperti invoice/receive/fulfill: panggil service lain dulu, baru update lokal).
- **Penting**: karena production-service sudah tahu `product_id` pasti (bukan cuma nama teks seperti PO/SO), endpoint `POST /stock-movements/batch` di warehouse-service diperluas menerima field `product_id` opsional di tiap baris (selain `product_name` yang sudah ada) — kalau `product_id` diisi, warehouse-service langsung pakai itu tanpa name-matching/auto-create. Ini masih backward-compatible, purchasing-service/sales-service tetap kirim `product_name` seperti sebelumnya.
- **Bug yang sempat ketemu & sudah diperbaiki**: kolom `stock_movements.reference_type` di warehouse-service punya CHECK constraint DB yang cuma mengizinkan `PURCHASE_ORDER/SALES_ORDER/TRANSFER/OPNAME/MANUAL` — lupa nambahin `WORK_ORDER` di constraint-nya (cuma ditambah di validasi Go-nya). Karena migration lama sudah tercatat `applied` di `schema_migrations`, fix-nya lewat migration BARU `002_add_work_order_reference_type.sql` (ALTER CONSTRAINT), bukan edit `001_init.sql`. Kalau nanti nambah `reference_type` baru lagi di modul lain, jangan lupa sinkronkan CHECK constraint DB-nya juga, tidak cukup update `validReferenceTypes` map di Go saja.
- Menu RBAC (`work_orders`, `bom`, `production_schedule`) sudah ada dari seed lama (003/004/005/006 rbac-service) — tidak perlu migration baru untuk Production, beda dengan Warehouse yang sempat butuh 2 menu master baru.
- Halaman "Jadwal Produksi" (`/production/schedule`) bukan tabel baru — cuma me-render ulang data `GET /work-orders` yang sama, dikelompokkan per `planned_start_date` di sisi frontend, supaya tidak over-engineer tabel terpisah untuk sesuatu yang bisa diturunkan dari data yang sudah ada.
- Sudah diverifikasi end-to-end pakai Playwright (login → buat produk bahan baku & produk jadi → stok masuk manual bahan baku → buat BOM → buat Work Order → Mulai → Selesaikan → cek saldo bahan baku turun & produk jadi bertambah di Stok Gudang → cek Jadwal Produksi) — semua jalan tanpa error setelah fix constraint di atas.

### QC module — detail implementasi (untuk konteks kalau ada bug/lanjutan)

- `quality_standards` (kriteria pass/fail per produk) dan `quality_inspections` hidup di `qc-service` (baru, port 8091, db `qc_service`). `product_id` di `quality_standards` menunjuk ke `products` milik warehouse-service tanpa FK fisik, sama seperti pola `production-service`.
- **Sengaja dibuat lebih ringan** dari modul lain sesuai keputusan produk sesi ini: satu inspeksi = satu catatan hasil yang sudah final saat dibuat (tidak ada status DRAFT/POSTED, tidak ada `internal/warehouseclient`, tidak ada mutasi stok otomatis). Kalau hasil FAIL perlu dikoreksi ke saldo stok, itu manual lewat Stock Opname/manual movement di Warehouse.
- `result` (`PASS`/`FAIL`/`PARTIAL`) dihitung server-side saat create, bukan dikirim dari frontend: `FAIL` kalau `passed_quantity == 0`, `PASS` kalau `failed_quantity == 0`, selain itu `PARTIAL`.
- `reference_type` (`PURCHASE_ORDER`/`WORK_ORDER`/`MANUAL`) + `reference_id` + `reference_number` bersifat opsional & informational saja (tanpa FK fisik) — `reference_number` (mis. nomor PO/WO) disimpan sebagai teks bebas di baris inspeksi supaya list bisa tampil tanpa perlu manggil purchasing-service/production-service lagi. Frontend (`QualityInspectionsPage.jsx`) yang bertugas mengambil daftar PO/WO dari service masing-masing untuk dropdown referensi.
- Menu RBAC (`inspections`, `quality_standards`) sudah ada dari seed lama (003/004/005/006 rbac-service) — tidak perlu migration baru untuk QC, sama seperti Production.
- Sudah diverifikasi end-to-end pakai Playwright (login → buat standar mutu untuk Barang Jadi Y → catat 3 inspeksi: satu terhubung ke Work Order dengan sebagian gagal → PARTIAL, satu terhubung ke Purchase Order semua lolos → PASS, satu manual semua gagal → FAIL) — hasil PASS/FAIL/PARTIAL dan link referensi tampil benar, tanpa error.

### Asset module — detail implementasi (untuk konteks kalau ada bug/lanjutan)

- `assets` dan `maintenance_schedules` hidup di `asset-service` (baru, port 8092, db `asset_service`). Beda dari semua modul lain, Asset **tidak melibatkan product master maupun stock sama sekali** — aset fisik perusahaan (mesin, kendaraan, dst), bukan barang dagangan/bahan baku.
- `assets.warehouse_id` sengaja opsional & informational-only (lokasi fisik aset ditaruh di gudang mana) — TIDAK memicu apa pun di warehouse-service, tidak ada stock_movements terkait. Frontend cukup fetch `/api/warehouse/warehouses` untuk dropdown lokasi, sama seperti pola pemilihan warehouse di halaman lain.
- `maintenance_schedules` pakai FK fisik biasa ke `assets` (bukan pola "tanpa FK lintas database") karena satu database yang sama — beda dengan referensi lintas-service (mis. QC ke PO/WO) yang memang harus tanpa FK.
- Alur status maintenance: `SCHEDULED` → `POST .../complete` (isi `completed_date` otomatis ke tanggal hari itu) atau `POST .../cancel` → `COMPLETED`/`CANCELLED` (final, tidak ada balik lagi). Saat `complete`, ada bonus side-effect kecil: kalau `assets.status` sedang `MAINTENANCE`, otomatis dikembalikan ke `ACTIVE` dalam transaksi yang sama (no-op kalau status aset bukan `MAINTENANCE`) — tidak ada langkah otomatis yang MENGUBAH status aset ke `MAINTENANCE` saat schedule dibuat (itu manual lewat edit aset), supaya tidak ada "sihir" yang membingungkan.
- "Overdue" (jadwal yang tanggalnya sudah lewat tapi masih `SCHEDULED`) dihitung di frontend saat render (`MaintenanceSchedulePage.jsx`), bukan status tersendiri di database — menghindari kebutuhan job terjadwal untuk memperbarui status.
- Menu RBAC (`asset_register`, `asset_maintenance`) sudah ada dari seed lama (003/004/005/006 rbac-service) — tidak perlu migration baru, sama seperti Production & QC.
- Sudah diverifikasi end-to-end pakai Playwright (login → buat aset dengan lokasi gudang → jadwalkan maintenance tanggal mundur & tanggal maju → selesaikan yang tanggal mundur (cek completed_date & indikator overdue) → batalkan yang tanggal maju → cek status aset) — semua transisi jalan tanpa error.
