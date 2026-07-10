-- RBAC Service schema.
-- Menyediakan akses granular: per company, per menu, per departement, per user,
-- dengan pilihan hak akses penuh (create/update/delete/approve/export) atau view-only,
-- lewat dua lapis:
--   1. role_menu_permissions  -> default akses per role, per menu
--   2. user_menu_permission_overrides -> override eksplisit per user pada scope
--      company/branch/department/menu tertentu (opsional, menang atas role)
--
-- company_id / branch_id / department_id / user_id merujuk ke tabel di
-- company-service dan auth-service (database terpisah, pola microservices),
-- sehingga TIDAK memakai FOREIGN KEY fisik lintas database — konsistensi
-- dijaga lewat event Kafka (company.*, auth.*) dan validasi di application layer.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE modules (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code       VARCHAR(50) NOT NULL UNIQUE, -- core, finance, hr, sales, purchasing, warehouse, production, qc, asset, ai_bi
    name       VARCHAR(255) NOT NULL,
    sort_order INT NOT NULL DEFAULT 0
);

CREATE TABLE menus (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    module_id  UUID NOT NULL REFERENCES modules(id) ON DELETE CASCADE,
    parent_id  UUID REFERENCES menus(id) ON DELETE CASCADE, -- NULL = menu level teratas
    code       VARCHAR(100) NOT NULL, -- unik per module, dipakai sebagai kunci permission
    name       VARCHAR(255) NOT NULL,
    path       VARCHAR(255), -- route di frontend, mis. /finance/invoices
    icon       VARCHAR(100),
    sort_order INT NOT NULL DEFAULT 0,
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE (module_id, code)
);

CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id  UUID, -- NULL = role template global/system (mis. Super Admin, Auditor); terisi = custom role milik 1 company
    code        VARCHAR(100) NOT NULL,
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    is_system   BOOLEAN NOT NULL DEFAULT FALSE, -- role bawaan platform, tidak boleh dihapus
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- code unik per company; untuk role global (company_id NULL) code unik lintas seluruh role global
CREATE UNIQUE INDEX uq_roles_company_code ON roles (COALESCE(company_id, '00000000-0000-0000-0000-000000000000'), code);

-- Default permission per role, per menu.
-- can_view saja TRUE = view-only; seluruh kolom TRUE = full access.
CREATE TABLE role_menu_permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id     UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    menu_id     UUID NOT NULL REFERENCES menus(id) ON DELETE CASCADE,
    can_view    BOOLEAN NOT NULL DEFAULT TRUE,
    can_create  BOOLEAN NOT NULL DEFAULT FALSE,
    can_update  BOOLEAN NOT NULL DEFAULT FALSE,
    can_delete  BOOLEAN NOT NULL DEFAULT FALSE,
    can_approve BOOLEAN NOT NULL DEFAULT FALSE,
    can_export  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (role_id, menu_id)
);

-- Penugasan role ke user, di-scope ke company / branch / department tertentu.
-- Satu user bisa punya role berbeda di company/branch/department berbeda
-- (mis. Finance di Company A Branch 1, tapi hanya Auditor read-only di Company B).
CREATE TABLE user_roles (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL,
    role_id       UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    company_id    UUID NOT NULL,
    branch_id     UUID,      -- NULL = berlaku di seluruh branch dalam company tsb
    department_id UUID,      -- NULL = berlaku di seluruh department
    assigned_by   UUID,
    valid_from    TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to      TIMESTAMPTZ, -- NULL = tanpa batas waktu
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_user_roles_user_scope ON user_roles (user_id, company_id, branch_id, department_id);
CREATE INDEX idx_user_roles_role_id ON user_roles (role_id);

-- Override permission per user (opsional): menimpa hasil role_menu_permissions
-- pada scope company/branch/department/menu yang sama persis. Inilah yang
-- membuat "masing-masing user bisa berbeda" walau role-nya sama.
CREATE TABLE user_menu_permission_overrides (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL,
    company_id    UUID NOT NULL,
    branch_id     UUID,
    department_id UUID,
    menu_id       UUID NOT NULL REFERENCES menus(id) ON DELETE CASCADE,
    can_view      BOOLEAN NOT NULL DEFAULT TRUE,
    can_create    BOOLEAN NOT NULL DEFAULT FALSE,
    can_update    BOOLEAN NOT NULL DEFAULT FALSE,
    can_delete    BOOLEAN NOT NULL DEFAULT FALSE,
    can_approve   BOOLEAN NOT NULL DEFAULT FALSE,
    can_export    BOOLEAN NOT NULL DEFAULT FALSE,
    created_by    UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_user_menu_override_scope ON user_menu_permission_overrides (
    user_id, company_id,
    COALESCE(branch_id, '00000000-0000-0000-0000-000000000000'),
    COALESCE(department_id, '00000000-0000-0000-0000-000000000000'),
    menu_id
);
