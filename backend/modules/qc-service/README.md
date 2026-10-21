# QC Service / Quality Control

Status: **diimplementasikan (Fase 2)**, dengan test integrasi terhadap Postgres sungguhan.

Role terkait: `QC`. Port `28091`, database `qc_service`, route gateway `/api/qc`.

Lingkup: standar mutu per produk dan inspeksi kualitas. Hasil inspeksi (PASS / FAIL / PARTIAL) dihitung otomatis dari jumlah lolos dan gagal, dan inspeksi bisa dikaitkan ke PO atau work order.

## Endpoint

| Method | Path | Fungsi |
|---|---|---|
| GET/POST | `/standards` | Daftar / buat standar mutu |
| PUT | `/standards/{id}` | Ubah standar mutu |
| GET/POST | `/inspections` | Daftar / buat inspeksi (result dihitung otomatis) |
| GET | `/inspections/{id}` | Detail inspeksi |

Di gateway, path ini diawali `/api/qc` dan izinnya dicek per menu (lihat `backend/services/api-gateway/internal/authz/policy.go`).

## Event Kafka

qc.standard.created/updated, qc.inspection.created.

## Memanggil service lain

tidak ada (mandiri).
