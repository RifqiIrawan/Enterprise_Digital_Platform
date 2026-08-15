# project-service — Project Management

Modul ke-21. Proyek, tugas, dan timesheet, dengan integrasi ke `hr-service`
(karyawan) dan `finance-service` (posting biaya ke GL).

Port `8100`, database `project_service`, route gateway `/api/project`.

Status: **selesai, termasuk production-readiness** (modul inti 2026-08-16,
production-readiness 2026-08-16 hari yang sama). Dockerfile, blok
docker-compose, manifest K8s base + overlay dev/staging/prod, env template
staging/production, entri CI, dan target scrape Prometheus semuanya sudah ada.
Image-nya sudah benar-benar di-build (`docker compose build project-service`)
dan container-nya di-smoke test terhadap Postgres native: `/health` 200,
`/metrics` melayani metrik Prometheus, dan `GET /projects` mengembalikan data
proyek nyata dari dalam container.

## Lingkup

| Entitas | Keterangan |
|---|---|
| `projects` | Kode manual (master data), anggaran vs realisasi, manajer opsional dari hr-service |
| `tasks` | Nomor otomatis `TSK-YYYYMM-0001`, penanggung jawab opsional, prioritas, tenggat |
| `timesheets` | Jam kerja per proyek (opsional per tugas), tarif per jam, alur DRAFT → APPROVED → POSTED |

## Alur status

**Proyek** — `PLANNING → ACTIVE → COMPLETED`, dengan `ON_HOLD` di tengah dan
`CANCELLED` dari mana saja kecuali yang sudah selesai. Semua perpindahan lewat
endpoint transisi khusus (`/activate`, `/hold`, `/complete`, `/cancel`), tidak
ada PUT status — pola `delivery_orders` di fleet-service.

**Tugas** — SENGAJA berbeda: statusnya ikut `PUT /tasks/{id}` biasa (pola
`tickets` di ticketing-service), karena tugas berpindah status berkali-kali
sehari dan tidak punya efek samping lintas entitas. Satu-satunya otomatisasi
adalah `completed_at` yang terisi saat DONE dan dikosongkan lagi saat tugasnya
dibuka kembali.

**Timesheet** — `DRAFT → APPROVED → POSTED`, plus `REJECTED` dari DRAFT atau
APPROVED. POSTED adalah titik tidak bisa kembali: biayanya sudah masuk jurnal
finance-service, koreksinya lewat jurnal balik di modul Finance.

## Aturan lintas entitas

Ini yang membedakan modul ini dari CRUD biasa:

- **Proyek tidak bisa ditutup selagi ada tugas terbuka** (TODO/IN_PROGRESS).
  Tugas CANCELLED tidak menahan.
- **Proyek juga tidak bisa ditutup selagi ada timesheet DRAFT/APPROVED.**
  Alasannya akuntansi: timesheet APPROVED yang belum diposting adalah biaya
  yang sudah diakui tapi belum masuk GL. Kalau proyeknya keburu ditutup,
  `actual_cost` permanen understated dan tidak ada lagi jalur mempostingnya
  (post-cost menolak proyek non-ACTIVE).
- **Timesheet hanya bisa dicatat pada proyek ACTIVE** — lebih ketat daripada
  tugas, yang boleh disusun sejak PLANNING.
- **`actual_cost` tidak pernah diinput manusia.** Dia hanya bertambah saat
  posting timesheet ke GL berhasil, jadi selalu bisa direkonsiliasi dengan
  jurnal yang benar-benar ada di finance-service.

## Integrasi hr-service

`GET /employees/{id}` dipakai untuk memvalidasi + men-SNAPSHOT nama karyawan
saat penugasan manajer proyek, penanggung jawab tugas, dan pemilik timesheet.
Karyawan wajib milik company yang sama dan berstatus `ACTIVE`.

