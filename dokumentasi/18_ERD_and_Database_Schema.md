# 18 — ERD & Database Schema
## Enterprise Digital Platform (EDP)

---

## Relasi Lintas Service

Karena database-per-service pattern, tidak ada foreign key lintas database. Relasi lintas service menggunakan UUID reference yang divalidasi di application layer:

```
auth_service.users.id
    ↑ (reference by UUID, no FK)
company_service.companies.id
    ↑
rbac_service.role_menu_permissions.role_id
    ↑
finance_service.journal_entries.company_id
    ↑
hr_service.payroll_runs.company_id    → finance_service (HTTP POST: journal entry)
    ↑
sales_service.sales_orders.company_id → finance_service (HTTP POST: invoice AR)
                                      → warehouse_service (HTTP POST: stock out)
    ↑
purchasing_service.purchase_orders.company_id → finance_service (HTTP POST: invoice AP)
                                              → warehouse_service (HTTP POST: stock in)
    ↑
production_service.work_orders.company_id → warehouse_service (HTTP POST: stock movement)
```

---

## Finance Service

```
accounts
├── id UUID PK
├── company_id UUID NOT NULL
├── branch_id UUID
├── account_code VARCHAR(20) UNIQUE per company
├── account_name VARCHAR(200)
├── account_type VARCHAR(20)  -- ASSET/LIABILITY/EQUITY/REVENUE/EXPENSE
└── is_active BOOLEAN DEFAULT true

journal_entries
├── id UUID PK
├── company_id UUID NOT NULL
├── branch_id UUID
├── entry_number VARCHAR(30) UNIQUE per company
├── entry_date DATE
├── period VARCHAR(7)  -- "2026-07"
├── reference_type VARCHAR(30)  -- MANUAL/PAYROLL/SALES_INVOICE/PURCHASE_INVOICE
├── status VARCHAR(20) DEFAULT 'DRAFT'  -- DRAFT/POSTED
├── posted_at TIMESTAMPTZ
└── created_at TIMESTAMPTZ

journal_lines  -- harus balance (sum debit = sum credit per journal entry)
├── id UUID PK
├── journal_id UUID → journal_entries.id
├── account_id UUID → accounts.id
├── debit_amount NUMERIC(18,2) DEFAULT 0
└── credit_amount NUMERIC(18,2) DEFAULT 0

invoices
├── id UUID PK
├── company_id UUID NOT NULL
├── branch_id UUID
├── invoice_number VARCHAR(30)
├── invoice_type VARCHAR(5)  -- AR/AP
├── partner_id UUID  -- customer_id (AR) atau supplier_id (AP)
├── amount NUMERIC(15,2)
├── tax_amount NUMERIC(15,2) DEFAULT 0
├── status VARCHAR(20) DEFAULT 'DRAFT'  -- DRAFT/POSTED
└── journal_id UUID → journal_entries.id  -- diisi saat POSTED
```

---

## HR Service

```
employees
├── id UUID PK
├── company_id UUID
├── branch_id UUID
├── employee_code VARCHAR(20)
├── name VARCHAR(200)
├── department VARCHAR(100)
├── position VARCHAR(100)
├── basic_salary NUMERIC(15,2)
└── status VARCHAR(20) DEFAULT 'ACTIVE'

attendance_logs
├── id UUID PK
├── company_id UUID
├── branch_id UUID
├── employee_id UUID → employees.id
├── date DATE
├── check_in TIMESTAMPTZ
├── check_out TIMESTAMPTZ
└── status VARCHAR(20)  -- PRESENT/ABSENT/LATE/HALF_DAY

payroll_runs
├── id UUID PK
├── company_id UUID
├── branch_id UUID
├── period VARCHAR(7)  -- "2026-07"
├── status VARCHAR(20) DEFAULT 'DRAFT'  -- DRAFT/POSTED
├── posted_at TIMESTAMPTZ
└── journal_id UUID  -- diisi saat POSTED (dari finance-service)

payroll_details  -- dihitung saat run dibuat
├── id UUID PK
├── payroll_run_id UUID → payroll_runs.id
├── employee_id UUID → employees.id
├── employee_name VARCHAR(200)  -- snapshot saat run
├── basic_salary NUMERIC(15,2)
├── gross_salary NUMERIC(15,2)  -- basic + allowance
├── total_pph21 NUMERIC(15,2)
├── total_bpjs NUMERIC(15,2)
├── total_deduction NUMERIC(15,2)
└── net_salary NUMERIC(15,2)  -- gross - total_deduction
```

