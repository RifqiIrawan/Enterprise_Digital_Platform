-- Menu Data Lake (modul dw): halaman Gold (agregat bulanan) + status rekonsiliasi
-- lake vs ClickHouse. Izin identik dengan menu sync_status (pola 010): endpoint
-- build/reconcile di gateway memang dikunci ke /dw/sync-status.

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'data_lake', 'Data Lake', '/dw/data-lake', 'bi-water', 2
FROM modules WHERE code = 'dw';

INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE
FROM roles r, menus m
WHERE r.code IN ('super_admin', 'dw', 'company_admin', 'branch_manager') AND m.code = 'data_lake';

INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r, menus m
WHERE r.code = 'auditor' AND m.code = 'data_lake';
