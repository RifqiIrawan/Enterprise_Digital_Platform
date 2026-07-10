# 04 — Database Design
## Enterprise Data Center Simulator (EDCS)

---

## 🗄️ Database Strategy

EDCS menggunakan **Database-per-Service pattern** — setiap microservice memiliki database sendiri untuk memastikan loose coupling dan independent deployability.

| Service | Database | Engine | Alasan |
|---------|----------|--------|--------|
| auth | auth_db | PostgreSQL | ACID, relational |
| erp-core | erp_db | PostgreSQL | Master data kompleks |
| hris | hris_db | PostgreSQL | Relational, compliance |
| payroll | payroll_db | PostgreSQL | ACID kritis |
| crm | crm_db | PostgreSQL | Relational + JSONB |
| sales | sales_db | PostgreSQL | Transaksional |
| wms | wms_db | PostgreSQL | Inventory ACID |
| inventory | inventory_db | PostgreSQL + Redis | Read-heavy |
| mes | mes_db | PostgreSQL | OEE tracking |
| finance | finance_db | PostgreSQL | Audit trail |
| procurement | procurement_db | PostgreSQL | Workflow state |
| asset | asset_db | PostgreSQL | Lifecycle tracking |
| iot | iot_db | TimescaleDB | Time-series data |
| notification | notif_db | Redis | Fast queue |
| search | — | Elasticsearch | Full-text search |
| ml-features | — | Redis + Postgres | Feature store |
| vector | — | Qdrant | Embeddings |

---

## 📐 Core Schema Conventions

### Naming Conventions
```sql
-- Table names: snake_case, plural
-- Column names: snake_case
-- Primary key: id (UUID v4)
-- Foreign keys: {table_singular}_id
-- Timestamps: created_at, updated_at, deleted_at (soft delete)
-- Booleans: is_{condition} (is_active, is_deleted)
-- Enums: stored sebagai VARCHAR dengan CHECK constraint

CREATE TABLE employees (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_code VARCHAR(20) UNIQUE NOT NULL,
  -- ... kolom bisnis ...
  is_active     BOOLEAN DEFAULT TRUE,
  created_by    UUID REFERENCES users(id),
  updated_by    UUID REFERENCES users(id),
  created_at    TIMESTAMPTZ DEFAULT NOW(),
  updated_at    TIMESTAMPTZ DEFAULT NOW(),
  deleted_at    TIMESTAMPTZ  -- NULL = tidak terhapus
);
```

### Audit Trail Pattern
```sql
-- Setiap tabel bisnis penting memiliki audit table
CREATE TABLE employees_audit (
  audit_id     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  operation    VARCHAR(10) NOT NULL, -- INSERT, UPDATE, DELETE
  changed_by   UUID NOT NULL,
  changed_at   TIMESTAMPTZ DEFAULT NOW(),
  old_data     JSONB,
  new_data     JSONB,
  entity_id    UUID NOT NULL -- referensi ke employees.id
);
```

---

## 🏢 ERP Database (erp_db)

```sql
-- Business Units
CREATE TABLE business_units (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code        VARCHAR(20) UNIQUE NOT NULL,
  name        VARCHAR(100) NOT NULL,
  parent_id   UUID REFERENCES business_units(id),
  is_active   BOOLEAN DEFAULT TRUE,
  created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Cost Centers
CREATE TABLE cost_centers (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code            VARCHAR(20) UNIQUE NOT NULL,
  name            VARCHAR(100) NOT NULL,
  business_unit_id UUID REFERENCES business_units(id),
  manager_id      UUID, -- FK ke HRIS employees
  budget_annual   NUMERIC(15,2),
  is_active       BOOLEAN DEFAULT TRUE
);

-- Chart of Accounts
CREATE TABLE accounts (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_code  VARCHAR(20) UNIQUE NOT NULL,
  account_name  VARCHAR(200) NOT NULL,
  account_type  VARCHAR(20) NOT NULL CHECK (account_type IN
                ('ASSET','LIABILITY','EQUITY','REVENUE','EXPENSE')),
  parent_id     UUID REFERENCES accounts(id),
  is_posting    BOOLEAN DEFAULT TRUE, -- leaf node bisa posting
  currency      CHAR(3) DEFAULT 'IDR',
  is_active     BOOLEAN DEFAULT TRUE
);
```

