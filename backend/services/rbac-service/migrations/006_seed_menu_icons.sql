-- Seed: bootstrap-icons class per menu, dipakai langsung oleh sidebar frontend
-- (lihat frontend/web/src/components/layout/Sidebar.jsx) supaya menu dari DB
-- tidak perlu mapping icon manual di kode frontend.

UPDATE menus SET icon = 'bi-building' WHERE code = 'company_management';
UPDATE menus SET icon = 'bi-diagram-3' WHERE code = 'branch_management';
UPDATE menus SET icon = 'bi-diagram-2' WHERE code = 'department_management';
UPDATE menus SET icon = 'bi-shield-lock' WHERE code = 'role_management';
UPDATE menus SET icon = 'bi-people' WHERE code = 'user_management';
UPDATE menus SET icon = 'bi-journal-text' WHERE code = 'audit_log';

UPDATE menus SET icon = 'bi-journal-bookmark' WHERE code = 'gl_journal';
UPDATE menus SET icon = 'bi-receipt' WHERE code = 'invoices';
UPDATE menus SET icon = 'bi-cash-coin' WHERE code = 'ar_ap';

UPDATE menus SET icon = 'bi-person-badge' WHERE code = 'employees';
UPDATE menus SET icon = 'bi-calendar-check' WHERE code = 'attendance';
UPDATE menus SET icon = 'bi-wallet2' WHERE code = 'payroll';

UPDATE menus SET icon = 'bi-person-lines-fill' WHERE code = 'customers';
UPDATE menus SET icon = 'bi-file-earmark-text' WHERE code = 'quotations';
UPDATE menus SET icon = 'bi-cart-check' WHERE code = 'sales_orders';

UPDATE menus SET icon = 'bi-truck' WHERE code = 'suppliers';
UPDATE menus SET icon = 'bi-file-earmark-plus' WHERE code = 'purchase_requisitions';
UPDATE menus SET icon = 'bi-cart' WHERE code = 'purchase_orders';

UPDATE menus SET icon = 'bi-boxes' WHERE code = 'stock';
UPDATE menus SET icon = 'bi-arrow-left-right' WHERE code = 'stock_transfer';
UPDATE menus SET icon = 'bi-clipboard-check' WHERE code = 'stock_opname';

UPDATE menus SET icon = 'bi-gear-wide-connected' WHERE code = 'work_orders';
UPDATE menus SET icon = 'bi-diagram-3-fill' WHERE code = 'bom';
UPDATE menus SET icon = 'bi-calendar3' WHERE code = 'production_schedule';

UPDATE menus SET icon = 'bi-patch-check' WHERE code = 'inspections';
UPDATE menus SET icon = 'bi-clipboard-data' WHERE code = 'quality_standards';

UPDATE menus SET icon = 'bi-hdd-stack' WHERE code = 'asset_register';
UPDATE menus SET icon = 'bi-tools' WHERE code = 'asset_maintenance';

UPDATE menus SET icon = 'bi-bar-chart-line' WHERE code = 'dashboards';
UPDATE menus SET icon = 'bi-graph-up' WHERE code = 'forecasting';
UPDATE menus SET icon = 'bi-exclamation-triangle' WHERE code = 'anomaly_detection';

UPDATE menus SET icon = 'bi-app-indicator' WHERE icon IS NULL;
