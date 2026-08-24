-- Seed: empat menu MES (Fase 3) di modul Production yang sudah ada -- Mesin,
-- Shift, Eksekusi Produksi, dan OEE. Melengkapi Work Order/BOM/Jadwal Produksi
-- dari Fase 2. Pola sama dengan 016-018.

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'machines', 'Mesin', '/production/machines', 'bi-gear-wide-connected', 40 FROM modules WHERE code = 'production';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'shifts', 'Shift Produksi', '/production/shifts', 'bi-clock-history', 50 FROM modules WHERE code = 'production';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'production_runs', 'Eksekusi Produksi', '/production/runs', 'bi-play-circle', 60 FROM modules WHERE code = 'production';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'oee', 'OEE Mesin', '/production/oee', 'bi-speedometer2', 70 FROM modules WHERE code = 'production';

-- Super Admin, Production, Company Admin, Branch Manager: akses penuh.
-- can_delete benar-benar terpakai di Eksekusi Produksi: catatan downtime yang
-- salah masuk harus bisa dihapus selama run-nya belum ditutup.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r
JOIN menus m ON m.code IN ('machines', 'shifts', 'production_runs', 'oee')
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'production'
WHERE r.code IN ('super_admin', 'production', 'company_admin', 'branch_manager');

-- Auditor: view-only, sama seperti menu produksi lainnya.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r
JOIN menus m ON m.code IN ('machines', 'shifts', 'production_runs', 'oee')
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'production'
WHERE r.code = 'auditor';
