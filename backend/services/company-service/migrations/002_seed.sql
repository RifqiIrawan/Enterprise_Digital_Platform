-- Seed: satu default company supaya user_roles (company_id NOT NULL) bisa
-- langsung dipakai di Fase 1, sebelum ada UI Company Management sungguhan.

INSERT INTO companies (code, name, status)
VALUES ('DEFAULT', 'PT Enterprise Digital Platform', 'active');
