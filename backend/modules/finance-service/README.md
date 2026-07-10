# Finance Service

Status: **Fase 2 — diimplementasikan** (2026-07-10).

Role terkait: `Finance` (lihat `01_Vision_and_Roadmap.md`).

## Lingkup

- **Chart of Accounts** — master akun (`ASSET`/`LIABILITY`/`EQUITY`/`REVENUE`/`EXPENSE`), hierarkis via `parent_id`.
- **General Ledger** — journal entry header + baris double-entry (`journal_entries` + `journal_lines`). Draft → Posted (immutable setelah posted, koreksi lewat jurnal baru, bukan edit). Divalidasi balance (total debit = total credit) sebelum bisa disimpan.
- **Invoices (AR/AP)** — invoice header + baris (`invoices` + `invoice_lines`). Posting invoice otomatis membuat journal entry berimbang:
  - AR: debit akun piutang (`control_account_id`), credit akun revenue per baris (+ credit akun pajak bila ada).
  - AP: debit akun expense per baris (+ debit akun pajak bila ada), credit akun hutang (`control_account_id`).
- **AR/AP Summary** — agregat outstanding (`total_amount - paid_amount`) per tipe, dari invoice berstatus `POSTED`/`PARTIALLY_PAID`.

Belum ada master data Customer/Vendor terpisah — `partner_name` pada invoice masih teks bebas (akan dihubungkan ke Sales/Purchasing module saat modul itu ada).

## Endpoint (lihat kode di `internal/httpapi/` untuk detail request/response)

```
GET    /accounts?company_id=
POST   /accounts
PUT    /accounts/{id}

GET    /journal-entries?company_id=
POST   /journal-entries
GET    /journal-entries/{id}
POST   /journal-entries/{id}/post

GET    /invoices?company_id=&invoice_type=
POST   /invoices
GET    /invoices/{id}
POST   /invoices/{id}/post

GET    /ar-ap-summary?company_id=
```

Semua endpoint diakses lewat api-gateway dengan prefix `/api/finance/*` (lihat `backend/services/api-gateway/internal/gateway/gateway.go`).

## Menjalankan

```bash
# Database finance_service sudah dibuat (lihat infra/README.md untuk setup role `platform`)
cd backend/modules/finance-service
go run ./cmd/server        # migrasi jalan otomatis saat startup, listen di :8085
```

Frontend: `frontend/web/src/pages/finance/{ChartOfAccountsPage,JournalPage,InvoicesPage,ArApPage}.jsx`, routes `/finance/accounts`, `/finance/journal`, `/finance/invoices`, `/finance/ar-ap` (menu sudah ter-seed di rbac-service migration `007_seed_finance_coa_menu.sql` + `003_seed_business_menus.sql`).

## Belum ada / batasan yang disengaja

- Nomor invoice/jurnal digenerate via `COUNT(*)+1` per company per periode — cukup untuk dev/demo, bukan production-grade di bawah concurrency tinggi.
- Belum ada endpoint pencatatan pembayaran (`paid_amount` masih statis 0 setelah posting) — akan ditambah bareng modul Sales/Purchasing kalau ada kebutuhan match invoice-payment.
- Belum publish/consume event `finance.*` secara sistematis untuk dikonsumsi modul lain (baru dipublish ke audit-service). `POST /journal-entries` sudah didesain agar bisa dipanggil langsung HTTP dari service lain (mis. payroll-service nanti), pola ini mengikuti `financeClient.postJournalEntry()` di `20_Implementation_Guide.md`.
