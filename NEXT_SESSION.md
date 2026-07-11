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
| **Fase 2 — Production, QC, Asset, AI, BI** | ⏳ Belum dikerjakan (masih placeholder README di `backend/modules/`) |
| Frontend — DataTable (search+sort+pagination) di semua halaman list | ✅ Selesai |
| Kafka/Redis/MinIO/ClickHouse (docker-compose) | ❌ Belum bisa dites — Docker Desktop error di mesin ini (lihat "Known Issues") |
| Git | ✅ 4 commit di branch `master`: `a796b1b` (checkpoint Fase 1+Finance+HR), `1fa8c1f` (Sales), `8ee2ef6` (update dok), `6440e11` (Purchasing), `f9e58a0` (update dok). Warehouse module belum di-commit (lihat bawah). Belum ada remote. |

---

## Cara Menjalankan (urutan)

1. **Postgres** — jalan native sebagai Windows service (`postgresql-x64-18`), tidak perlu langkah apa pun kalau service Windows-nya sudah `Running`. Role `platform`/`platform` dan database `auth_service`, `company_service`, `rbac_service`, `audit_service`, `finance_service` sudah dibuat.
2. **(Opsional) Infra Kafka/Redis/MinIO/ClickHouse** — coba dulu:
   ```
   cd infra
   docker compose up -d
   ```
   Kalau Docker Desktop error (`pipe error 500` / `request returned 500 Internal Server Error`), restart Docker Desktop dulu. Semua service Go **tetap bisa jalan tanpa ini** — publish/consume Kafka didesain best-effort (gagal → log warning, tidak crash).
3. **Backend — 10 service Go**, masing-masing `go run ./cmd/server` di foldernya:
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

   Migrasi jalan otomatis saat startup (embed FS + tabel `schema_migrations`, aman dijalankan berkali-kali). Databasenya (`hr_service`, `sales_service`, `purchasing_service`, `warehouse_service`) perlu dibuat dulu kalau belum ada (`CREATE DATABASE hr_service;` dst lewat psql, role `platform`).
4. **Frontend**:
   ```
   cd frontend/web
   npm install   # kalau belum
   npm run dev   # default port 3000, auto-naik ke port lain kalau kepakai
   ```
5. **Login**: `admin@edp.local` / `Admin@12345` (Super Admin). User lain hasil seed demo pakai password `Password123`.

Cek cepat semua service hidup:
```bash
for port in 8079 8081 8082 8083 8084 8085 8086 8087 8088 8089; do curl -s http://localhost:$port/health; echo; done
```

---

## ⚠️ Known Issues / Perlu Perhatian

1. **Docker Desktop error di mesin ini** (`pipe error 500`) — sudah didokumentasikan lama di `infra/README.md`, bukan hal baru. Kafka/Redis/MinIO/ClickHouse belum pernah benar-benar dites sepanjang sesi ini. Kalau mau tes alur Kafka (audit trail masuk ke `audit_logs`), perbaiki Docker dulu, lalu restart `audit-service` supaya consumer reconnect.
2. **Sempat terdeteksi proses lain yang aktif menulis file & menjalankan service** di awal sesi ini (pola `internal/handler`+`repository`+`router`+`service`, beda dari pola yang dipakai sesi ini yaitu `internal/httpapi`+`store`+`eventbus`). User mengonfirmasi untuk lanjut pakai implementasi sesi ini; implementasi paralel tsb di-backup (bukan dihapus permanen dari disk manapun kecuali working tree) ke:
   ```
   C:\Users\rifqi\AppData\Local\Temp\claude\C--xampp7-4-htdocs-Enterprise-Digital-Platform\3c0fee6f-b631-4b9e-aee7-e6df0ff3786f\scratchpad\parallel-impl-backup\
   ```
   **Ini folder scratchpad sesi (temporary), bisa hilang.** Kalau ternyata itu kerjaan penting dari sesi lain yang belum di-commit, cek folder itu SEGERA di awal sesi besok sebelum scratchpad dibersihkan sistem, atau tanyakan ke user apakah masih relevan.
3. **Belum ada commit git** — semua pekerjaan (Fase 1 + Finance module) masih di working tree, belum di-`git add`/`commit`. `git status` di root masih akan menunjukkan semuanya untracked. Pertimbangkan commit checkpoint di awal sesi besok (tanya user dulu, jangan commit otomatis).

---

## Data Demo yang Sudah Di-seed (Session Ini)

Supaya pagination DataTable kelihatan, ditambahkan data dummy lewat API (bukan migration seed — jadi kalau database di-reset dari nol, data ini hilang dan perlu diulang manual kalau mau demo):
- 16 Chart of Accounts (company `DEFAULT`, id `89e53684-a8e4-4e3e-be37-15f00eef232e`)
- 11 Journal Entries (2 dari posting invoice real, 9 dummy "Transaksi demo #")
- 11 Invoices (5 AR + 5 AP dummy partner, 1 invoice AR asli yang sudah di-post lengkap ke GL)
- 14 Users (termasuk `budi@edp.local` dan 10 user dummy nama Indonesia)
- 14 Roles (13 role sistem bawaan + 1 custom "Finance Viewer" hasil testing)
- Warehouse module (dari verifikasi end-to-end Playwright sesi ini): 2 gudang (`WH-A` Gudang Utama A, `WH-B` Gudang Cabang B), 1 produk (`SKU-TEST-01` Produk Uji Coba), 1 PO diterima (10 pcs masuk ke WH-A), 1 SO di-fulfill (3 pcs keluar dari WH-A), 1 stock transfer WH-A→WH-B (2 pcs, CONFIRMED), 1 stock opname di WH-A (disesuaikan ke 100 pcs, POSTED). Saldo akhir WH-A = 100 pcs setelah opname (bukan hasil hitung fisik asli, cuma angka tes) — jangan bingung kalau demo ke user, cukup jelaskan ini data uji coba.

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

HR, Sales, Purchasing, dan Warehouse sudah selesai (lihat tabel status di atas). Modul Fase 2 berikutnya, urutan disarankan:
1. **Production** — work order, bill of material, jadwal produksi. Kemungkinan konsumen terbesar dari product master & stock movement yang baru dibangun di Warehouse: BOM butuh referensi ke `products` (warehouse-service), dan penyelesaian work order kemungkinan perlu stock OUT (bahan baku) + stock IN (barang jadi) lewat pola `internal/warehouseclient` yang sudah ada di purchasing-service/sales-service (lihat bawah).
2. Sisanya (QC, Asset, AI, BI) menyusul.

Sebelum mulai modul baru, cek dulu apakah proses lain (lihat Known Issues #2) sudah/sedang mengerjakan modul yang sama — hindari tabrakan file/port seperti awal sesi lalu.

### Pola cross-service posting (HR, Sales, Purchasing, & Warehouse sudah pakai ini)

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
