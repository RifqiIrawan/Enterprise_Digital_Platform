# Sales Service / Penjualan

Status: **diimplementasikan (Fase 2)**, dengan test integrasi terhadap Postgres sungguhan.

Role terkait: `Sales`. Port `28087`, database `sales_service`, route gateway `/api/sales`.

Lingkup: customer, quotation, dan sales order. Quotation yang diterima dikonversi menjadi sales order; sales order yang dipenuhi mengeluarkan stok di warehouse-service dan bisa ditagihkan sebagai invoice AR di finance-service. Semua data dibatasi per `company_id` / `branch_id`.

## Endpoint

| Method | Path | Fungsi |
|---|---|---|
| GET/POST | `/customers` | Daftar / buat customer |
| PUT | `/customers/{id}` | Ubah customer |
| GET/POST | `/quotations` | Daftar / buat quotation |
| GET | `/quotations/{id}` | Detail |
| POST | `/quotations/{id}/send | accept | reject` | Alur quotation |
| POST | `/quotations/{id}/convert` | Konversi quotation yang diterima menjadi sales order |
| GET/POST | `/sales-orders` | Daftar / buat sales order |
| GET | `/sales-orders/{id}` | Detail |
| POST | `/sales-orders/{id}/confirm` | Konfirmasi |
| POST | `/sales-orders/{id}/fulfill` | Penuhi: stok keluar (OUT) di warehouse-service; `409` jika stok tidak cukup |
| POST | `/sales-orders/{id}/invoice` | Buat invoice AR di finance-service |

Di gateway, path ini diawali `/api/sales` dan izinnya dicek per menu (lihat `backend/services/api-gateway/internal/authz/policy.go`).

## Event Kafka

sales.customer.created/updated, sales.quotation.created/converted, sales.order.created/fulfilled/invoiced.

## Memanggil service lain

warehouse-service (stok keluar saat fulfill), finance-service (invoice AR).
