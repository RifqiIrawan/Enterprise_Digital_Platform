# 04 — Database Design
## Enterprise Digital Platform (EDP)

---

## Database per Service

EDP menggunakan **Database-per-Service** pattern. Semua database dijalankan di satu PostgreSQL instance (Windows service `postgresql-x64-18`, port 5432), diakses oleh role `platform` (password `platform`).

| Service | Database | Catatan |
|---------|----------|---------|
| auth-service | `auth_service` | users, sessions |
| company-service | `company_service` | companies, branches, departments |
| rbac-service | `rbac_service` | roles, menus, permissions |
| audit-service | `audit_service` | audit_logs |
| finance-service | `finance_service` | accounts, journal_entries, journal_lines, invoices |
| hr-service | `hr_service` | employees, attendance_logs, payroll_runs, payroll_details |
| sales-service | `sales_service` | customers, quotations, quotation_lines, sales_orders, sales_order_lines |
| purchasing-service | `purchasing_service` | suppliers, requisitions, requisition_lines, purchase_orders, purchase_order_lines |
| warehouse-service | `warehouse_service` | products, warehouses, stock_movements, stock_balances, stock_transfers, stock_opnames |
| production-service | `production_service` | bill_of_materials, bom_lines, work_orders, work_order_lines |
| qc-service | `qc_service` | quality_standards, quality_inspections |
| asset-service | `asset_service` | assets, maintenance_schedules |
| iot-service | `iot_service` | devices, readings, alerts |
| dw-service | *(tidak punya DB Postgres sendiri — baca dari 9 DB lain, tulis ClickHouse)* | |

---

## Konvensi Schema

```sql
-- Primary key: UUID v4
id UUID PRIMARY KEY DEFAULT gen_random_uuid()

-- Multi-tenant: semua tabel bisnis punya company_id
company_id UUID NOT NULL

-- Branch scoping (opsional, NULL-inclusive filter)
branch_id UUID  -- boleh NULL untuk data company-wide

-- Timestamps standar
created_at TIMESTAMPTZ NOT NULL DEFAULT now()
updated_at TIMESTAMPTZ NOT NULL DEFAULT now()  -- kalau record bisa di-update

-- Status kolom: VARCHAR, bukan enum
status VARCHAR(20) NOT NULL DEFAULT 'DRAFT'
```

---

## Schema Kunci

### Finance
```sql
accounts (id, company_id, branch_id, account_code, account_name, account_type, is_active)
journal_entries (id, company_id, branch_id, entry_number, entry_date, period, reference_type, status, posted_at)
journal_lines (id, journal_id, account_id, debit_amount, credit_amount)
invoices (id, company_id, branch_id, invoice_number, invoice_type[AR/AP], partner_id, amount, status, journal_id)
```

### HR
```sql
employees (id, company_id, branch_id, employee_code, name, department, position, basic_salary, status)
attendance_logs (id, company_id, branch_id, employee_id, date, check_in, check_out, status)
payroll_runs (id, company_id, branch_id, period, status, posted_at, journal_id)
payroll_details (id, payroll_run_id, employee_id, basic_salary, gross_salary, total_deduction, net_salary, pph21, bpjs)
```

### Sales
```sql
customers (id, company_id, branch_id, customer_code, name, contact_person, phone, email)
quotations (id, company_id, branch_id, quotation_number, customer_id, status, valid_until)
quotation_lines (id, quotation_id, product_name, quantity, unit_price, amount)
sales_orders (id, company_id, branch_id, so_number, quotation_id, customer_id, status, invoice_id)
sales_order_lines (id, sales_order_id, product_name, quantity, unit_price, amount)
```

### Warehouse
```sql
products (id, company_id, sku, name, category, unit)  -- company-wide, no branch_id
warehouses (id, company_id, code, name, location)      -- company-wide, no branch_id
stock_movements (id, company_id, branch_id, warehouse_id, product_id, movement_type, quantity, reference_type, reference_id, movement_date)
stock_balances (id, warehouse_id, product_id, quantity)  -- materialized, no branch_id
stock_transfers (id, company_id, branch_id, from_warehouse_id, to_warehouse_id, status)
stock_opnames (id, company_id, branch_id, warehouse_id, status, posted_at)
```

### Alur Status Umum
```
Finance Journal:    DRAFT → POSTED
Quotation:          DRAFT → SENT → ACCEPTED/REJECTED → CONVERTED
Sales Order:        DRAFT → CONFIRMED → FULFILLED → INVOICED
Purchase Order:     DRAFT → CONFIRMED → RECEIVED → INVOICED
Work Order:         DRAFT → IN_PROGRESS → COMPLETED
Maintenance:        SCHEDULED → COMPLETED/CANCELLED
IoT Alert:          OPEN → ACKNOWLEDGED → RESOLVED
```

---

## Migrasi

Setiap service mengelola migrasi sendiri via embedded SQL files. Tabel `schema_migrations` melacak file mana yang sudah diapply. Penambahan kolom/tabel = file baru (bukan edit `001_init.sql`).

```
migrations/
├── embed.go           # //go:embed *.sql
├── 001_init.sql       # Schema awal
├── 002_add_branch.sql # Penambahan kolom branch_id (contoh)
└── 003_seed_menus.sql # Seed data RBAC
```
