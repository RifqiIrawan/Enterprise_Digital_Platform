-- Seed: modul, contoh menu, dan role global sesuai tabel User Role
-- di Enterprise_Digital_Platform_Documentation/01_Vision_and_Roadmap.md

INSERT INTO modules (code, name, sort_order) VALUES
    ('core', 'Core / Administrasi', 0),
    ('finance', 'Finance', 10),
    ('hr', 'HR', 20),
    ('sales', 'Sales', 30),
    ('purchasing', 'Purchasing', 40),
    ('warehouse', 'Warehouse', 50),
    ('production', 'Production', 60),
    ('qc', 'Quality Control', 70),
    ('asset', 'Asset', 80),
    ('ai_bi', 'AI & BI', 90);

-- Contoh menu di module core (admin panel)
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'company_management', 'Company Management', '/admin/companies', 10 FROM modules WHERE code = 'core';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'branch_management', 'Branch Management', '/admin/branches', 20 FROM modules WHERE code = 'core';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'department_management', 'Department Management', '/admin/departments', 30 FROM modules WHERE code = 'core';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'role_management', 'Role Management', '/admin/roles', 40 FROM modules WHERE code = 'core';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'user_management', 'User Management', '/admin/users', 50 FROM modules WHERE code = 'core';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'audit_log', 'Audit Log', '/admin/audit-logs', 60 FROM modules WHERE code = 'core';

-- Role global (company_id NULL = template bawaan platform, is_system = TRUE)
INSERT INTO roles (code, name, description, is_system) VALUES
    ('super_admin', 'Super Admin', 'Akses semua company', TRUE),
    ('company_admin', 'Company Admin', 'Akses penuh dalam company sendiri', TRUE),
    ('branch_manager', 'Branch Manager', 'Akses dalam branch sendiri', TRUE),
    ('finance', 'Finance', 'Modul Finance', TRUE),
    ('hr', 'HR', 'Modul HRIS', TRUE),
    ('sales', 'Sales', 'Modul Sales', TRUE),
    ('purchasing', 'Purchasing', 'Modul Purchasing', TRUE),
    ('warehouse', 'Warehouse', 'Modul Gudang', TRUE),
    ('production', 'Production', 'Modul MES', TRUE),
    ('qc', 'QC', 'Modul Quality', TRUE),
    ('asset', 'Asset', 'Modul Asset', TRUE),
    ('auditor', 'Auditor', 'Read only lintas modul', TRUE),
    ('ai_analyst', 'AI Analyst', 'Modul AI & BI', TRUE);

-- Permission Super Admin & Auditor di-seed di 005_seed_admin_auditor_permissions.sql
-- (dijalankan setelah seluruh menu, termasuk menu modul bisnis, sudah ada).
