# 17 — API Documentation
## Enterprise Digital Platform (EDP)

---

## Overview

Semua endpoint diakses lewat **api-gateway** (`http://localhost:8079`) dengan prefix `/api/{service}/`. JWT token wajib di header `Authorization: Bearer <token>` untuk semua endpoint kecuali `/api/auth/login` dan health checks.

**Base URL dev**: `http://localhost:8079`  
**Auth**: JWT custom (bukan Keycloak)

---

## Authentication

### Login
```http
POST /api/auth/login
Content-Type: application/json

{"email": "admin@edp.local", "password": "Admin@12345"}
```
Response: `{"token": "eyJ..."}` — simpan dan kirim di header `Authorization: Bearer <token>`.

### Endpoints Auth
| Method | Path | Fungsi |
|--------|------|--------|
| POST | `/api/auth/login` | Login, return JWT |
| GET | `/api/auth/me` | Info user dari token |
| POST | `/api/auth/users` | Buat user baru |
| GET | `/api/auth/users` | List users |

---

## Company & Branch

| Method | Path | Fungsi |
|--------|------|--------|
| GET | `/api/company/companies` | List companies |
| POST | `/api/company/companies` | Buat company |
| GET | `/api/company/branches?company_id=<uuid>` | List branches |
| POST | `/api/company/branches` | Buat branch |

---

## RBAC

| Method | Path | Fungsi |
|--------|------|--------|
| GET | `/api/rbac/menu-tree` | Tree menu sesuai role user (dipakai sidebar frontend) |
| GET | `/api/rbac/roles` | List roles |
| POST | `/api/rbac/roles` | Buat role |
| GET | `/api/rbac/roles/{id}/menus` | Permission matrix role |
| PUT | `/api/rbac/roles/{id}/menus` | Update permission |
| POST | `/api/rbac/users/{userId}/roles/{roleId}` | Assign role ke user |

---

## Finance

Query param `company_id` wajib. `branch_id` opsional (NULL-inclusive filter).

| Method | Path | Fungsi |
|--------|------|--------|
| GET | `/api/finance/accounts` | List CoA |
| POST | `/api/finance/accounts` | Buat akun |
| GET | `/api/finance/journal-entries` | List journal entries |
| POST | `/api/finance/journal-entries` | Buat journal entry (DRAFT) |
| POST | `/api/finance/journal-entries/{id}/post` | Post journal entry ke GL |
| GET | `/api/finance/invoices` | List invoices |
| POST | `/api/finance/invoices` | Buat invoice |
| POST | `/api/finance/invoices/{id}/post` | Post invoice + buat journal |
| GET | `/api/finance/ar-ap-summary` | Summary AR/AP per company |

---

## HR

| Method | Path | Fungsi |
|--------|------|--------|
| GET | `/api/hr/employees` | List karyawan |
| POST | `/api/hr/employees` | Buat karyawan |
| PUT | `/api/hr/employees/{id}` | Update karyawan |
| GET | `/api/hr/attendance` | List absensi |
| POST | `/api/hr/attendance` | Catat absensi |
| GET | `/api/hr/payroll-runs` | List payroll run |
| POST | `/api/hr/payroll-runs` | Buat payroll run (DRAFT) |
| POST | `/api/hr/payroll-runs/{id}/post` | Post payroll → buat journal GL |

---

## Sales

| Method | Path | Fungsi |
|--------|------|--------|
| GET/POST | `/api/sales/customers` | List/buat customer |
| GET/POST | `/api/sales/quotations` | List/buat quotation |
| POST | `/api/sales/quotations/{id}/send` | Kirim quotation |
| POST | `/api/sales/quotations/{id}/accept` | Terima → buat SO |
| GET/POST | `/api/sales/orders` | List/buat SO |
| POST | `/api/sales/orders/{id}/confirm` | Konfirmasi SO |
| POST | `/api/sales/orders/{id}/fulfill` | Fulfill → stock out |
| POST | `/api/sales/orders/{id}/invoice` | Invoice → buat AR |

