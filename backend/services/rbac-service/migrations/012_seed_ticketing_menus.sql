-- Seed: modul, role, menu, dan permission untuk Ticketing (ticketing-service).
-- Pola identik dengan 011_seed_crm_menus.sql -- modul baru (tidak ada di 13
-- role awal 002_seed.sql), satu role baru khusus modul ini, 3 menu
-- (Categories/Tickets/Comments), 4-block permission grant standar.

INSERT INTO modules (code, name, sort_order) VALUES ('ticketing', 'Ticketing', 130);

INSERT INTO roles (code, name, description, is_system) VALUES
    ('ticketing', 'Ticketing', 'Modul Ticketing (Customer Support / Helpdesk)', TRUE);

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'categories', 'Categories', '/ticketing/categories', 'bi-tags', 1
FROM modules WHERE code = 'ticketing';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'tickets', 'Tickets', '/ticketing/tickets', 'bi-ticket-detailed', 2
FROM modules WHERE code = 'ticketing';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'comments', 'Comments', '/ticketing/comments', 'bi-chat-dots', 3
FROM modules WHERE code = 'ticketing';

-- Super Admin: full access
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'super_admin' AND m.code IN ('categories', 'tickets', 'comments');

-- Auditor: view only
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r, menus m
WHERE r.code = 'auditor' AND m.code IN ('categories', 'tickets', 'comments');

-- Ticketing role: full access ke modul sendiri
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'ticketing' AND m.code IN ('categories', 'tickets', 'comments');

-- Company Admin & Branch Manager: full access operasional (sama seperti
-- aturan menu bisnis lain di 004_seed_role_permissions.sql)
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code IN ('company_admin', 'branch_manager') AND m.code IN ('categories', 'tickets', 'comments');
