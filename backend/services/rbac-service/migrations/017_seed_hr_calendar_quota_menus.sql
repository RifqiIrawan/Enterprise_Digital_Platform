-- Seed: dua menu baru di modul HR yang sudah ada -- Kalender Hari Libur &
-- Kuota Cuti (melengkapi Fase 4 HRIS). Pola persis sama dengan
-- 016_seed_hr_leave_overtime_menus.sql: modul `hr` dan role fungsionalnya
-- sudah ada sejak 002/004, jadi yang perlu ditambahkan hanya menu-nya plus
-- grant eksplisit untuk kelima role yang memang sudah punya akses menu HR.
--
-- Grant tidak bisa mengandalkan 004_seed_role_permissions.sql: file itu sudah
-- pernah jalan di database yang ada dan tidak akan diulang migrator.

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'holidays', 'Kalender Hari Libur', '/hr/holidays', 'bi-calendar-event', 60 FROM modules WHERE code = 'hr';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'leave_quota', 'Kuota Cuti', '/hr/leave-quota', 'bi-calendar2-check', 70 FROM modules WHERE code = 'hr';

-- Super Admin, HR, Company Admin, Branch Manager: akses penuh. can_approve
-- ikut TRUE supaya seragam dengan menu HR lain, walau kedua menu ini tidak
-- punya alur persetujuan sendiri.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r
JOIN menus m ON m.code IN ('holidays', 'leave_quota')
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'hr'
WHERE r.code IN ('super_admin', 'hr', 'company_admin', 'branch_manager');

-- Auditor: view-only, sama seperti menu HR lainnya.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r
JOIN menus m ON m.code IN ('holidays', 'leave_quota')
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'hr'
WHERE r.code = 'auditor';
