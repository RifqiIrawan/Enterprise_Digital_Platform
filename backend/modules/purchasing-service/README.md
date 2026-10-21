# Purchasing Service / Pembelian

Status: **diimplementasikan (Fase 2)**, dengan test integrasi terhadap Postgres sungguhan.

Role terkait: `Purchasing`. Port `28088`, database `purchasing_service`, route gateway `/api/purchasing`.

Lingkup: supplier, purchase requisition (PR) dengan alur persetujuan, dan purchase order (PO). PO yang diterima menambah stok di warehouse-service dan bisa ditagihkan sebagai invoice AP di finance-service.

## Endpoint

| Method | Path | Fungsi |
|---|---|---|
| GET/POST | `/suppliers` | Daftar / buat supplier |
| PUT | `/suppliers/{id}` | Ubah supplier |
| GET/POST | `/requisitions` | Daftar / buat purchase requisition |
| GET | `/requisitions/{id}` | Detail |
| POST | `/requisitions/{id}/submit | approve | reject` | Alur persetujuan PR |
| POST | `/requisitions/{id}/convert` | Ubah PR yang disetujui menjadi PO |
| GET/POST | `/purchase-orders` | Daftar / buat PO |
| GET | `/purchase-orders/{id}` | Detail |
| POST | `/purchase-orders/{id}/confirm` | Konfirmasi PO |
| POST | `/purchase-orders/{id}/receive` | Terima barang: stok masuk (IN) di warehouse-service |
| POST | `/purchase-orders/{id}/invoice` | Buat invoice AP di finance-service |

Di gateway, path ini diawali `/api/purchasing` dan izinnya dicek per menu (lihat `backend/services/api-gateway/internal/authz/policy.go`).

## Event Kafka

purchasing.supplier.created/updated, purchasing.requisition.created/converted, purchasing.order.created/received/invoiced.

## Memanggil service lain

warehouse-service (stok masuk saat PO diterima), finance-service (invoice AP).
