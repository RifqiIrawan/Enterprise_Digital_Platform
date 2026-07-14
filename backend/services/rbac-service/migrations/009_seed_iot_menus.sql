-- Seed: modul, role, menu, dan permission untuk IoT Simulator (Fase 6,
-- backend/modules/iot-service). Beda dari migrasi menu sebelumnya (007/008),
-- migrasi ini juga menambah baris `modules` dan `roles` baru -- tidak ada
-- migrasi setelah 002_seed.sql yang menambah modul, dan role 'iot' tidak ada
-- di 13 role awal 002_seed.sql (tabel User Role di Vision doc dibuat sebelum
-- modul ini direncanakan). Polanya tetap sama seperti
-- 008_seed_warehouse_master_menus.sql untuk menu & permission-nya.

INSERT INTO modules (code, name, sort_order) VALUES ('iot', 'IoT Simulator', 100);

INSERT INTO roles (code, name, description, is_system) VALUES
    ('iot', 'IoT', 'Modul IoT Simulator', TRUE);

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'devices', 'Devices', '/iot/devices', 'bi-cpu', 1
FROM modules WHERE code = 'iot';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'readings', 'Readings', '/iot/readings', 'bi-graph-up', 2
FROM modules WHERE code = 'iot';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'alerts', 'Alerts', '/iot/alerts', 'bi-exclamation-triangle', 3
FROM modules WHERE code = 'iot';

-- Super Admin: full access
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'super_admin' AND m.code IN ('devices', 'readings', 'alerts');

-- Auditor: view only
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r, menus m
WHERE r.code = 'auditor' AND m.code IN ('devices', 'readings', 'alerts');

-- IoT role: full access ke modul sendiri
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'iot' AND m.code IN ('devices', 'readings', 'alerts');

-- Company Admin & Branch Manager: full access operasional (sama seperti aturan
-- menu bisnis lain di 004_seed_role_permissions.sql)
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code IN ('company_admin', 'branch_manager') AND m.code IN ('devices', 'readings', 'alerts');