`model.Employee` hr-service TIDAK punya field `name` tunggal — namanya terpisah
`first_name`/`last_name`, dan response `GET /employees/{id}` berbentuk FLAT
(tidak dibungkus key `employee`). Keduanya dikonfirmasi dengan membaca kode
hr-service, dan stub test-nya sengaja meniru bentuk itu supaya bug client tidak
bisa lolos diam-diam.

Kalau `hourly_rate` tidak dikirim saat membuat timesheet, tarifnya diturunkan
dari `basic_salary / 173` — pembagi standar upah sejam di Indonesia (1/173 ×
upah sebulan, Pasal 61 PP 35/2021). Tarif yang dikirim eksplisit selalu menang,
karena tarif billing proyek sering berbeda dari gaji karyawan.

Arah panggilan SENGAJA project → hr, tidak pernah sebaliknya. Konsekuensinya
hr-service yang mati hanya memblokir PENUGASAN orang, bukan seluruh modul:
tugas backlog tanpa assignee dan semua perubahan status tetap jalan.

## Integrasi finance-service

`POST /projects/{id}/post-cost` menjadikan SEMUA timesheet APPROVED milik satu
proyek sebagai SATU journal entry (debit beban proyek, kredit hutang/akrual),
lalu menandainya POSTED dan menambahkan totalnya ke `actual_cost`. ID akun
dikirim pemanggil, pola identik `postPayrollRun` di hr-service — pemilihan akun
COA adalah keputusan akuntansi, bukan sesuatu yang boleh ditebak service ini.

Dua keputusan yang sengaja berbeda dari `deliverDeliveryOrder` di fleet-service:

1. Baris timesheet dikunci `SELECT ... FOR UPDATE` dan transaksinya **tetap
   terbuka selama panggilan HTTP** ke finance-service. Menahan lock selama
   panggilan jaringan bukan hal yang disukai, tapi di sini itulah yang mencegah
   dua post-cost bersamaan sama-sama membaca himpunan APPROVED yang sama lalu
   memposting biaya yang sama dua kali ke GL. Uang yang dobel masuk jurnal
   tidak bisa diperbaiki otomatis; latensi lock bisa.
2. Panggilan finance tetap dilakukan SEBELUM commit lokal (sama seperti
   fleet-service): kalau finance-service gagal, transaksi lokal di-rollback dan
   timesheet tetap APPROVED sehingga bisa dicoba lagi.

## Belum ada / batasan yang disengaja

- **Fact table dw-service + chart BI sudah ada** sejak 2026-08-16
  (`fact_project_timesheets` dengan grain satu baris per timesheet,
  endpoint `GET /analytics/project-cost-summary` di dw-service, dan chart
  "Biaya Proyek yang Sudah Diposting ke GL" di BI Dashboards). Yang belum:
  Materialized View untuk mempercepatnya, dan agregasi per KARYAWAN
  (endpoint yang ada meringkas per proyek).
- **Tidak ada dependensi antar tugas / Gantt / jalur kritis** — tugas berdiri
  sendiri, tidak ada `predecessor_id` maupun penjadwalan otomatis.
- **Tidak ada rekap jam per karyawan lintas proyek di service ini** —
  `GET /timesheets` bisa difilter per proyek dan status, tapi tidak ada endpoint
  agregasi. Datanya sudah tersedia untuk itu di dw-service lewat
  `fact_project_timesheets`; query analitiknya sendiri belum dibuat.
- **Anggaran tidak menahan apa pun** — `budget_amount` murni pembanding; posting
  biaya yang melewati anggaran tetap diterima (UI menandainya merah). Menolaknya
  akan memblokir pencatatan biaya yang sudah benar-benar terjadi.
- **Tidak ada approval berjenjang** — satu langkah APPROVED, tanpa peran
  approver terpisah; kontrol siapa yang boleh menyetujui ada di RBAC menu
  permission, bukan di alur data.
- Nomor tugas digenerate via `COUNT(*)+1` per company per periode
  (`nextSequence`, disalin dari fleet-service/sales-service) — cukup untuk
  dev/demo, bukan production-grade di bawah concurrency tinggi (batasan sama
  seperti seluruh modul lain yang memakai helper ini).
