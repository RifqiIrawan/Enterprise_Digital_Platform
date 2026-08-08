-- Ticketing Service: kategori tiket, tiket (helpdesk/customer support), dan
-- komentar/timeline per tiket. Mengikuti prinsip platform (README root):
-- setiap data transaksi membawa company_id/branch_id, tidak ada FK fisik
-- lintas database.
--
-- Beda dengan crm-service's activities (polimorfik, reference_type +
-- reference_id tanpa FK karena bisa menunjuk ke salah satu dari 4 tabel):
-- ticket_comments.ticket_id di sini PAKAI FK FISIK BIASA -- komentar hanya
-- pernah menempel ke satu tiket, tidak ada kebutuhan polimorfik, jadi FK
-- normal sudah cukup dan lebih aman (Postgres bisa menjaga integritasnya
-- sendiri, tidak perlu validasi manual di kode seperti referenceExists di
-- crm-service).
--
-- ticket_categories SENGAJA berupa tabel master, bukan enum CHECK seperti
-- crm.leads.source/accounts.account_type -- kategori support (mis. Billing,
-- Technical, Feature Request) berbeda-beda per company dan wajar diedit
-- user, tidak seperti konsep CRM yang platform-wide.
--
-- Penomoran dokumen (ticket_number) di-generate lewat helper nextSequence
-- (disalin per-modul dari sales-service/crm-service, bukan diimpor lintas
-- modul, sama seperti semua modul lain di platform ini).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE ticket_categories (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id     UUID NOT NULL,
    branch_id      UUID,
    category_code  VARCHAR(30) NOT NULL,
    name           VARCHAR(100) NOT NULL,
    description    TEXT,
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, category_code)
);

CREATE INDEX idx_ticket_categories_company_id ON ticket_categories (company_id);

CREATE TABLE tickets (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id       UUID NOT NULL,
    branch_id        UUID,
    ticket_number    VARCHAR(30) NOT NULL,
    category_id      UUID NOT NULL REFERENCES ticket_categories(id),
    subject          VARCHAR(200) NOT NULL,
    description      TEXT,
    priority         VARCHAR(20) NOT NULL DEFAULT 'MEDIUM'
        CHECK (priority IN ('LOW', 'MEDIUM', 'HIGH', 'URGENT')),
    status           VARCHAR(20) NOT NULL DEFAULT 'OPEN'
        CHECK (status IN ('OPEN', 'IN_PROGRESS', 'RESOLVED', 'CLOSED')),
    requester_name   VARCHAR(200) NOT NULL,
    requester_email  VARCHAR(200),
    assigned_to      UUID, -- auth-service user, tanpa FK fisik (lintas service)
    resolved_at      TIMESTAMPTZ,
    closed_at        TIMESTAMPTZ,
    notes            TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, ticket_number)
);

CREATE INDEX idx_tickets_company_id ON tickets (company_id);
CREATE INDEX idx_tickets_status ON tickets (company_id, status);
CREATE INDEX idx_tickets_category_id ON tickets (category_id);

CREATE TABLE ticket_comments (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id       UUID NOT NULL,
    branch_id        UUID,
    ticket_id        UUID NOT NULL REFERENCES tickets(id),
    comment_text     TEXT NOT NULL,
    is_internal      BOOLEAN NOT NULL DEFAULT FALSE, -- catatan internal vs balasan yang terlihat customer
    author_user_id   UUID, -- dari header X-User-Id, tanpa FK (lintas service)
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ticket_comments_company_id ON ticket_comments (company_id);
CREATE INDEX idx_ticket_comments_ticket_id ON ticket_comments (ticket_id);
