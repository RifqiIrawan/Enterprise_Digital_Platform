-- Seed: modul, role, menu, dan permission untuk Project Management
-- (project-service). Pola identik dengan 014_seed_fleet_menus.sql -- modul
-- baru (tidak ada di 13 role awal 002_seed.sql), satu role baru khusus modul
-- ini, 3 menu (Proyek/Tugas/Timesheet), 4-block permission grant standar.

INSERT INTO modules (code, name, sort_order) VALUES ('project', 'Project Management', 160);

INSERT INTO roles (code, name, description, is_system) VALUES
    ('project', 'Project Management', 'Modul Project Management (Proyek, Tugas, Timesheet)', TRUE);

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'projects', 'Proyek', '/project/projects', 'bi-kanban', 1
FROM modules WHERE code = 'project';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'tasks', 'Tugas', '/project/tasks', 'bi-list-task', 2
FROM modules WHERE code = 'project';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'timesheets', 'Timesheet', '/project/timesheets', 'bi-clock-history', 3
FROM modules WHERE code = 'project';

-- Super Admin: full access
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'super_admin' AND m.code IN ('projects', 'tasks', 'timesheets');

-- Auditor: view only
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r, menus m
WHERE r.code = 'auditor' AND m.code IN ('projects', 'tasks', 'timesheets');

-- Project role: full access ke modul sendiri
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'project' AND m.code IN ('projects', 'tasks', 'timesheets');

-- Company Admin & Branch Manager: full access operasional (sama seperti
-- aturan menu bisnis lain di 004_seed_role_permissions.sql)
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code IN ('company_admin', 'branch_manager') AND m.code IN ('projects', 'tasks', 'timesheets');
