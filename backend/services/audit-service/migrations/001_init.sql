-- Audit Service: audit trail terpusat, diisi lewat Kafka consumer (bukan
-- ditulis langsung oleh client) dari topic auth.*, company.*, rbac.* dst
-- sesuai konvensi di infra/kafka/topics.md.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE audit_logs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id       VARCHAR(100),
    event_type     VARCHAR(150) NOT NULL, -- nama topic, mis. auth.user.logged_in
    source_service VARCHAR(100) NOT NULL,
    actor_user_id  UUID,
    actor_email    VARCHAR(255),
    company_id     UUID,
    branch_id      UUID,
    action         VARCHAR(50),  -- create | update | delete | login | assign | revoke
    entity_type    VARCHAR(100), -- mis. user, role, company, user_role
    entity_id      VARCHAR(100),
    payload        JSONB,
    occurred_at    TIMESTAMPTZ NOT NULL,
    recorded_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_logs_company_id ON audit_logs (company_id);
CREATE INDEX idx_audit_logs_event_type ON audit_logs (event_type);
CREATE INDEX idx_audit_logs_actor_user_id ON audit_logs (actor_user_id);
CREATE INDEX idx_audit_logs_occurred_at ON audit_logs (occurred_at DESC);
