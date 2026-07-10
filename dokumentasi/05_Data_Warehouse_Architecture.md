# 05 — Data Warehouse Architecture
## Enterprise Data Center Simulator (EDCS)

---

## 🏛️ Overview

EDCS Data Warehouse menggunakan **Kimball Dimensional Modeling** (Bottom-Up) dengan **ClickHouse** sebagai engine OLAP berkinerja tinggi. Semua data dimodelkan dalam skema **Star Schema** dan dikelola via **dbt (data build tool)**.

---

## 🔷 Arsitektur Lapisan

```
Operational DBs (PostgreSQL, TimescaleDB)
    │
    ▼ (CDC via Debezium / ELT via Airbyte)
┌─────────────────────────────────────┐
│          STAGING LAYER              │
│  Raw copies dari source systems     │
│  stg_hris_employees                 │
│  stg_sales_orders                   │
│  stg_finance_journals               │
│  (dbt source freshness check)       │
└─────────────────┬───────────────────┘
                  │ (dbt transformations)
┌─────────────────▼───────────────────┐
│         INTEGRATION LAYER           │
│  Cleaned, deduplicated, enriched    │
│  int_employees                      │
│  int_sales_orders                   │
│  int_financial_transactions         │
└─────────────────┬───────────────────┘
                  │ (dbt models)
┌─────────────────▼───────────────────┐
│         PRESENTATION LAYER          │
│  Dimensions & Facts (Star Schema)   │
│  dim_employee, dim_customer         │
│  fact_sales, fact_payroll           │
│  (Accessible by BI tools)           │
└─────────────────────────────────────┘
```

---

## 📐 Dimension Tables

### dim_date
```sql
CREATE TABLE dim_date (
  date_key        INT PRIMARY KEY,  -- YYYYMMDD
  full_date       DATE,
  day_of_week     TINYINT,
  day_name        VARCHAR(10),
  day_of_month    TINYINT,
  day_of_year     SMALLINT,
  week_of_year    TINYINT,
  month_number    TINYINT,
  month_name      VARCHAR(10),
  quarter         TINYINT,
  year            SMALLINT,
  is_weekend      BOOLEAN,
  is_holiday      BOOLEAN,
  fiscal_period   VARCHAR(7),       -- 'YYYY-MM'
  fiscal_year     SMALLINT,
  fiscal_quarter  TINYINT
);
-- Pre-populated: 2020-01-01 sampai 2030-12-31
```

### dim_employee (SCD Type 2)
```sql
CREATE TABLE dim_employee (
  employee_sk       BIGINT PRIMARY KEY AUTO_INCREMENT, -- surrogate key
  employee_id       UUID NOT NULL,    -- natural key dari HRIS
  employee_code     VARCHAR(20),
  full_name         VARCHAR(200),
  email             VARCHAR(200),
  gender            CHAR(1),
  birth_date        DATE,
  hire_date         DATE,
  department_name   VARCHAR(100),
  department_code   VARCHAR(20),
  position_name     VARCHAR(100),
  position_level    VARCHAR(50),
  manager_name      VARCHAR(200),
  employment_type   VARCHAR(20),
  work_location     VARCHAR(50),
  cost_center_code  VARCHAR(20),
  -- SCD Type 2 fields
  effective_from    DATE NOT NULL,
  effective_to      DATE,
  is_current        BOOLEAN DEFAULT TRUE
);
```

### dim_customer
```sql
CREATE TABLE dim_customer (
  customer_sk       BIGINT PRIMARY KEY AUTO_INCREMENT,
  customer_id       UUID NOT NULL,
  customer_code     VARCHAR(20),
  customer_name     VARCHAR(200),
  customer_type     VARCHAR(50),
  industry          VARCHAR(100),
  segment           VARCHAR(50),
  country           VARCHAR(100),
  city              VARCHAR(100),
  sales_rep_name    VARCHAR(200),
  -- SCD Type 2
  effective_from    DATE NOT NULL,
  effective_to      DATE,
  is_current        BOOLEAN DEFAULT TRUE
);
```