---

## 👥 HRIS Database (hris_db)

```sql
-- Employees
CREATE TABLE employees (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_code     VARCHAR(20) UNIQUE NOT NULL,
  first_name        VARCHAR(100) NOT NULL,
  last_name         VARCHAR(100),
  email             VARCHAR(200) UNIQUE NOT NULL,
  phone             VARCHAR(20),
  nik               CHAR(16) UNIQUE, -- NIK KTP
  gender            CHAR(1) CHECK (gender IN ('M','F')),
  birth_date        DATE,
  hire_date         DATE NOT NULL,
  termination_date  DATE,
  status            VARCHAR(20) DEFAULT 'ACTIVE'
                    CHECK (status IN ('ACTIVE','INACTIVE','TERMINATED','ON_LEAVE')),
  department_id     UUID REFERENCES departments(id),
  position_id       UUID REFERENCES positions(id),
  manager_id        UUID REFERENCES employees(id),
  employment_type   VARCHAR(20) CHECK (employment_type IN
                    ('PERMANENT','CONTRACT','INTERN','OUTSOURCE')),
  work_location     VARCHAR(50),
  photo_url         TEXT,
  is_active         BOOLEAN DEFAULT TRUE,
  created_at        TIMESTAMPTZ DEFAULT NOW(),
  updated_at        TIMESTAMPTZ DEFAULT NOW()
);

-- Departments
CREATE TABLE departments (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code        VARCHAR(20) UNIQUE NOT NULL,
  name        VARCHAR(100) NOT NULL,
  head_id     UUID REFERENCES employees(id),
  parent_id   UUID REFERENCES departments(id),
  is_active   BOOLEAN DEFAULT TRUE
);

-- Payroll Structure
CREATE TABLE salary_components (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code          VARCHAR(20) UNIQUE NOT NULL,
  name          VARCHAR(100) NOT NULL,
  component_type VARCHAR(20) CHECK (component_type IN
                  ('BASIC','ALLOWANCE','DEDUCTION','BENEFIT')),
  is_taxable    BOOLEAN DEFAULT TRUE,
  is_fixed      BOOLEAN DEFAULT TRUE,
  formula       TEXT -- untuk komponen variabel
);

CREATE TABLE employee_salaries (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id       UUID REFERENCES employees(id),
  component_id      UUID REFERENCES salary_components(id),
  amount            NUMERIC(15,2) NOT NULL,
  effective_date    DATE NOT NULL,
  end_date          DATE,
  UNIQUE (employee_id, component_id, effective_date)
);

-- Attendance
CREATE TABLE attendance_logs (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id   UUID REFERENCES employees(id),
  log_date      DATE NOT NULL,
  check_in      TIMESTAMPTZ,
  check_out     TIMESTAMPTZ,
  source        VARCHAR(20) DEFAULT 'MANUAL'
                CHECK (source IN ('BIOMETRIC','QR','GPS','MANUAL')),
  location      JSONB, -- {lat, lng, address}
  status        VARCHAR(20) DEFAULT 'PRESENT'
                CHECK (status IN ('PRESENT','LATE','EARLY_LEAVE','ABSENT'))
);

-- Leave
CREATE TABLE leave_requests (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id     UUID REFERENCES employees(id),
  leave_type_id   UUID REFERENCES leave_types(id),
  start_date      DATE NOT NULL,
  end_date        DATE NOT NULL,
  total_days      NUMERIC(4,1),
  reason          TEXT,
  status          VARCHAR(20) DEFAULT 'PENDING'
                  CHECK (status IN ('PENDING','APPROVED','REJECTED','CANCELLED')),
  approved_by     UUID REFERENCES employees(id),
  approved_at     TIMESTAMPTZ
);
```

---

## 💼 CRM Database (crm_db)

