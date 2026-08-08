-- CRM Service: leads, accounts, contacts, opportunities, dan activity log
-- (calls/emails/meetings/notes/tasks) terhadap salah satu dari 4 entitas di
-- atas. Mengikuti prinsip platform (README root): setiap data transaksi
-- membawa company_id/branch_id, tidak ada FK fisik lintas database.
--
-- FK di bawah (contacts.account_id, opportunities.account_id/contact_id)
-- SEMUA ke tabel dalam database crm_service ini sendiri -- bukan lintas
-- service, jadi FK fisik aman dipakai (sama seperti maintenance_schedules
-- -> assets di asset-service).
--
-- activities.reference_id SENGAJA TIDAK punya FK fisik -- dia polimorfik,
-- bisa menunjuk ke leads/accounts/contacts/opportunities tergantung
-- reference_type, dan Postgres tidak punya FK kondisional. Validasi
-- keberadaan + kepemilikan company_id dilakukan di level aplikasi
-- (internal/httpapi/activities.go), bukan di skema.
--
-- Penomoran dokumen (lead_number, opportunity_number) di-generate lewat
-- helper nextSequence (disalin per-modul dari sales-service, bukan
-- diimpor lintas modul) -- account_code SEBALIKNYA diisi manual oleh user
-- saat membuat Account langsung (master data, sama seperti
-- customers.customer_code di sales-service), tapi di-generate otomatis
-- lewat nextSequence saat Account dibuat sebagai efek samping konversi
-- Lead (tidak ada input manusia di momen itu, sama seperti so_number
-- yang di-generate otomatis saat convertQuotation).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE leads (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id                UUID NOT NULL,
    branch_id                 UUID,
    lead_number               VARCHAR(30) NOT NULL,
    first_name                VARCHAR(100) NOT NULL,
    last_name                 VARCHAR(100),
    company_name              VARCHAR(200),
    email                     VARCHAR(200),
    phone                     VARCHAR(50),
    source                    VARCHAR(20) NOT NULL DEFAULT 'OTHER'
        CHECK (source IN ('WEBSITE', 'REFERRAL', 'COLD_CALL', 'EVENT', 'ADVERTISEMENT', 'SOCIAL_MEDIA', 'OTHER')),
    status                    VARCHAR(20) NOT NULL DEFAULT 'NEW'
        CHECK (status IN ('NEW', 'CONTACTED', 'QUALIFIED', 'UNQUALIFIED', 'CONVERTED')),
    owner_user_id             UUID, -- auth-service user, tanpa FK fisik (lintas service)
    notes                     TEXT,
    converted_account_id      UUID,
    converted_contact_id      UUID,
    converted_opportunity_id  UUID,
    converted_at              TIMESTAMPTZ,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, lead_number)
);

CREATE INDEX idx_leads_company_id ON leads (company_id);
CREATE INDEX idx_leads_status ON leads (company_id, status);

CREATE TABLE accounts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id    UUID NOT NULL,
    branch_id     UUID,
    account_code  VARCHAR(30) NOT NULL,
    name          VARCHAR(200) NOT NULL,
    industry      VARCHAR(100),
    website       VARCHAR(200),
    phone         VARCHAR(50),
    email         VARCHAR(200),
    address       TEXT,
    account_type  VARCHAR(20) NOT NULL DEFAULT 'PROSPECT'
        CHECK (account_type IN ('PROSPECT', 'CUSTOMER', 'PARTNER', 'OTHER')),
    owner_user_id UUID,
    notes         TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, account_code)
);

CREATE INDEX idx_accounts_company_id ON accounts (company_id);

CREATE TABLE contacts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id  UUID NOT NULL,
    branch_id   UUID,
    account_id  UUID REFERENCES accounts(id),
    first_name  VARCHAR(100) NOT NULL,
    last_name   VARCHAR(100),
    job_title   VARCHAR(100),
    email       VARCHAR(200),
    phone       VARCHAR(50),
    is_primary  BOOLEAN NOT NULL DEFAULT FALSE,
    notes       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_contacts_company_id ON contacts (company_id);
CREATE INDEX idx_contacts_account_id ON contacts (account_id);

CREATE TABLE opportunities (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id            UUID NOT NULL,
    branch_id             UUID,
    opportunity_number    VARCHAR(30) NOT NULL,
    account_id            UUID NOT NULL REFERENCES accounts(id),
    contact_id            UUID REFERENCES contacts(id),
    name                  VARCHAR(200) NOT NULL,
    stage                 VARCHAR(20) NOT NULL DEFAULT 'PROSPECTING'
        CHECK (stage IN ('PROSPECTING', 'QUALIFICATION', 'PROPOSAL', 'NEGOTIATION', 'WON', 'LOST')),
    amount                NUMERIC(15, 2) NOT NULL DEFAULT 0,
    probability           SMALLINT NOT NULL DEFAULT 0 CHECK (probability BETWEEN 0 AND 100),
    expected_close_date   DATE,
    owner_user_id         UUID,
    lost_reason           TEXT,
    notes                 TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, opportunity_number)
);

CREATE INDEX idx_opportunities_company_id ON opportunities (company_id);
CREATE INDEX idx_opportunities_account_id ON opportunities (account_id);
CREATE INDEX idx_opportunities_stage ON opportunities (company_id, stage);

CREATE TABLE activities (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id      UUID NOT NULL,
    branch_id       UUID,
    reference_type  VARCHAR(20) NOT NULL CHECK (reference_type IN ('LEAD', 'ACCOUNT', 'CONTACT', 'OPPORTUNITY')),
    reference_id    UUID NOT NULL, -- polimorfik, lihat komentar di atas -- validasi di aplikasi, bukan FK
    activity_type   VARCHAR(20) NOT NULL CHECK (activity_type IN ('CALL', 'EMAIL', 'MEETING', 'NOTE', 'TASK')),
    subject         VARCHAR(200) NOT NULL,
    description     TEXT,
    activity_date   TIMESTAMPTZ NOT NULL DEFAULT now(),
    due_date        TIMESTAMPTZ, -- pengingat untuk activity_type = TASK
    is_done         BOOLEAN NOT NULL DEFAULT FALSE,
    owner_user_id   UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_activities_company_id ON activities (company_id);
CREATE INDEX idx_activities_reference ON activities (reference_type, reference_id);
