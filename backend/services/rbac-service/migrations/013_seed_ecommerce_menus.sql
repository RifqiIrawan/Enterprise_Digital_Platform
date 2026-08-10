-- Seed: modul, role, menu, dan permission untuk E-Commerce (ecommerce-service).
-- Pola identik dengan 012_seed_ticketing_menus.sql -- modul baru (tidak ada
-- di 13 role awal 002_seed.sql), satu role baru khusus modul ini, 1 menu
-- (Orders -- order_items tidak punya halaman terpisah, cuma dilihat lewat
-- detail order), 4-block permission grant standar.

INSERT INTO modules (code, name, sort_order) VALUES ('ecommerce', 'E-Commerce', 140);

INSERT INTO roles (code, name, description, is_system) VALUES
    ('ecommerce', 'E-Commerce', 'Modul E-Commerce (Order Online / Checkout)', TRUE);

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'orders', 'Orders', '/ecommerce/orders', 'bi-cart3', 1
FROM modules WHERE code = 'ecommerce';

-- Super Admin: full access
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'super_admin' AND m.code = 'orders';

-- Auditor: view only
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r, menus m
WHERE r.code = 'auditor' AND m.code = 'orders';

-- Ecommerce role: full access ke modul sendiri
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'ecommerce' AND m.code = 'orders';

-- Company Admin & Branch Manager: full access operasional (sama seperti
-- aturan menu bisnis lain di 004_seed_role_permissions.sql)
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code IN ('company_admin', 'branch_manager') AND m.code = 'orders';