```sql
-- Contacts
CREATE TABLE contacts (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  first_name    VARCHAR(100) NOT NULL,
  last_name     VARCHAR(100),
  email         VARCHAR(200),
  phone         VARCHAR(20),
  company       VARCHAR(200),
  title         VARCHAR(100),
  source        VARCHAR(50),
  tags          TEXT[],
  custom_fields JSONB,
  assigned_to   UUID, -- sales rep (FK ke HRIS)
  is_active     BOOLEAN DEFAULT TRUE,
  created_at    TIMESTAMPTZ DEFAULT NOW()
);

-- Opportunities
CREATE TABLE opportunities (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name            VARCHAR(200) NOT NULL,
  contact_id      UUID REFERENCES contacts(id),
  stage           VARCHAR(50) NOT NULL,
  amount          NUMERIC(15,2),
  currency        CHAR(3) DEFAULT 'IDR',
  probability     SMALLINT CHECK (probability BETWEEN 0 AND 100),
  expected_close  DATE,
  actual_close    DATE,
  status          VARCHAR(20) DEFAULT 'OPEN'
                  CHECK (status IN ('OPEN','WON','LOST','CANCELLED')),
  owner_id        UUID,
  products        JSONB, -- [{product_id, qty, price}]
  notes           TEXT,
  created_at      TIMESTAMPTZ DEFAULT NOW(),
  updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Tickets
CREATE TABLE support_tickets (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ticket_number   VARCHAR(20) UNIQUE NOT NULL,
  subject         VARCHAR(200) NOT NULL,
  description     TEXT,
  contact_id      UUID REFERENCES contacts(id),
  priority        VARCHAR(10) DEFAULT 'MEDIUM'
                  CHECK (priority IN ('LOW','MEDIUM','HIGH','CRITICAL')),
  status          VARCHAR(20) DEFAULT 'OPEN'
                  CHECK (status IN ('OPEN','IN_PROGRESS','PENDING','RESOLVED','CLOSED')),
  assigned_to     UUID,
  channel         VARCHAR(20) DEFAULT 'EMAIL'
                  CHECK (channel IN ('EMAIL','PHONE','CHAT','PORTAL')),
  sla_due_at      TIMESTAMPTZ,
  resolved_at     TIMESTAMPTZ,
  csat_score      SMALLINT CHECK (csat_score BETWEEN 1 AND 5),
  created_at      TIMESTAMPTZ DEFAULT NOW()
);
```

---

## 🏭 WMS Database (wms_db)

```sql
-- Warehouses
CREATE TABLE warehouses (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code        VARCHAR(20) UNIQUE NOT NULL,
  name        VARCHAR(100) NOT NULL,
  address     TEXT,
  type        VARCHAR(20) CHECK (type IN ('MAIN','TRANSIT','VIRTUAL')),
  is_active   BOOLEAN DEFAULT TRUE
);

-- Locations (bin)
CREATE TABLE warehouse_locations (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  warehouse_id  UUID REFERENCES warehouses(id),
  zone          VARCHAR(20),
  aisle         VARCHAR(10),
  rack          VARCHAR(10),
  bin           VARCHAR(10),
  barcode       VARCHAR(50) UNIQUE,
  max_weight_kg NUMERIC(8,2),
  is_active     BOOLEAN DEFAULT TRUE
);

-- Stock
CREATE TABLE stock_levels (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  location_id     UUID REFERENCES warehouse_locations(id),
  product_id      UUID NOT NULL, -- FK ke ERP product master
  lot_number      VARCHAR(50),
  serial_number   VARCHAR(100),
  qty_on_hand     NUMERIC(12,3) DEFAULT 0,
  qty_reserved    NUMERIC(12,3) DEFAULT 0,
  qty_available   NUMERIC(12,3) GENERATED ALWAYS AS
                  (qty_on_hand - qty_reserved) STORED,
  unit_of_measure VARCHAR(20),
  expiry_date     DATE,
  last_counted_at TIMESTAMPTZ,
  UNIQUE (location_id, product_id, lot_number)
);

-- Stock Movements
CREATE TABLE stock_movements (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  movement_type   VARCHAR(30) NOT NULL
                  CHECK (movement_type IN ('RECEIPT','ISSUE','TRANSFER','ADJUSTMENT','RETURN')),
  reference_type  VARCHAR(30), -- 'PO','SO','WO','ADJ'
  reference_id    UUID,
  product_id      UUID NOT NULL,
  from_location   UUID REFERENCES warehouse_locations(id),
  to_location     UUID REFERENCES warehouse_locations(id),
  lot_number      VARCHAR(50),
  quantity        NUMERIC(12,3) NOT NULL,
  unit_cost       NUMERIC(15,4),
  total_cost      NUMERIC(15,2),
  moved_by        UUID,
  moved_at        TIMESTAMPTZ DEFAULT NOW()
);
```

