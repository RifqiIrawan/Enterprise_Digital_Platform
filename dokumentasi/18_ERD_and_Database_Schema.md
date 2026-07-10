# 18 — ERD & Database Schema (Cross-Domain)
## Enterprise Data Center Simulator (EDCS)

---

## 🗺️ Cross-Domain Entity Relationships

Meskipun setiap service memiliki database sendiri (database-per-service), entitas berikut saling merujuk via **UUID natural key** (bukan foreign key cross-database). Integrasi dijaga melalui **event sourcing** dan **API calls**.

---

## 📐 Domain Entity Map

```
┌─────────────────────────────────────────────────────────────────────┐
│                        CROSS-DOMAIN ERD                             │
│                                                                     │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────────┐   │
│  │  ERP CORE    │     │    HRIS      │     │      CRM         │   │
│  │              │     │              │     │                  │   │
│  │ business_unit│◄────│ department   │     │ contact          │   │
│  │ cost_center  │◄────│ employee     │────►│ opportunity      │   │
│  │ account      │     │ salary       │     │ ticket           │   │
│  │ product      │     │ attendance   │     │ campaign         │   │
│  │ customer_mst │────►│ payroll      │     │                  │   │
│  │ vendor_mst   │     └──────────────┘     └────────┬─────────┘   │
│  └──────┬───────┘                                   │             │
│         │                                           │             │
│  ┌──────▼───────┐     ┌──────────────┐     ┌───────▼──────────┐  │
│  │   FINANCE    │     │     WMS      │     │      SALES       │  │
│  │              │     │              │     │                  │  │
│  │ journal_entry│◄────│ stock_movmnt │     │ sales_order      │──►
│  │ ap_invoice   │◄────│ stock_level  │◄────│ order_line       │  │
│  │ ar_invoice   │     │ warehouse    │     │ price_list       │  │
│  │ payment      │     │ location     │     │ discount         │  │
│  │ budget       │     │ receipt      │     └──────────────────┘  │
│  └──────────────┘     └──────┬───────┘                           │
│                              │                                    │
│  ┌──────────────┐     ┌──────▼───────┐     ┌──────────────────┐  │
│  │ PROCUREMENT  │     │     MES      │     │   ASSET MGMT     │  │
│  │              │     │              │     │                  │  │
│  │ purchase_req │     │ work_order   │     │ asset            │  │
│  │ rfq          │     │ bom          │     │ maintenance      │  │
│  │ purchase_ord │────►│ routing      │     │ depreciation     │  │
│  │ vendor_eval  │     │ quality      │◄────│ asset_movement   │  │
│  │ contract     │     │ oee_tracking │     └──────────────────┘  │
│  └──────────────┘     └──────────────┘                           │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │                      IOT PLATFORM                            │  │
│  │  device ──────────────────────────────────► asset (ASSET DB) │  │
│  │  sensor_reading ──── (Kafka) ──► mes.oee_tracking            │  │
│  │  device_alert ──────(Kafka) ──► notification                 │  │
│  └──────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 📋 Complete Schema Reference

### ERP Core (erp_db)

```sql
-- ========================================
-- MASTER DATA
-- ========================================

