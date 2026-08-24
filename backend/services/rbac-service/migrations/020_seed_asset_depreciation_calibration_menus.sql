-- Seed: dua menu Fase 5 di modul Asset yang sudah ada -- Kalibrasi dan
-- Penyusutan. Melengkapi Pendataan Aset & Maintenance Schedule dari Fase 2.
-- Pola sama dengan 016-019.

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'asset_calibration', 'Kalibrasi', '/asset/calibration', 'bi-rulers', 30 FROM modules WHERE code = 'asset';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'asset_depreciation', 'Penyusutan', '/asset/depreciation', 'bi-graph-down-arrow', 40 FROM modules WHERE code = 'asset';

-- Super Admin, Asset, Company Admin, Branch Manager: akses penuh. can_approve
-- terpakai betul di Penyusutan -- memposting jurnal penyusutan ke buku besar
-- adalah hak tersendiri, terpisah dari sekadar menghitungnya.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r
JOIN menus m ON m.code IN ('asset_calibration', 'asset_depreciation')
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'asset'
WHERE r.code IN ('super_admin', 'asset', 'company_admin', 'branch_manager');

-- Finance ikut diberi akses ke Penyusutan (view + approve, tanpa mengubah data
-- aset): jurnal yang dihasilkannya masuk ke buku besar yang mereka pegang, dan
-- orang yang bertanggung jawab atas buku besar itulah yang paling masuk akal
-- menekan tombol postingnya.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, TRUE, TRUE
FROM roles r
JOIN menus m ON m.code = 'asset_depreciation'
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'asset'
WHERE r.code = 'finance';

-- Auditor: view-only, sama seperti menu aset lainnya.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r
JOIN menus m ON m.code IN ('asset_calibration', 'asset_depreciation')
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'asset'
WHERE r.code = 'auditor';