---

## 💰 Finance Database (finance_db)

```sql
-- Journal Entries
CREATE TABLE journal_entries (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  entry_number    VARCHAR(30) UNIQUE NOT NULL,
  entry_date      DATE NOT NULL,
  period          VARCHAR(7) NOT NULL, -- 'YYYY-MM'
  description     TEXT,
  reference_type  VARCHAR(30),
  reference_id    UUID,
  status          VARCHAR(20) DEFAULT 'DRAFT'
                  CHECK (status IN ('DRAFT','POSTED','REVERSED')),
  total_debit     NUMERIC(18,2),
  total_credit    NUMERIC(18,2),
  posted_by       UUID,
  posted_at       TIMESTAMPTZ,
  created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE journal_lines (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  journal_id      UUID REFERENCES journal_entries(id),
  line_number     SMALLINT NOT NULL,
  account_id      UUID NOT NULL, -- FK ke ERP accounts
  cost_center_id  UUID,
  debit_amount    NUMERIC(18,2) DEFAULT 0,
  credit_amount   NUMERIC(18,2) DEFAULT 0,
  currency        CHAR(3) DEFAULT 'IDR',
  description     TEXT,
  CHECK (debit_amount = 0 OR credit_amount = 0)
);

-- AP Invoices
CREATE TABLE ap_invoices (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  invoice_number  VARCHAR(50) NOT NULL,
  vendor_id       UUID NOT NULL,
  po_id           UUID, -- FK ke procurement
  invoice_date    DATE NOT NULL,
  due_date        DATE,
  gross_amount    NUMERIC(15,2),
  tax_amount      NUMERIC(15,2),
  net_amount      NUMERIC(15,2),
  currency        CHAR(3) DEFAULT 'IDR',
  status          VARCHAR(20) DEFAULT 'PENDING'
                  CHECK (status IN ('PENDING','APPROVED','PAID','DISPUTED','CANCELLED')),
  payment_terms   VARCHAR(50),
  paid_amount     NUMERIC(15,2) DEFAULT 0,
  paid_at         TIMESTAMPTZ
);
```

---

## 📡 IoT Database (TimescaleDB)

```sql
-- Device Registry
CREATE TABLE iot_devices (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  device_code     VARCHAR(50) UNIQUE NOT NULL,
  device_name     VARCHAR(100),
  device_type     VARCHAR(50),
  location        VARCHAR(200),
  asset_id        UUID, -- FK ke asset
  firmware_version VARCHAR(20),
  is_online       BOOLEAN DEFAULT FALSE,
  last_seen_at    TIMESTAMPTZ,
  metadata        JSONB
);

-- Time-Series Sensor Readings (hypertable)
CREATE TABLE sensor_readings (
  time            TIMESTAMPTZ NOT NULL,
  device_id       UUID NOT NULL REFERENCES iot_devices(id),
  metric_name     VARCHAR(50) NOT NULL,
  metric_value    DOUBLE PRECISION NOT NULL,
  unit            VARCHAR(20),
  quality         SMALLINT DEFAULT 100
);
SELECT create_hypertable('sensor_readings', 'time');

-- Partitioning otomatis per minggu
SELECT add_retention_policy('sensor_readings', INTERVAL '1 year');
SELECT add_compression_policy('sensor_readings', INTERVAL '7 days');
```
