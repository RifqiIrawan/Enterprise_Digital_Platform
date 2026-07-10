-- Seed: satu Super Admin user supaya login bisa langsung dicoba di Fase 1.
-- Password: Admin@12345 (ganti setelah login pertama kali di Fase 2 saat ada
-- fitur ganti password). Hash di-generate dengan bcrypt cost 10.

INSERT INTO users (email, username, password_hash, full_name, is_super_admin, status)
VALUES (
    'admin@edp.local',
    'admin',
    '$2a$10$CwbtXuNaFO0h.agxG4QojeIEvAcD19r6z6vE7kICO6d3QTrCuJi.y',
    'Super Admin',
    TRUE,
    'active'
);