### dim_product
```sql
CREATE TABLE dim_product (
  product_sk        BIGINT PRIMARY KEY AUTO_INCREMENT,
  product_id        UUID NOT NULL,
  product_code      VARCHAR(50),
  product_name      VARCHAR(200),
  category_l1       VARCHAR(100),
  category_l2       VARCHAR(100),
  category_l3       VARCHAR(100),
  unit_of_measure   VARCHAR(20),
  standard_cost     NUMERIC(15,4),
  list_price        NUMERIC(15,2),
  brand             VARCHAR(100),
  is_active         BOOLEAN,
  effective_from    DATE NOT NULL,
  effective_to      DATE,
  is_current        BOOLEAN DEFAULT TRUE
);
```

### dim_vendor
```sql
CREATE TABLE dim_vendor (
  vendor_sk         BIGINT PRIMARY KEY AUTO_INCREMENT,
  vendor_id         UUID NOT NULL,
  vendor_code       VARCHAR(20),
  vendor_name       VARCHAR(200),
  vendor_type       VARCHAR(50),
  country           VARCHAR(100),
  city              VARCHAR(100),
  payment_terms     VARCHAR(50),
  credit_limit      NUMERIC(15,2),
  currency          CHAR(3),
  effective_from    DATE NOT NULL,
  effective_to      DATE,
  is_current        BOOLEAN DEFAULT TRUE
);
```

---

## 📊 Fact Tables

### fact_sales
```sql
CREATE TABLE fact_sales (
  sales_key         BIGINT PRIMARY KEY AUTO_INCREMENT,
  -- Foreign Keys
  date_key          INT REFERENCES dim_date(date_key),
  customer_sk       BIGINT REFERENCES dim_customer(customer_sk),
  product_sk        BIGINT REFERENCES dim_product(product_sk),
  employee_sk       BIGINT REFERENCES dim_employee(employee_sk), -- sales rep
  -- Degenerate Dimensions
  order_number      VARCHAR(30),
  invoice_number    VARCHAR(30),
  -- Measures
  order_qty         NUMERIC(12,3),
  unit_price        NUMERIC(15,4),
  discount_pct      NUMERIC(5,2),
  gross_amount      NUMERIC(15,2),
  discount_amount   NUMERIC(15,2),
  net_amount        NUMERIC(15,2),
  tax_amount        NUMERIC(15,2),
  total_amount      NUMERIC(15,2),
  cogs_amount       NUMERIC(15,2),
  gross_margin      NUMERIC(15,2),
  gross_margin_pct  NUMERIC(7,4),
  -- Date dimensions (denormalized untuk performa)
  order_date_key    INT,
  delivery_date_key INT
);
```

### fact_inventory
```sql
CREATE TABLE fact_inventory (
  inventory_key     BIGINT PRIMARY KEY AUTO_INCREMENT,
  date_key          INT REFERENCES dim_date(date_key),
  product_sk        BIGINT REFERENCES dim_product(product_sk),
  -- Degenerate
  warehouse_code    VARCHAR(20),
  location_code     VARCHAR(50),
  -- Measures (snapshot harian)
  qty_on_hand       NUMERIC(12,3),
  qty_reserved      NUMERIC(12,3),
  qty_available     NUMERIC(12,3),
  qty_in_transit    NUMERIC(12,3),
  unit_cost         NUMERIC(15,4),
  inventory_value   NUMERIC(15,2),
  days_of_supply    NUMERIC(8,2),
  turnover_rate     NUMERIC(8,4)
);
```

### fact_payroll
```sql
CREATE TABLE fact_payroll (
  payroll_key       BIGINT PRIMARY KEY AUTO_INCREMENT,
  date_key          INT REFERENCES dim_date(date_key),
  employee_sk       BIGINT REFERENCES dim_employee(employee_sk),
  -- Degenerate
  payroll_period    VARCHAR(7),    -- 'YYYY-MM'
  payroll_run_id    UUID,
  -- Measures
  basic_salary      NUMERIC(15,2),
  total_allowance   NUMERIC(15,2),
  gross_salary      NUMERIC(15,2),
  pph21_amount      NUMERIC(15,2),
  bpjs_kes_employee NUMERIC(15,2),
  bpjs_tk_employee  NUMERIC(15,2),
  total_deduction   NUMERIC(15,2),
  net_salary        NUMERIC(15,2),
  overtime_hours    NUMERIC(6,2),
  overtime_amount   NUMERIC(15,2),
  attendance_days   SMALLINT,
  leave_days        SMALLINT
);
```