---

## Purchasing

| Method | Path | Fungsi |
|--------|------|--------|
| GET/POST | `/api/purchasing/suppliers` | List/buat supplier |
| GET/POST | `/api/purchasing/requisitions` | List/buat PR |
| POST | `/api/purchasing/requisitions/{id}/submit` | Submit PR |
| POST | `/api/purchasing/requisitions/{id}/approve` | Approve → buat PO |
| GET/POST | `/api/purchasing/purchase-orders` | List/buat PO |
| POST | `/api/purchasing/purchase-orders/{id}/confirm` | Konfirmasi PO |
| POST | `/api/purchasing/purchase-orders/{id}/receive` | Terima barang → stock in |
| POST | `/api/purchasing/purchase-orders/{id}/invoice` | Invoice → buat AP |

---

## Warehouse

| Method | Path | Fungsi |
|--------|------|--------|
| GET/POST | `/api/warehouse/products` | List/buat produk |
| GET/POST | `/api/warehouse/warehouses` | List/buat gudang |
| GET | `/api/warehouse/stock-movements` | List mutasi stok |
| POST | `/api/warehouse/stock-movements/batch` | Batch stock movement (service-to-service) |
| GET | `/api/warehouse/stock-balances` | Saldo stok per warehouse |
| GET/POST | `/api/warehouse/stock-transfers` | List/buat transfer |
| POST | `/api/warehouse/stock-transfers/{id}/confirm` | Konfirmasi transfer → pindah stok |
| GET/POST | `/api/warehouse/stock-opnames` | List/buat opname |
| POST | `/api/warehouse/stock-opnames/{id}/post` | Post opname → adjust saldo |

---

## Production, QC, Asset

```
GET/POST /api/production/boms              # Bill of Material
GET/POST /api/production/work-orders       # Work Order
POST /api/production/work-orders/{id}/start     # Mulai WO
POST /api/production/work-orders/{id}/complete  # Selesai → konsumsi + produksi

GET/POST /api/qc/standards                 # Standar mutu
GET/POST /api/qc/inspections               # Inspeksi (result otomatis PASS/FAIL/PARTIAL)

GET/POST /api/asset/assets                 # Aset
GET/POST /api/asset/maintenance-schedules  # Jadwal maintenance
POST /api/asset/maintenance-schedules/{id}/complete  # Selesaikan
POST /api/asset/maintenance-schedules/{id}/cancel    # Batalkan
```

---

## IoT & AI-BI & DW

```
GET/POST /api/iot/devices          # Device management
GET      /api/iot/readings         # Sensor readings
GET/PUT  /api/iot/alerts           # Alert management + acknowledge/resolve

GET /api/aibi/dashboards/summary   # Aggregated KPIs dari 8 service
GET /api/aibi/forecasting/{metric} # Proyeksi tren linear
GET /api/aibi/anomaly-detection/{domain}  # Z-score outlier detection

GET  /api/dw/sync/status  # Status per fact table (row count + watermark)
POST /api/dw/sync         # Trigger manual batch ETL
```

---

## Audit Trail

```
GET /api/audit/logs?company_id=<uuid>&entity_type=sales_order&limit=50
```

Response:
```json
[{
  "id": "uuid",
  "event_type": "sales.order.fulfilled",
  "source_service": "sales-service",
  "actor_user_id": "uuid",
  "entity_type": "sales_order",
  "entity_id": "uuid",
  "payload": {...},
  "occurred_at": "2026-07-21T10:00:00Z"
}]
```

---

## Error Response Format

Semua service mengembalikan format yang sama:
```json
{"error": "pesan error yang jelas"}
```

HTTP status codes yang dipakai: 200, 201, 400 (validation), 401 (unauthenticated), 403 (unauthorized), 404 (not found), 409 (conflict — misal double-post), 500 (server error).
