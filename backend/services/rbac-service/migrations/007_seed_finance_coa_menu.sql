-- Seed: menu Chart of Accounts untuk finance-service (Fase 2). Menu
-- gl_journal/invoices/ar_ap sudah ada dari 003_seed_business_menus.sql;
-- migrasi ini menambah satu menu lagi dan permission-nya karena
-- 004/005_seed_*_permissions.sql sudah lebih dulu dijalankan.

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'chart_of_accounts', 'Chart of Accounts', '/finance/accounts', 'bi-list-columns-reverse', 5
FROM modules WHERE code = 'finance';

-- Super Admin: full access
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'super_admin' AND m.code = 'chart_of_accounts';

-- Auditor: view only
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r, menus m
WHERE r.code = 'auditor' AND m.code = 'chart_of_accounts';

-- Finance role: full access ke modul sendiri
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'finance' AND m.code = 'chart_of_accounts';

-- Company Admin & Branch Manager: full access operasional (sama seperti aturan
-- menu bisnis lain di 004_seed_role_permissions.sql)
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code IN ('company_admin', 'branch_manager') AND m.code = 'chart_of_accounts';
