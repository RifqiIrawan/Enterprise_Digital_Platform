-- Seed: modul, role, menu, dan permission untuk Data Warehouse (dw-service).
-- Pola identik dengan 009_seed_iot_menus.sql -- modul baru (tidak ada di 13
-- role awal 002_seed.sql, dibuat setelah Vision doc), satu role baru khusus
-- modul ini, satu menu (Sync Status), 4-block permission grant standar.

INSERT INTO modules (code, name, sort_order) VALUES ('dw', 'Data Warehouse', 110);

INSERT INTO roles (code, name, description, is_system) VALUES
    ('dw', 'Data Warehouse', 'Modul Data Warehouse (ETL & sync status)', TRUE);

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'sync_status', 'Sync Status', '/dw/sync-status', 'bi-database-gear', 1
FROM modules WHERE code = 'dw';

-- Super Admin: full access
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'super_admin' AND m.code IN ('sync_status');

-- Auditor: view only
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r, menus m
WHERE r.code = 'auditor' AND m.code IN ('sync_status');

-- DW role: full access ke modul sendiri
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'dw' AND m.code IN ('sync_status');

-- Company Admin & Branch Manager: full access operasional (sama seperti aturan
-- menu bisnis lain di 004_seed_role_permissions.sql)
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code IN ('company_admin', 'branch_manager') AND m.code IN ('sync_status');
