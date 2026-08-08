-- Seed: modul, role, menu, dan permission untuk CRM (crm-service).
-- Pola identik dengan 009_seed_iot_menus.sql/010_seed_dw_menus.sql -- modul
-- baru (tidak ada di 13 role awal 002_seed.sql), satu role baru khusus
-- modul ini, 5 menu (Leads/Accounts/Contacts/Opportunities/Activities),
-- 4-block permission grant standar.

INSERT INTO modules (code, name, sort_order) VALUES ('crm', 'CRM', 120);

INSERT INTO roles (code, name, description, is_system) VALUES
    ('crm', 'CRM', 'Modul CRM (Customer Relationship Management)', TRUE);

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'leads', 'Leads', '/crm/leads', 'bi-funnel', 1
FROM modules WHERE code = 'crm';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'accounts', 'Accounts', '/crm/accounts', 'bi-building', 2
FROM modules WHERE code = 'crm';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'contacts', 'Contacts', '/crm/contacts', 'bi-person-badge', 3
FROM modules WHERE code = 'crm';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'opportunities', 'Opportunities', '/crm/opportunities', 'bi-graph-up-arrow', 4
FROM modules WHERE code = 'crm';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'activities', 'Activities', '/crm/activities', 'bi-clock-history', 5
FROM modules WHERE code = 'crm';

-- Super Admin: full access
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'super_admin' AND m.code IN ('leads', 'accounts', 'contacts', 'opportunities', 'activities');

-- Auditor: view only
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r, menus m
WHERE r.code = 'auditor' AND m.code IN ('leads', 'accounts', 'contacts', 'opportunities', 'activities');

-- CRM role: full access ke modul sendiri
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'crm' AND m.code IN ('leads', 'accounts', 'contacts', 'opportunities', 'activities');

-- Company Admin & Branch Manager: full access operasional (sama seperti
-- aturan menu bisnis lain di 004_seed_role_permissions.sql)
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code IN ('company_admin', 'branch_manager') AND m.code IN ('leads', 'accounts', 'contacts', 'opportunities', 'activities');
