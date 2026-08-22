-- Seed: dua menu KPI di modul HR yang sudah ada -- Indikator KPI & Penilaian
-- KPI. Ini melengkapi Fase 4 HRIS (cuti, lembur, kalender libur, kuota cuti,
-- dan sekarang KPI). Pola persis sama dengan 016 & 017.

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'kpi_indicators', 'Indikator KPI', '/hr/kpi-indicators', 'bi-bullseye', 80 FROM modules WHERE code = 'hr';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'kpi_reviews', 'Penilaian KPI', '/hr/kpi-reviews', 'bi-clipboard-data', 90 FROM modules WHERE code = 'hr';

-- Super Admin, HR, Company Admin, Branch Manager: akses penuh. can_approve
-- benar-benar terpakai di sini -- penilaian KPI punya alur persetujuan sendiri.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r
JOIN menus m ON m.code IN ('kpi_indicators', 'kpi_reviews')
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'hr'
WHERE r.code IN ('super_admin', 'hr', 'company_admin', 'branch_manager');

-- Auditor: view-only, sama seperti menu HR lainnya.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r
JOIN menus m ON m.code IN ('kpi_indicators', 'kpi_reviews')
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'hr'
WHERE r.code = 'auditor';
