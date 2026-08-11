# CRM Service

Status: **Fase 3 — diimplementasikan** (2026-08-08). Modul bisnis baru pertama di luar 9 modul Fase 2 awal, dibangun dari nol lewat sesi ini setelah dipilih dari 3 opsi roadmap tersisa (chart tambahan / modul bisnis baru / Silver-Gold Data Lake).

Role terkait: `CRM`.

## Lingkup

- **Leads** — prospek awal (belum tentu jadi Account/Contact/Opportunity). Status `NEW → CONTACTED → QUALIFIED → CONVERTED` (atau `UNQUALIFIED`, jalur mati).
- **Accounts** — organisasi yang berhubungan (`PROSPECT`/`CUSTOMER`/`PARTNER`/`OTHER`), master data seperti `customers` di sales-service — `account_code` diisi manual saat dibuat langsung, di-generate otomatis (`nextSequence`) hanya saat jadi efek samping konversi Lead.
- **Contacts** — orang di dalam sebuah Account (opsional, bisa berdiri sendiri tanpa Account).
- **Opportunities** — pipeline penjualan per Account, stage `PROSPECTING → QUALIFICATION → PROPOSAL → NEGOTIATION`, lalu terminal `WON`/`LOST` lewat endpoint khusus (`/win`, `/lose`) -- BUKAN lewat `PUT` biasa, supaya opportunity yang sudah closed benar-benar tidak bisa diedit lagi.
- **Activities** — log call/email/meeting/note/task, polimorfik terhadap salah satu dari 4 entitas di atas (`reference_type` + `reference_id`, divalidasi di level aplikasi karena Postgres tidak punya FK kondisional -- lihat komentar di `migrations/001_init.sql`).

**Lead conversion** (`POST /leads/{id}/convert`) adalah satu-satunya business logic lintas-entitas di modul ini: lead yang sudah `QUALIFIED` dikonversi jadi Account + Contact (primary) + Opportunity (stage `PROSPECTING`) sekaligus dalam satu transaksi, meniru pola `convertQuotation` di sales-service persis (leads/accounts/contacts/opportunities semuanya di database `crm_service` yang sama, jadi tidak perlu panggilan HTTP lintas service).

## Endpoint (lihat kode di `internal/httpapi/` untuk detail request/response)

```
GET    /leads?company_id=&branch_id=&status=
POST   /leads
PUT    /leads/{id}
POST   /leads/{id}/convert

GET    /accounts?company_id=&branch_id=
POST   /accounts
PUT    /accounts/{id}

GET    /contacts?company_id=&branch_id=&account_id=
POST   /contacts
PUT    /contacts/{id}

GET    /opportunities?company_id=&branch_id=&account_id=&stage=
POST   /opportunities
PUT    /opportunities/{id}
POST   /opportunities/{id}/win
POST   /opportunities/{id}/lose

GET    /activities?company_id=&branch_id=&reference_type=&reference_id=
POST   /activities
PUT    /activities/{id}
```

Semua endpoint diakses lewat api-gateway dengan prefix `/api/crm/*` (lihat `backend/services/api-gateway/internal/gateway/gateway.go`).

## Menjalankan

```bash
# Buat database dulu kalau belum ada
psql -U platform -h localhost -c "CREATE DATABASE crm_service;"

cd backend/modules/crm-service
go run ./cmd/server        # migrasi jalan otomatis saat startup, listen di :8096
```

Frontend: `frontend/web/src/pages/crm/{LeadsPage,AccountsPage,ContactsPage,OpportunitiesPage,ActivitiesPage}.jsx`, routes `/crm/leads`, `/crm/accounts`, `/crm/contacts`, `/crm/opportunities`, `/crm/activities` (menu ter-seed di rbac-service migration `011_seed_crm_menus.sql`).

## Belum ada / batasan yang disengaja

- **Belum ada Materialized View** di atas `fact_crm_opportunities` -- fact table (2026-08-11) dan endpoint analitik `GET /analytics/crm-pipeline-summary` + chart pipeline di BI Dashboards (2026-08-12) sudah ada, tapi endpoint itu masih query `FINAL` langsung; MV menyusul kalau terbukti perlu di bawah beban query nyata (pola bertahap yang sama seperti `finance-monthly-summary`).
- Nomor lead/opportunity/account (saat konversi) digenerate via `COUNT(*)+1` per company per periode (`nextSequence`, disalin dari sales-service) -- cukup untuk dev/demo, bukan production-grade di bawah concurrency tinggi (sama batasan yang sudah didokumentasikan di `finance-service/README.md`).
- Tidak ada integrasi otomatis ke Sales module saat Opportunity `WON` (mis. auto-create Sales Quotation) -- keputusan sengaja supaya scope tetap CRM standalone di fase ini, bukan oversight.