---

## Warehouse Service

```
products (company-wide, bukan branch-scoped)
├── id UUID PK
├── company_id UUID
├── sku VARCHAR(50)
├── name VARCHAR(200)
├── category VARCHAR(100)
└── unit VARCHAR(20)

warehouses (company-wide)
├── id UUID PK
├── company_id UUID
├── code VARCHAR(20)
├── name VARCHAR(200)
└── location VARCHAR(200)

stock_movements (append-only, tidak pernah UPDATE)
├── id UUID PK
├── company_id UUID
├── branch_id UUID
├── warehouse_id UUID → warehouses.id
├── product_id UUID → products.id
├── movement_type VARCHAR(10)  -- IN/OUT
├── quantity NUMERIC(15,2)
├── reference_type VARCHAR(30)  -- PURCHASE_ORDER/SALES_ORDER/TRANSFER/OPNAME/WORK_ORDER/MANUAL
├── reference_id UUID  -- id dari entitas referensi
└── movement_date DATE

stock_balances (materialized, di-update transaksional bersamaan dengan stock_movements)
├── id UUID PK
├── warehouse_id UUID → warehouses.id
├── product_id UUID → products.id
└── quantity NUMERIC(15,2)  -- saldo saat ini (bisa negatif kalau validasi longgar)
```

---

## IoT Service

```
devices
├── id UUID PK
├── company_id UUID
├── branch_id UUID
├── device_code VARCHAR(30)
├── device_type VARCHAR(20)  -- TEMPERATURE/HUMIDITY/PRESSURE/VIBRATION/ENERGY
├── location VARCHAR(200)
├── threshold_min NUMERIC(10,4)
├── threshold_max NUMERIC(10,4)
└── status VARCHAR(20) DEFAULT 'ACTIVE'  -- ACTIVE/INACTIVE/MAINTENANCE

readings (time-series, insert-only)
├── id UUID PK
├── device_id UUID → devices.id
├── company_id UUID
├── branch_id UUID
├── reading_type VARCHAR(20)
├── value_numeric NUMERIC(15,4)
├── value_text VARCHAR(200)
└── recorded_at TIMESTAMPTZ

alerts
├── id UUID PK
├── device_id UUID → devices.id
├── company_id UUID
├── reading_id UUID → readings.id
├── message TEXT
├── severity VARCHAR(10)  -- LOW/MEDIUM/HIGH/CRITICAL
├── status VARCHAR(20) DEFAULT 'OPEN'  -- OPEN/ACKNOWLEDGED/RESOLVED
├── acknowledged_by UUID
├── acknowledged_at TIMESTAMPTZ
├── resolved_by UUID
└── resolved_at TIMESTAMPTZ
```

---

## ClickHouse (dw database)

Semua tabel menggunakan `ReplacingMergeTree(synced_at)` — tidak ada relasi antar tabel (denormalized).

```sql
-- Contoh: fact_finance_journal_lines
CREATE TABLE fact_finance_journal_lines (
    line_id UUID,
    journal_id UUID,
    company_id UUID,
    branch_id Nullable(UUID),
    entry_number String,
    entry_date Date,
    period String,
    reference_type String,
    entry_status String,
    account_id UUID,
    account_code String,
    account_name String,
    account_type String,
    debit_amount Decimal(18,2),
    credit_amount Decimal(18,2),
    posted_at Nullable(DateTime),
    synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY toYYYYMM(entry_date)
ORDER BY (company_id, line_id);
```

Semua 12 fact table mengikuti pola yang sama: data denormalized + ORDER BY (company_id, {entity_id}).