### fact_production
```sql
CREATE TABLE fact_production (
  production_key    BIGINT PRIMARY KEY AUTO_INCREMENT,
  date_key          INT REFERENCES dim_date(date_key),
  product_sk        BIGINT REFERENCES dim_product(product_sk),
  -- Degenerate
  work_order_number VARCHAR(30),
  shift             TINYINT,
  work_center_code  VARCHAR(20),
  -- Measures
  planned_qty       NUMERIC(12,3),
  actual_qty        NUMERIC(12,3),
  scrap_qty         NUMERIC(12,3),
  rework_qty        NUMERIC(12,3),
  yield_pct         NUMERIC(7,4),
  planned_hours     NUMERIC(8,2),
  actual_hours      NUMERIC(8,2),
  downtime_hours    NUMERIC(8,2),
  oee_pct           NUMERIC(7,4),      -- Overall Equipment Effectiveness
  availability_pct  NUMERIC(7,4),
  performance_pct   NUMERIC(7,4),
  quality_pct       NUMERIC(7,4),
  production_cost   NUMERIC(15,2)
);
```

### fact_finance
```sql
CREATE TABLE fact_finance (
  finance_key       BIGINT PRIMARY KEY AUTO_INCREMENT,
  date_key          INT REFERENCES dim_date(date_key),
  -- Degenerate
  account_code      VARCHAR(20),
  account_type      VARCHAR(20),
  cost_center_code  VARCHAR(20),
  business_unit_code VARCHAR(20),
  -- Measures
  debit_amount      NUMERIC(18,2),
  credit_amount     NUMERIC(18,2),
  balance           NUMERIC(18,2),
  budget_amount     NUMERIC(18,2),
  variance_amount   NUMERIC(18,2),
  variance_pct      NUMERIC(7,4)
);
```

---

## ⚙️ dbt Project Structure

```
dbt/
├── dbt_project.yml
├── profiles.yml
├── models/
│   ├── staging/          # 1:1 dari source, minimal transform
│   │   ├── hris/
│   │   │   ├── stg_hris__employees.sql
│   │   │   ├── stg_hris__attendance.sql
│   │   │   └── stg_hris__payroll_runs.sql
│   │   ├── crm/
│   │   ├── sales/
│   │   ├── wms/
│   │   ├── mes/
│   │   └── finance/
│   ├── intermediate/     # Business logic, joining, enrichment
│   │   ├── int_employees_enriched.sql
│   │   ├── int_orders_with_products.sql
│   │   └── int_financial_transactions.sql
│   └── marts/            # Final dimension & fact tables
│       ├── core/
│       │   ├── dim_date.sql
│       │   ├── dim_employee.sql
│       │   └── dim_product.sql
│       ├── finance/
│       │   ├── fact_finance.sql
│       │   └── fact_payroll.sql
│       ├── sales/
│       │   └── fact_sales.sql
│       └── operations/
│           ├── fact_inventory.sql
│           └── fact_production.sql
├── tests/                # Data quality tests
├── seeds/                # Static reference data
├── macros/               # Reusable SQL macros
└── snapshots/            # SCD Type 2 snapshots
```

---

## 🔄 Refresh Schedule (Airflow DAG)

| DAG | Tujuan | Schedule | SLA |
|-----|--------|----------|-----|
| `stg_full_load` | Initial load semua staging | Manual | — |
| `stg_incremental` | CDC incremental update | Setiap 15 menit | 10 menit |
| `dbt_daily_run` | Run semua dbt models | 01:00 WIB | 2 jam |
| `dim_snapshot` | SCD Type 2 snapshot | 02:00 WIB | 30 menit |
| `fact_daily` | Fact table harian | 03:00 WIB | 1 jam |
| `dq_checks` | Data quality validation | 04:00 WIB | 30 menit |
| `cube_refresh` | OLAP cube materialisasi | 05:00 WIB | 1 jam |
