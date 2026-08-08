# Ticketing Service

Status: **Fase 3 — diimplementasikan** (2026-08-09). Modul bisnis baru kedua di luar 9 modul Fase 2 awal (setelah CRM), dipilih dari 2 opsi roadmap tersisa (Ticketing vs E-Commerce).

Role terkait: `ticketing`.

## Lingkup

- **Ticket Categories** — kategori dukungan (mis. Billing, Technical, Feature Request), tabel master (bukan enum CHECK seperti field enum lain di platform ini) karena kategori support wajar berbeda-beda per company dan diedit user.
- **Tickets** — tiket helpdesk. `priority` (`LOW`/`MEDIUM`/`HIGH`/`URGENT`), `status` (`OPEN`/`IN_PROGRESS`/`RESOLVED`/`CLOSED`). Requester **standalone** (`requester_name`/`requester_email` teks bebas) — sengaja TIDAK di-link ke Account/Contact crm-service, supaya ticketing-service tetap independen (tidak ada pola soft-reference lintas service yang belum pernah dipakai di platform ini sebelumnya).
- **Ticket Comments** — komentar/timeline per tiket, FK fisik biasa ke `tickets` (bukan polimorfik seperti `crm.activities` — komentar cuma pernah menempel ke satu tiket). `is_internal` membedakan catatan internal vs balasan yang terlihat customer.

Status flow: `OPEN`/`IN_PROGRESS`/`RESOLVED` bisa dipindah bebas lewat `PUT` (workflow support wajar maju-mundur, mis. RESOLVED balik ke IN_PROGRESS kalau customer reply lagi) — beda dari transisi satu-arah dokumen lain di platform ini. `CLOSED` HANYA lewat endpoint terminal `POST /tickets/{id}/close` (PUT ditolak 409 setelah CLOSED, pola sama seperti Opportunity WON/LOST di crm-service), dan bisa dibuka kembali lewat `POST /tickets/{id}/reopen` (CLOSED → OPEN) — berbeda dari Opportunity yang genuinely terminal (keputusan bisnis final), tiket yang closed masih wajar dibuka lagi.

## Endpoint (lihat kode di `internal/httpapi/` untuk detail request/response)

```
GET    /categories?company_id=&branch_id=
POST   /categories
PUT    /categories/{id}

GET    /tickets?company_id=&branch_id=&status=&category_id=
POST   /tickets
PUT    /tickets/{id}
POST   /tickets/{id}/close
POST   /tickets/{id}/reopen

GET    /comments?company_id=&branch_id=&ticket_id=
POST   /comments
```

Semua endpoint diakses lewat api-gateway dengan prefix `/api/ticketing/*` (lihat `backend/services/api-gateway/internal/gateway/gateway.go`).

## Menjalankan

```bash
# Buat database dulu kalau belum ada
psql -U platform -h localhost -c "CREATE DATABASE ticketing_service;"

cd backend/modules/ticketing-service
go run ./cmd/server        # migrasi jalan otomatis saat startup, listen di :8097
```

Frontend: `frontend/web/src/pages/ticketing/{TicketCategoriesPage,TicketsPage,TicketCommentsPage}.jsx`, routes `/ticketing/categories`, `/ticketing/tickets`, `/ticketing/comments` (menu ter-seed di rbac-service migration `012_seed_ticketing_menus.sql`).

## Belum ada / batasan yang disengaja

- **Belum ada fact table dw-service** untuk Ticketing — tidak ada bukti kebutuhan BI/reporting saat modul ini dibangun.
- Nomor tiket digenerate via `COUNT(*)+1` per company per periode (`nextSequence`, disalin dari crm-service/sales-service) — cukup untuk dev/demo, bukan production-grade di bawah concurrency tinggi (batasan yang sama sudah didokumentasikan di `finance-service/README.md` dan `crm-service/README.md`).
- Requester TIDAK di-link ke CRM Account/Contact (keputusan sengaja, lihat "Lingkup" di atas) — kalau ternyata dibutuhkan nanti, pola soft-link (nullable UUID tanpa FK/validasi lintas service, karena beda database) bisa ditambahkan sebagai kolom opsional tanpa migrasi breaking.
