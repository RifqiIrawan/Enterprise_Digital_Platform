# Handoff — Lanjutan Sesi Berikutnya

Terakhir dikerjakan: **2026-07-11**. Dokumen ini ringkasan supaya sesi besok bisa langsung lanjut tanpa re-explore dari nol.

---

## Status Ringkas

| Bagian | Status |
|---|---|
| **Fase 1** — auth, company, rbac, audit, api-gateway | ✅ Selesai & diverifikasi end-to-end |
| **Fase 2 — Finance module** | ✅ Selesai & diverifikasi end-to-end |
| **Fase 2 — HR module** (employees, attendance, payroll + posting ke GL) | ✅ Selesai & diverifikasi end-to-end di browser |
| **Fase 2 — Sales module** (customers, quotations, sales orders + invoice ke GL) | ✅ Selesai & diverifikasi end-to-end di browser |
| **Fase 2 — Purchasing, Warehouse, Production, QC, Asset, AI, BI** | ⏳ Belum dikerjakan (masih placeholder README di `backend/modules/`) |
| Frontend — DataTable (search+sort+pagination) di semua halaman list | ✅ Selesai |
| Kafka/Redis/MinIO/ClickHouse (docker-compose) | ❌ Belum bisa dites — Docker Desktop error di mesin ini (lihat "Known Issues") |
| Git | ✅ Commit pertama (`a796b1b`, checkpoint Fase 1+Finance+HR) dan kedua (`1fa8c1f`, Sales) sudah dibuat di branch `master`. Belum ada remote. |

---

## Cara Menjalankan (urutan)

1. **Postgres** — jalan native sebagai Windows service (`postgresql-x64-18`), tidak perlu langkah apa pun kalau service Windows-nya sudah `Running`. Role `platform`/`platform` dan database `auth_service`, `company_service`, `rbac_service`, `audit_service`, `finance_service` sudah dibuat.
2. **(Opsional) Infra Kafka/Redis/MinIO/ClickHouse** — coba dulu:
   ```
   cd infra
   docker compose up -d
   ```
   Kalau Docker Desktop error (`pipe error 500` / `request returned 500 Internal Server Error`), restart Docker Desktop dulu. Semua service Go **tetap bisa jalan tanpa ini** — publish/consume Kafka didesain best-effort (gagal → log warning, tidak crash).
3. **Backend — 8 service Go**, masing-masing `go run ./cmd/server` di foldernya:
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

   Migrasi jalan otomatis saat startup (embed FS + tabel `schema_migrations`, aman dijalankan berkali-kali). Databasenya (`hr_service`, `sales_service`) perlu dibuat dulu kalau belum ada (`CREATE DATABASE hr_service;` / `CREATE DATABASE sales_service;` lewat psql, role `platform`).
4. **Frontend**:
   ```
   cd frontend/web
   npm install   # kalau belum
   npm run dev   # default port 3000, auto-naik ke port lain kalau kepakai
   ```
5. **Login**: `admin@edp.local` / `Admin@12345` (Super Admin). User lain hasil seed demo pakai password `Password123`.

Cek cepat semua service hidup:
```bash
for port in 8079 8081 8082 8083 8084 8085 8086 8087; do curl -s http://localhost:$port/health; echo; done
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

HR dan Sales sudah selesai (lihat tabel status di atas). Modul Fase 2 berikutnya, urutan disarankan:
1. **Purchasing** — vendor, PR/PO; polanya sama seperti Sales (lihat `backend/modules/sales-service` sebagai contoh lengkap termasuk `internal/financeclient`). Invoice AP di finance-service idealnya dihubungkan ke sini (`partner_name` teks bebas → jadi `vendor_id` referensi), sama seperti catatan `invoice_id` di `sales_orders`.
2. Sisanya (Warehouse, Production, QC, Asset, AI, BI) menyusul.

Sebelum mulai modul baru, cek dulu apakah proses lain (lihat Known Issues #2) sudah/sedang mengerjakan modul yang sama — hindari tabrakan file/port seperti awal sesi lalu.

### Pola cross-service posting (HR & Sales sudah pakai ini)

Kalau modul baru butuh membuat journal entry / invoice di finance-service (mis. Purchasing untuk AP invoice), ikuti pola `internal/financeclient` di `hr-service` (posting payroll ke journal entry) atau `sales-service` (posting sales order ke invoice AR):
- Panggilan HTTP langsung ke `FINANCE_SERVICE_URL` (bukan lewat api-gateway), karena finance-service tidak validasi JWT.
- Header `X-User-Id` diteruskan manual dari actor pemanggil supaya tercatat sebagai `posted_by`/`actor_user_id` yang benar (harus UUID valid, bukan sembarang string).
- Urutan: panggil finance-service dulu, baru update status lokal setelah sukses (tidak ada distributed transaction, jadi finance-service adalah sumber kebenaran duluan).
