-- Permission Super Admin (full access) & Auditor (view-only) ke SELURUH menu
-- yang ada, termasuk menu modul bisnis. Sengaja dijalankan paling akhir
-- (setelah 003_seed_business_menus.sql) supaya mencakup semua menu, bukan
-- cuma menu core yang ada saat 002_seed.sql dijalankan.

INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'super_admin';

INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r, menus m
WHERE r.code = 'auditor';
