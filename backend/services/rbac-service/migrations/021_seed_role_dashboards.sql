-- Fase 9 (Business Intelligence): enam dashboard per peran di modul AI & BI --
-- Eksekutif, Sales, Finance, Gudang, Manufaktur, dan SDM. Berbeda dari menu
-- "BI Dashboards" yang sudah ada (semua 17 grafik untuk analis), tiap dashboard
-- di sini menjawab pertanyaan satu peran saja.
--
-- Yang menentukan siapa melihat apa adalah hak menunya, bukan pemeriksaan peran
-- di dalam kode halaman: mekanisme yang sudah dipakai seluruh platform ini
-- (menu-tree menggambar sidebar, gateway menegakkan endpoint-nya) sudah
-- persis mekanisme yang dibutuhkan "dashboard per peran".

-- Role baru: Direksi. Dashboard Eksekutif butuh audiens, dan sampai sekarang
-- satu-satunya orang yang bisa melihat seluruh company adalah Company Admin --
-- peran administratif, bukan peran yang membaca angka. Direksi sengaja
-- view-only dan tidak diberi menu operasional apa pun.
INSERT INTO roles (code, name, description, is_system) VALUES
    ('executive', 'Direksi', 'Dashboard eksekutif lintas modul, hanya baca', TRUE);

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'bi_executive', 'Dashboard Eksekutif', '/bi/executive', 'bi-briefcase', 40 FROM modules WHERE code = 'ai_bi';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'bi_sales', 'Dashboard Sales', '/bi/sales', 'bi-graph-up-arrow', 50 FROM modules WHERE code = 'ai_bi';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'bi_finance', 'Dashboard Finance', '/bi/finance', 'bi-cash-stack', 60 FROM modules WHERE code = 'ai_bi';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'bi_warehouse', 'Dashboard Gudang', '/bi/warehouse', 'bi-box-seam', 70 FROM modules WHERE code = 'ai_bi';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'bi_manufacturing', 'Dashboard Manufaktur', '/bi/manufacturing', 'bi-gear-wide-connected', 80 FROM modules WHERE code = 'ai_bi';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'bi_hr', 'Dashboard SDM', '/bi/hr', 'bi-people', 90 FROM modules WHERE code = 'ai_bi';

-- Yang melihat SEMUA dashboard: Super Admin, Company Admin, Direksi, AI Analyst,
-- dan Branch Manager (yang memang mengurusi lintas fungsi di cabangnya).
-- Semuanya view + export saja: dashboard tidak punya aksi tulis.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, TRUE
FROM roles r
JOIN menus m ON m.code IN ('bi_executive', 'bi_sales', 'bi_finance', 'bi_warehouse', 'bi_manufacturing', 'bi_hr')
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'ai_bi'
WHERE r.code IN ('super_admin', 'company_admin', 'executive', 'ai_analyst', 'branch_manager');

-- Auditor: melihat semuanya juga, tapi tanpa export -- sama seperti perlakuan
-- auditor di seluruh menu lain.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r
JOIN menus m ON m.code IN ('bi_executive', 'bi_sales', 'bi_finance', 'bi_warehouse', 'bi_manufacturing', 'bi_hr')
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'ai_bi'
WHERE r.code = 'auditor';

-- Peran operasional hanya mendapat dashboard bidangnya sendiri. Manufaktur
-- dibagi ke Production DAN QC karena keduanya membaca lantai produksi yang
-- sama dari sisi berbeda (output vs mutu), dan Gudang ke Warehouse DAN
-- Purchasing karena isinya arus barang masuk-keluar beserta pemasoknya.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, TRUE
FROM roles r
JOIN menus m ON m.module_id = (SELECT id FROM modules WHERE code = 'ai_bi')
WHERE (r.code = 'sales'      AND m.code = 'bi_sales')
   OR (r.code = 'finance'    AND m.code = 'bi_finance')
   OR (r.code = 'warehouse'  AND m.code = 'bi_warehouse')
   OR (r.code = 'purchasing' AND m.code = 'bi_warehouse')
   OR (r.code = 'production' AND m.code = 'bi_manufacturing')
   OR (r.code = 'qc'         AND m.code = 'bi_manufacturing')
   OR (r.code = 'hr'         AND m.code = 'bi_hr');