CREATE TABLE business_units (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code          VARCHAR(20) UNIQUE NOT NULL,
  name          VARCHAR(100) NOT NULL,
  parent_id     UUID REFERENCES business_units(id),
  is_active     BOOLEAN DEFAULT TRUE,
  created_at    TIMESTAMPTZ DEFAULT NOW(),
  updated_at    TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE cost_centers (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code              VARCHAR(20) UNIQUE NOT NULL,
  name              VARCHAR(100) NOT NULL,
  business_unit_id  UUID REFERENCES business_units(id),
  manager_employee_id UUID,           -- ref ke hris.employees.id
  budget_annual     NUMERIC(15,2),
  currency          CHAR(3) DEFAULT 'IDR',
  is_active         BOOLEAN DEFAULT TRUE,
  created_at        TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE accounts (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_code    VARCHAR(20) UNIQUE NOT NULL,
  account_name    VARCHAR(200) NOT NULL,
  account_type    VARCHAR(20) NOT NULL
                  CHECK (account_type IN ('ASSET','LIABILITY','EQUITY','REVENUE','EXPENSE')),
  account_subtype VARCHAR(50),
  parent_id       UUID REFERENCES accounts(id),
  level           SMALLINT DEFAULT 1,
  is_posting      BOOLEAN DEFAULT TRUE,
  currency        CHAR(3) DEFAULT 'IDR',
  normal_balance  CHAR(1) CHECK (normal_balance IN ('D','C')),
  is_active       BOOLEAN DEFAULT TRUE
);

CREATE TABLE products (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  product_code    VARCHAR(50) UNIQUE NOT NULL,
  product_name    VARCHAR(200) NOT NULL,
  barcode         VARCHAR(100),
  category_id     UUID REFERENCES product_categories(id),
  uom_id          UUID REFERENCES units_of_measure(id),
  standard_cost   NUMERIC(15,4),
  list_price      NUMERIC(15,2),
  reorder_point   NUMERIC(12,3),
  reorder_qty     NUMERIC(12,3),
  lead_time_days  SMALLINT,
  weight_kg       NUMERIC(8,4),
  is_purchasable  BOOLEAN DEFAULT TRUE,
  is_saleable     BOOLEAN DEFAULT TRUE,
  is_manufactured BOOLEAN DEFAULT FALSE,
  is_active       BOOLEAN DEFAULT TRUE,
  created_at      TIMESTAMPTZ DEFAULT NOW(),
  updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE product_categories (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code        VARCHAR(20) UNIQUE NOT NULL,
  name        VARCHAR(100) NOT NULL,
  parent_id   UUID REFERENCES product_categories(id),
  level       SMALLINT DEFAULT 1
);

CREATE TABLE units_of_measure (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code        VARCHAR(20) UNIQUE NOT NULL,
  name        VARCHAR(50) NOT NULL,
  uom_type    VARCHAR(20) CHECK (uom_type IN ('UNIT','WEIGHT','VOLUME','LENGTH','TIME'))
);

CREATE TABLE customers (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_code   VARCHAR(20) UNIQUE NOT NULL,
  customer_name   VARCHAR(200) NOT NULL,
  customer_type   VARCHAR(20) CHECK (customer_type IN ('B2B','B2C','GOVERNMENT')),
  tax_id          VARCHAR(50),
  industry        VARCHAR(100),
  segment         VARCHAR(50),
  credit_limit    NUMERIC(15,2),
  payment_terms   VARCHAR(50),
  currency        CHAR(3) DEFAULT 'IDR',
  billing_address JSONB,
  is_active       BOOLEAN DEFAULT TRUE
);

CREATE TABLE vendors (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  vendor_code     VARCHAR(20) UNIQUE NOT NULL,
  vendor_name     VARCHAR(200) NOT NULL,
  vendor_type     VARCHAR(20),
  tax_id          VARCHAR(50),
  payment_terms   VARCHAR(50),
  currency        CHAR(3) DEFAULT 'IDR',
  credit_limit    NUMERIC(15,2),
  lead_time_days  SMALLINT,
  address         JSONB,
  bank_accounts   JSONB,  -- [{bank, account_no, account_name}]
  is_active       BOOLEAN DEFAULT TRUE
);
```

### HRIS (hris_db)

```sql
CREATE TABLE departments (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code          VARCHAR(20) UNIQUE NOT NULL,
  name          VARCHAR(100) NOT NULL,
  parent_id     UUID REFERENCES departments(id),
  head_id       UUID,    -- ref ke employees.id (self-ref setelah employee ada)
  cost_center_id UUID,  -- ref ke erp.cost_centers.id
  is_active     BOOLEAN DEFAULT TRUE
);

CREATE TABLE positions (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code          VARCHAR(20) UNIQUE NOT NULL,
  name          VARCHAR(100) NOT NULL,
  department_id UUID REFERENCES departments(id),
  level         VARCHAR(30),
  grade         VARCHAR(10),
  min_salary    NUMERIC(15,2),
  max_salary    NUMERIC(15,2),
  headcount     SMALLINT DEFAULT 1
);

CREATE TABLE employees (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_code       VARCHAR(20) UNIQUE NOT NULL,
  first_name          VARCHAR(100) NOT NULL,
  last_name           VARCHAR(100),
  email               VARCHAR(200) UNIQUE NOT NULL,
  phone               VARCHAR(20),
  nik                 CHAR(16) UNIQUE,
  gender              CHAR(1) CHECK (gender IN ('M','F')),
  birth_date          DATE,
  birth_place         VARCHAR(100),
  marital_status      VARCHAR(20),
  religion            VARCHAR(30),
  address             TEXT,
  hire_date           DATE NOT NULL,
  probation_end_date  DATE,
  termination_date    DATE,
  termination_reason  VARCHAR(50),
  status              VARCHAR(20) DEFAULT 'ACTIVE'
                      CHECK (status IN ('ACTIVE','PROBATION','INACTIVE','TERMINATED','ON_LEAVE')),
  department_id       UUID REFERENCES departments(id),
  position_id         UUID REFERENCES positions(id),
  manager_id          UUID REFERENCES employees(id),
  employment_type     VARCHAR(20)
                      CHECK (employment_type IN ('PERMANENT','CONTRACT','INTERN','OUTSOURCE')),
  work_location       VARCHAR(100),
  work_type           VARCHAR(20) CHECK (work_type IN ('ONSITE','REMOTE','HYBRID')),
  bpjs_kesehatan_no   VARCHAR(30),
  bpjs_ketenagakerjaan_no VARCHAR(30),
  npwp                VARCHAR(20),
  bank_account        JSONB,
  photo_url           TEXT,
  is_active           BOOLEAN DEFAULT TRUE,
  created_at          TIMESTAMPTZ DEFAULT NOW(),
  updated_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE salary_components (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code            VARCHAR(20) UNIQUE NOT NULL,
  name            VARCHAR(100) NOT NULL,
  component_type  VARCHAR(20)
                  CHECK (component_type IN ('BASIC','ALLOWANCE','DEDUCTION','BENEFIT','BONUS')),
  is_taxable      BOOLEAN DEFAULT TRUE,
  is_fixed        BOOLEAN DEFAULT TRUE,
  formula         TEXT,
  is_active       BOOLEAN DEFAULT TRUE
);

CREATE TABLE employee_salaries (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id     UUID REFERENCES employees(id),
  component_id    UUID REFERENCES salary_components(id),
  amount          NUMERIC(15,2) NOT NULL,
  effective_date  DATE NOT NULL,
  end_date        DATE,
  UNIQUE (employee_id, component_id, effective_date)
);

CREATE TABLE leave_types (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code            VARCHAR(20) UNIQUE NOT NULL,
  name            VARCHAR(100) NOT NULL,
  annual_quota    NUMERIC(4,1),
  carry_forward   BOOLEAN DEFAULT FALSE,
  max_carry_days  NUMERIC(4,1),
  is_paid         BOOLEAN DEFAULT TRUE,
  requires_doc    BOOLEAN DEFAULT FALSE
);

CREATE TABLE leave_balances (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id     UUID REFERENCES employees(id),
  leave_type_id   UUID REFERENCES leave_types(id),
  year            SMALLINT NOT NULL,
  quota           NUMERIC(5,1),
  used            NUMERIC(5,1) DEFAULT 0,
  carried_forward NUMERIC(5,1) DEFAULT 0,
  balance         NUMERIC(5,1) GENERATED ALWAYS AS (quota + carried_forward - used) STORED,
  UNIQUE (employee_id, leave_type_id, year)
);

CREATE TABLE payroll_runs (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  period          VARCHAR(7) NOT NULL,   -- 'YYYY-MM'
  run_type        VARCHAR(20) DEFAULT 'REGULAR'
                  CHECK (run_type IN ('REGULAR','CORRECTION','BONUS','THR')),
  status          VARCHAR(20) DEFAULT 'DRAFT'
                  CHECK (status IN ('DRAFT','PROCESSING','COMPLETED','CANCELLED')),
  total_employees SMALLINT,
  total_gross     NUMERIC(18,2),
  total_deduction NUMERIC(18,2),
  total_net       NUMERIC(18,2),
  processed_by    UUID REFERENCES employees(id),
  processed_at    TIMESTAMPTZ,
  approved_by     UUID REFERENCES employees(id),
  approved_at     TIMESTAMPTZ,
  UNIQUE (period, run_type)
);

CREATE TABLE payroll_details (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  payroll_run_id      UUID REFERENCES payroll_runs(id),
  employee_id         UUID REFERENCES employees(id),
  basic_salary        NUMERIC(15,2),
  total_allowance     NUMERIC(15,2),
  gross_salary        NUMERIC(15,2),
  pph21               NUMERIC(15,2),
  bpjs_kesehatan_emp  NUMERIC(15,2),
  bpjs_tk_jht_emp     NUMERIC(15,2),
  bpjs_tk_jp_emp      NUMERIC(15,2),
  other_deductions    NUMERIC(15,2),
  total_deduction     NUMERIC(15,2),
  net_salary          NUMERIC(15,2),
  components          JSONB,  -- Detail per komponen
  attendance_days     SMALLINT,
  overtime_hours      NUMERIC(6,2),
  overtime_amount     NUMERIC(15,2)
);
```

### MES (mes_db)

```sql
CREATE TABLE bills_of_materials (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  product_id      UUID NOT NULL,   -- ref ke erp.products.id
  bom_code        VARCHAR(30) UNIQUE NOT NULL,
  bom_name        VARCHAR(100),
  quantity         NUMERIC(12,3) NOT NULL DEFAULT 1,
  uom_id          UUID,
  effective_date  DATE,
  expiry_date     DATE,
  is_active       BOOLEAN DEFAULT TRUE
);

CREATE TABLE bom_lines (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bom_id          UUID REFERENCES bills_of_materials(id),
  component_id    UUID NOT NULL,   -- ref ke erp.products.id
  quantity        NUMERIC(12,4) NOT NULL,
  uom_id          UUID,
  scrap_pct       NUMERIC(5,2) DEFAULT 0,
  is_phantom      BOOLEAN DEFAULT FALSE,
  line_number     SMALLINT
);

CREATE TABLE work_centers (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code            VARCHAR(20) UNIQUE NOT NULL,
  name            VARCHAR(100) NOT NULL,
  capacity_per_hour NUMERIC(10,2),
  cost_per_hour   NUMERIC(10,2),
  efficiency_pct  NUMERIC(5,2) DEFAULT 100,
  is_active       BOOLEAN DEFAULT TRUE
);

CREATE TABLE work_orders (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  wo_number       VARCHAR(30) UNIQUE NOT NULL,
  product_id      UUID NOT NULL,
  bom_id          UUID REFERENCES bills_of_materials(id),
  planned_qty     NUMERIC(12,3) NOT NULL,
  actual_qty      NUMERIC(12,3) DEFAULT 0,
  scrap_qty       NUMERIC(12,3) DEFAULT 0,
  planned_start   TIMESTAMPTZ,
  planned_end     TIMESTAMPTZ,
  actual_start    TIMESTAMPTZ,
  actual_end      TIMESTAMPTZ,
  status          VARCHAR(20) DEFAULT 'DRAFT'
                  CHECK (status IN ('DRAFT','RELEASED','IN_PROGRESS','COMPLETED','CANCELLED')),
  priority        SMALLINT DEFAULT 5,
  sales_order_id  UUID,   -- ref ke sales.sales_orders.id
  created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE oee_records (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  work_center_id    UUID REFERENCES work_centers(id),
  shift             SMALLINT,
  record_date       DATE NOT NULL,
  planned_hours     NUMERIC(6,2),
  actual_hours      NUMERIC(6,2),
  downtime_hours    NUMERIC(6,2),
  planned_rate      NUMERIC(10,2),
  actual_rate       NUMERIC(10,2),
  good_output       NUMERIC(12,3),
  total_output      NUMERIC(12,3),
  availability_pct  NUMERIC(7,4),
  performance_pct   NUMERIC(7,4),
  quality_pct       NUMERIC(7,4),
  oee_pct           NUMERIC(7,4)
    GENERATED ALWAYS AS (availability_pct * performance_pct * quality_pct / 10000) STORED
);
```

---

## 🔗 Cross-Database Reference Matrix

| Field | Source DB | Referenced By |
|-------|-----------|---------------|
| `erp.products.id` | ERP | WMS, MES, Sales, Finance |
| `erp.customers.id` | ERP | CRM, Sales, Finance |
| `erp.vendors.id` | ERP | Procurement, Finance |
| `erp.cost_centers.id` | ERP | HRIS, Finance |
| `hris.employees.id` | HRIS | CRM, Sales, Finance, Procurement |
| `hris.departments.id` | HRIS | Finance (budget) |
| `sales.sales_orders.id` | Sales | WMS, MES, Finance |
| `procurement.po.id` | Procurement | WMS (receipt), Finance (AP) |
| `mes.work_orders.id` | MES | WMS (issue), Finance (WIP) |
| `asset.assets.id` | Asset | IoT, Finance, MES |

**Catatan:** Semua referensi ini adalah **soft references** (UUID string) — bukan foreign key. Konsistensi dijaga melalui event sourcing dan periodic reconciliation job.
