-- Seed: contoh menu untuk tiap modul bisnis (Fase 2). Menu ini indikatif,
-- akan disesuaikan saat modul terkait di `backend/modules/` diimplementasikan.

INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'gl_journal', 'General Ledger / Jurnal', '/finance/journal', 10 FROM modules WHERE code = 'finance';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'invoices', 'Invoices', '/finance/invoices', 20 FROM modules WHERE code = 'finance';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'ar_ap', 'Accounts Payable/Receivable', '/finance/ar-ap', 30 FROM modules WHERE code = 'finance';

INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'employees', 'Data Karyawan', '/hr/employees', 10 FROM modules WHERE code = 'hr';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'attendance', 'Absensi', '/hr/attendance', 20 FROM modules WHERE code = 'hr';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'payroll', 'Payroll', '/hr/payroll', 30 FROM modules WHERE code = 'hr';

INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'customers', 'Customers', '/sales/customers', 10 FROM modules WHERE code = 'sales';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'quotations', 'Quotations', '/sales/quotations', 20 FROM modules WHERE code = 'sales';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'sales_orders', 'Sales Orders', '/sales/orders', 30 FROM modules WHERE code = 'sales';

INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'suppliers', 'Suppliers', '/purchasing/suppliers', 10 FROM modules WHERE code = 'purchasing';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'purchase_requisitions', 'Purchase Requisitions', '/purchasing/requisitions', 20 FROM modules WHERE code = 'purchasing';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'purchase_orders', 'Purchase Orders', '/purchasing/orders', 30 FROM modules WHERE code = 'purchasing';

INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'stock', 'Stok Gudang', '/warehouse/stock', 10 FROM modules WHERE code = 'warehouse';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'stock_transfer', 'Mutasi Antar Branch', '/warehouse/transfers', 20 FROM modules WHERE code = 'warehouse';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'stock_opname', 'Stock Opname', '/warehouse/opname', 30 FROM modules WHERE code = 'warehouse';

INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'work_orders', 'Work Order', '/production/work-orders', 10 FROM modules WHERE code = 'production';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'bom', 'Bill of Material', '/production/bom', 20 FROM modules WHERE code = 'production';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'production_schedule', 'Jadwal Produksi', '/production/schedule', 30 FROM modules WHERE code = 'production';

INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'inspections', 'Inspeksi Kualitas', '/qc/inspections', 10 FROM modules WHERE code = 'qc';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'quality_standards', 'Standar Mutu', '/qc/standards', 20 FROM modules WHERE code = 'qc';

INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'asset_register', 'Pendataan Aset', '/asset/register', 10 FROM modules WHERE code = 'asset';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'asset_maintenance', 'Maintenance Schedule', '/asset/maintenance', 20 FROM modules WHERE code = 'asset';

INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'dashboards', 'BI Dashboards', '/ai-bi/dashboards', 10 FROM modules WHERE code = 'ai_bi';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'forecasting', 'Forecasting', '/ai-bi/forecasting', 20 FROM modules WHERE code = 'ai_bi';
INSERT INTO menus (module_id, code, name, path, sort_order)
SELECT id, 'anomaly_detection', 'Anomaly Detection', '/ai-bi/anomaly-detection', 30 FROM modules WHERE code = 'ai_bi';
