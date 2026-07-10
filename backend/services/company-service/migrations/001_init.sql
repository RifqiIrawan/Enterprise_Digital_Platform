-- Company Service: struktur organisasi (Multi Company / Multi Branch / Multi Departement).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE companies (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code       VARCHAR(50) NOT NULL UNIQUE,
    name       VARCHAR(255) NOT NULL,
    status     VARCHAR(20) NOT NULL DEFAULT 'active', -- active | inactive
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE branches (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    code       VARCHAR(50) NOT NULL,
    name       VARCHAR(255) NOT NULL,
    address    TEXT,
    status     VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, code)
);

CREATE TABLE departments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    branch_id  UUID REFERENCES branches(id) ON DELETE CASCADE, -- NULL = department berlaku company-wide, tidak terikat 1 branch
    code       VARCHAR(50) NOT NULL,
    name       VARCHAR(255) NOT NULL,
    status     VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, code)
);

CREATE INDEX idx_branches_company_id ON branches(company_id);
CREATE INDEX idx_departments_company_id ON departments(company_id);
CREATE INDEX idx_departments_branch_id ON departments(branch_id);
