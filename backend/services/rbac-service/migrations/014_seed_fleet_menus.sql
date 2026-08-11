-- Seed: modul, role, menu, dan permission untuk Fleet & Delivery
-- (fleet-service). Pola identik dengan 013_seed_ecommerce_menus.sql -- modul
-- baru (tidak ada di 13 role awal 002_seed.sql), satu role baru khusus modul
-- ini, 3 menu (Vehicles/Drivers/Delivery Orders), 4-block permission grant
-- standar.

INSERT INTO modules (code, name, sort_order) VALUES ('fleet', 'Fleet & Delivery', 150);

INSERT INTO roles (code, name, description, is_system) VALUES
    ('fleet', 'Fleet & Delivery', 'Modul Fleet & Delivery (Armada, Pengemudi, Surat Jalan)', TRUE);

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'vehicles', 'Kendaraan', '/fleet/vehicles', 'bi-truck', 1
FROM modules WHERE code = 'fleet';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'drivers', 'Pengemudi', '/fleet/drivers', 'bi-person-vcard', 2
FROM modules WHERE code = 'fleet';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'delivery-orders', 'Surat Jalan', '/fleet/delivery-orders', 'bi-clipboard-check', 3
FROM modules WHERE code = 'fleet';

-- Super Admin: full access
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'super_admin' AND m.code IN ('vehicles', 'drivers', 'delivery-orders');

-- Auditor: view only
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r, menus m
WHERE r.code = 'auditor' AND m.code IN ('vehicles', 'drivers', 'delivery-orders');

-- Fleet role: full access ke modul sendiri
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'fleet' AND m.code IN ('vehicles', 'drivers', 'delivery-orders');

-- Company Admin & Branch Manager: full access operasional (sama seperti
-- aturan menu bisnis lain di 004_seed_role_permissions.sql)
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code IN ('company_admin', 'branch_manager') AND m.code IN ('vehicles', 'drivers', 'delivery-orders');
