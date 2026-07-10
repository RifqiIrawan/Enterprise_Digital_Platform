-- Seed: default permission untuk role bawaan sisanya (selain super_admin & auditor
-- yang sudah di-set di 002_seed.sql). Pola: full access ke menu modul sendiri
-- sesuai pemetaan role -> modul pada tabel User Role di 01_Vision_and_Roadmap.md.
-- Company Admin & Branch Manager mendapat akses operasional penuh lintas modul
-- bisnis, karena scope company/branch mereka sudah dibatasi lewat user_roles,
-- bukan lewat role_menu_permissions ini.

-- Company Admin: full access ke seluruh menu bisnis + menu administrasi company
-- (branch/department/role/user management), kecuali company_management
-- (pembuatan company baru tetap wewenang Super Admin).
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r
JOIN menus m ON TRUE
JOIN modules mod ON mod.id = m.module_id
WHERE r.code = 'company_admin'
  AND NOT (mod.code = 'core' AND m.code = 'company_management');

INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r
JOIN menus m ON TRUE
JOIN modules mod ON mod.id = m.module_id
WHERE r.code = 'company_admin'
  AND mod.code = 'core' AND m.code = 'company_management';

-- Branch Manager: full access ke seluruh menu modul bisnis (operasional
-- harian branch), view-only ke menu administrasi core (branch/department/
-- role/user management tetap wewenang Company Admin).
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r
JOIN menus m ON TRUE
JOIN modules mod ON mod.id = m.module_id
WHERE r.code = 'branch_manager' AND mod.code <> 'core';

INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r
JOIN menus m ON TRUE
JOIN modules mod ON mod.id = m.module_id
WHERE r.code = 'branch_manager' AND mod.code = 'core';

-- Role fungsional: full access hanya ke menu modul miliknya sendiri.
-- Tidak ada akses ke modul lain kecuali di-override lewat
-- user_menu_permission_overrides atau ditambah user_roles dengan role lain.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r
JOIN modules mod ON mod.code = r.code -- role code == module code (finance, hr, sales, purchasing, warehouse, production, qc, asset)
JOIN menus m ON m.module_id = mod.id
WHERE r.code IN ('finance', 'hr', 'sales', 'purchasing', 'warehouse', 'production', 'qc', 'asset');

-- AI Analyst -> modul ai_bi
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r
JOIN modules mod ON mod.code = 'ai_bi'
JOIN menus m ON m.module_id = mod.id
WHERE r.code = 'ai_analyst';
