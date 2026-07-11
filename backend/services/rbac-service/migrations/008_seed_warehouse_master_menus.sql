-- Seed: menu Master Barang (Products) dan Master Gudang (Warehouses) untuk
-- warehouse-service (Fase 2). Menu stock/stock_transfer/stock_opname sudah
-- ada dari 003_seed_business_menus.sql (dan permission-nya dari
-- 004/005_seed_*_permissions.sql); migrasi ini menambah dua menu master lagi
-- (produk & gudang jadi prasyarat sebelum stok/mutasi/opname bisa dipakai)
-- beserta permission-nya, mengikuti pola 007_seed_finance_coa_menu.sql
-- karena 004/005 sudah lebih dulu dijalankan.

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'products', 'Master Barang', '/warehouse/products', 'bi-box-seam', 1
FROM modules WHERE code = 'warehouse';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'warehouses', 'Master Gudang', '/warehouse/warehouses', 'bi-building', 2
FROM modules WHERE code = 'warehouse';

-- Super Admin: full access
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'super_admin' AND m.code IN ('products', 'warehouses');

-- Auditor: view only
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r, menus m
WHERE r.code = 'auditor' AND m.code IN ('products', 'warehouses');

-- Warehouse role: full access ke modul sendiri
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code = 'warehouse' AND m.code IN ('products', 'warehouses');

-- Company Admin & Branch Manager: full access operasional (sama seperti aturan
-- menu bisnis lain di 004_seed_role_permissions.sql)
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code IN ('company_admin', 'branch_manager') AND m.code IN ('products', 'warehouses');
