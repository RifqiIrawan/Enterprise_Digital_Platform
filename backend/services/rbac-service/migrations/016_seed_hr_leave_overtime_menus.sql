-- Seed: dua menu baru di modul HR yang sudah ada -- Cuti & Lembur (Fase 4
-- HRIS di roadmap). Beda dari 013-015 yang menambah modul BARU: di sini
-- modulnya (`hr`) dan role fungsionalnya (`hr`) sudah ada sejak 002/004, jadi
-- yang perlu ditambahkan hanya menu-nya plus grant permission untuk kelima
-- role yang memang sudah punya akses ke menu HR lain.
--
-- Grant tidak bisa mengandalkan 004_seed_role_permissions.sql: file itu sudah
-- pernah jalan di database yang ada, dan tidak akan diulang oleh migrator.
-- Menu baru selalu butuh grant eksplisit -- pola yang sama seperti 013-015.

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'leave', 'Cuti', '/hr/leave', 'bi-calendar2-minus', 40 FROM modules WHERE code = 'hr';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'overtime', 'Lembur', '/hr/overtime', 'bi-clock-history', 50 FROM modules WHERE code = 'hr';

-- Super Admin, HR, Company Admin, Branch Manager: akses penuh (termasuk
-- can_approve -- kedua menu ini memang berbasis persetujuan).
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r
JOIN menus m ON m.code IN ('leave', 'overtime')
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'hr'
WHERE r.code IN ('super_admin', 'hr', 'company_admin', 'branch_manager');

-- Auditor: view-only, sama seperti menu HR lainnya.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r
JOIN menus m ON m.code IN ('leave', 'overtime')
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'hr'
WHERE r.code = 'auditor';
