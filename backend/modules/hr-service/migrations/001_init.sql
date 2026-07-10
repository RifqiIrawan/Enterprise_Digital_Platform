-- HR Service: data karyawan, absensi, dan payroll dasar. Mengikuti prinsip
-- platform (README root): setiap data transaksi membawa company_id/branch_id,
-- tidak ada FK fisik lintas database. Mengikuti simplifikasi yang sama seperti
-- finance-service (partner_name bebas untuk invoice): department & job_title
-- disimpan sebagai teks bebas di employees, bukan master table terpisah,
-- karena belum ada modul org-structure tersendiri di Fase 2 ini.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE employees (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id        UUID NOT NULL,
    branch_id         UUID,
    employee_code     VARCHAR(20) NOT NULL,
    first_name        VARCHAR(100) NOT NULL,
    last_name         VARCHAR(100),
    email             VARCHAR(200) NOT NULL,
    phone             VARCHAR(20),
    department        VARCHAR(100),
    job_title         VARCHAR(100),
    manager_id        UUID REFERENCES employees(id),
    employment_type   VARCHAR(20) NOT NULL DEFAULT 'PERMANENT' CHECK (employment_type IN ('PERMANENT', 'CONTRACT', 'INTERN', 'OUTSOURCE')),
    status            VARCHAR(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE', 'TERMINATED', 'ON_LEAVE')),
    hire_date         DATE NOT NULL,
    termination_date  DATE,
    -- Struktur gaji disederhanakan langsung di baris employee (bukan tabel
    -- salary_components terpisah dengan effective_date) untuk Fase 2 ini.
    basic_salary      NUMERIC(15, 2) NOT NULL DEFAULT 0,
    monthly_allowance NUMERIC(15, 2) NOT NULL DEFAULT 0,
    ptkp_status       VARCHAR(10) NOT NULL DEFAULT 'TK/0' CHECK (ptkp_status IN ('TK/0', 'TK/1', 'TK/2', 'TK/3', 'K/0', 'K/1', 'K/2', 'K/3')),
    is_active         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, employee_code),
    UNIQUE (company_id, email)
);

CREATE INDEX idx_employees_company_id ON employees (company_id);
CREATE INDEX idx_employees_status ON employees (company_id, status);

-- Absensi harian. source MANUAL untuk input lewat UI, sesuai skema referensi
-- di 04_Database_Design.md (BIOMETRIC/QR/GPS belum ada integrasinya).
CREATE TABLE attendance_logs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id    UUID NOT NULL,
    employee_id   UUID NOT NULL REFERENCES employees(id),
    log_date      DATE NOT NULL,
    check_in      TIMESTAMPTZ,
    check_out     TIMESTAMPTZ,
    source        VARCHAR(20) NOT NULL DEFAULT 'MANUAL' CHECK (source IN ('BIOMETRIC', 'QR', 'GPS', 'MANUAL')),
    status        VARCHAR(20) NOT NULL DEFAULT 'PRESENT' CHECK (status IN ('PRESENT', 'LATE', 'EARLY_LEAVE', 'ABSENT', 'LEAVE')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (employee_id, log_date)
);

CREATE INDEX idx_attendance_company_id ON attendance_logs (company_id);
CREATE INDEX idx_attendance_employee_period ON attendance_logs (employee_id, log_date);

-- Payroll run: satu baris per periode (YYYY-MM) per company. journal_id
-- terisi setelah run diposting ke GL finance-service (lintas database,
-- lihat internal/financeclient).
CREATE TABLE payroll_runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id      UUID NOT NULL,
    branch_id       UUID,
    period          VARCHAR(7) NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'POSTED')),
    total_employees INTEGER NOT NULL DEFAULT 0,
    total_gross     NUMERIC(18, 2) NOT NULL DEFAULT 0,
    total_pph21     NUMERIC(18, 2) NOT NULL DEFAULT 0,
    total_bpjs      NUMERIC(18, 2) NOT NULL DEFAULT 0,
    total_deduction NUMERIC(18, 2) NOT NULL DEFAULT 0, -- = total_pph21 + total_bpjs, disimpan terpisah untuk breakdown jurnal GL
    total_net       NUMERIC(18, 2) NOT NULL DEFAULT 0,
    journal_id      UUID,
    posted_by       UUID,
    posted_at       TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, period)
);

CREATE INDEX idx_payroll_runs_company_id ON payroll_runs (company_id);

CREATE TABLE payroll_details (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payroll_run_id      UUID NOT NULL REFERENCES payroll_runs(id) ON DELETE CASCADE,
    employee_id         UUID NOT NULL REFERENCES employees(id),
    employee_name       VARCHAR(200) NOT NULL, -- snapshot, supaya tetap terbaca walau data employee berubah
    basic_salary        NUMERIC(15, 2) NOT NULL DEFAULT 0,
    total_allowance     NUMERIC(15, 2) NOT NULL DEFAULT 0,
    gross_salary        NUMERIC(15, 2) NOT NULL DEFAULT 0,
    pph21               NUMERIC(15, 2) NOT NULL DEFAULT 0,
    bpjs_kesehatan_emp  NUMERIC(15, 2) NOT NULL DEFAULT 0,
    bpjs_tk_jht_emp     NUMERIC(15, 2) NOT NULL DEFAULT 0,
    bpjs_tk_jp_emp      NUMERIC(15, 2) NOT NULL DEFAULT 0,
    total_deduction     NUMERIC(15, 2) NOT NULL DEFAULT 0,
    net_salary          NUMERIC(15, 2) NOT NULL DEFAULT 0,
    working_days        SMALLINT NOT NULL DEFAULT 0,
    present_days         SMALLINT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (payroll_run_id, employee_id)
);

CREATE INDEX idx_payroll_details_run_id ON payroll_details (payroll_run_id);
