-- Finance Service: Chart of Accounts, General Ledger (journal entries), dan
-- Invoices (AR/AP). Mengikuti prinsip platform (README root): setiap data
-- transaksi membawa company_id/branch_id, dan tidak ada FK fisik lintas
-- database ke company-service (konsistensi lewat event Kafka + validasi
-- application layer), sama seperti pola di rbac-service.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Chart of Accounts
CREATE TABLE accounts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id   UUID NOT NULL,
    account_code VARCHAR(20) NOT NULL,
    account_name VARCHAR(200) NOT NULL,
    account_type VARCHAR(20) NOT NULL CHECK (account_type IN ('ASSET', 'LIABILITY', 'EQUITY', 'REVENUE', 'EXPENSE')),
    parent_id    UUID REFERENCES accounts(id),
    is_posting   BOOLEAN NOT NULL DEFAULT TRUE, -- leaf node yang boleh dipakai di journal_lines
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, account_code)
);

CREATE INDEX idx_accounts_company_id ON accounts (company_id);

-- General Ledger: header
CREATE TABLE journal_entries (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id     UUID NOT NULL,
    branch_id      UUID,
    entry_number   VARCHAR(30) NOT NULL,
    entry_date     DATE NOT NULL,
    period         VARCHAR(7) NOT NULL, -- 'YYYY-MM'
    description    TEXT,
    reference_type VARCHAR(30) NOT NULL DEFAULT 'manual', -- manual | invoice_ar | invoice_ap | payroll dst
    reference_id   UUID,
    status         VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'POSTED', 'REVERSED')),
    total_debit    NUMERIC(18, 2) NOT NULL DEFAULT 0,
    total_credit   NUMERIC(18, 2) NOT NULL DEFAULT 0,
    posted_by      UUID,
    posted_at      TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, entry_number)
);

CREATE INDEX idx_journal_entries_company_id ON journal_entries (company_id);
CREATE INDEX idx_journal_entries_period ON journal_entries (company_id, period);

-- General Ledger: baris (harus balance: total debit = total credit per journal)
CREATE TABLE journal_lines (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    journal_id    UUID NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
    line_number   SMALLINT NOT NULL,
    account_id    UUID NOT NULL REFERENCES accounts(id),
    debit_amount  NUMERIC(18, 2) NOT NULL DEFAULT 0,
    credit_amount NUMERIC(18, 2) NOT NULL DEFAULT 0,
    description   TEXT,
    CHECK (debit_amount >= 0 AND credit_amount >= 0),
    CHECK (debit_amount = 0 OR credit_amount = 0) -- satu baris hanya debit ATAU credit, tidak dua-duanya
);

CREATE INDEX idx_journal_lines_journal_id ON journal_lines (journal_id);
CREATE INDEX idx_journal_lines_account_id ON journal_lines (account_id);

-- Invoices: AR (piutang ke customer) & AP (hutang ke vendor). Belum ada
-- master data customer/vendor terpisah (Fase 2 lanjutan/Sales/Purchasing),
-- jadi partner_name diisi bebas untuk saat ini.
CREATE TABLE invoices (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id       UUID NOT NULL,
    branch_id        UUID,
    invoice_type     VARCHAR(2) NOT NULL CHECK (invoice_type IN ('AR', 'AP')),
    invoice_number   VARCHAR(50) NOT NULL,
    partner_name     VARCHAR(200) NOT NULL,
    invoice_date     DATE NOT NULL,
    due_date         DATE,
    control_account_id UUID NOT NULL REFERENCES accounts(id), -- akun Piutang Usaha (AR) atau Hutang Usaha (AP)
    tax_account_id   UUID REFERENCES accounts(id),            -- akun PPN Keluaran/Masukan, opsional
    subtotal_amount  NUMERIC(15, 2) NOT NULL DEFAULT 0,
    tax_amount       NUMERIC(15, 2) NOT NULL DEFAULT 0,
    total_amount     NUMERIC(15, 2) NOT NULL DEFAULT 0,
    paid_amount      NUMERIC(15, 2) NOT NULL DEFAULT 0,
    status           VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'POSTED', 'PARTIALLY_PAID', 'PAID', 'CANCELLED')),
    journal_id       UUID REFERENCES journal_entries(id),     -- terisi setelah invoice diposting ke GL
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, invoice_number)
);

CREATE INDEX idx_invoices_company_id ON invoices (company_id);
CREATE INDEX idx_invoices_status ON invoices (company_id, status);

CREATE TABLE invoice_lines (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id  UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    line_number SMALLINT NOT NULL,
    account_id  UUID NOT NULL REFERENCES accounts(id), -- akun Revenue (AR) atau Expense (AP) untuk baris ini
    description VARCHAR(255) NOT NULL,
    quantity    NUMERIC(12, 2) NOT NULL DEFAULT 1,
    unit_price  NUMERIC(15, 2) NOT NULL DEFAULT 0,
    amount      NUMERIC(15, 2) NOT NULL DEFAULT 0
);

CREATE INDEX idx_invoice_lines_invoice_id ON invoice_lines (invoice_id);
