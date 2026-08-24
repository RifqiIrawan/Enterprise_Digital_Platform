-- Fase 10 (AI): dua menu pelengkap di modul AI & BI -- Predictive Maintenance
-- dan Rekomendasi Pemesanan. Forecasting & Anomaly Detection sudah ada sejak
-- Fase 2; RAG Chatbot belum dikerjakan (butuh penyedia LLM & penyimpan vektor
-- yang belum ada di platform ini).
--
-- Keduanya layar OPERASIONAL, bukan layar eksekutif: yang membacanya adalah
-- orang yang menjadwalkan perbaikan dan orang yang membuat purchase order.
-- Karena itu role `executive` sengaja TIDAK diberi akses -- dia sudah punya
-- enam dashboard ringkasan dari Fase 9.

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'predictive_maintenance', 'Predictive Maintenance', '/ai-bi/predictive-maintenance', 'bi-activity', 40 FROM modules WHERE code = 'ai_bi';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'recommendations', 'Rekomendasi Pemesanan', '/ai-bi/recommendations', 'bi-lightbulb', 50 FROM modules WHERE code = 'ai_bi';

-- Akses penuh (view + export) untuk yang mengurus AI/BI dan lintas fungsi.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, TRUE
FROM roles r
JOIN menus m ON m.code IN ('predictive_maintenance', 'recommendations')
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'ai_bi'
WHERE r.code IN ('super_admin', 'company_admin', 'ai_analyst', 'branch_manager');

-- Auditor: view saja, seperti perlakuannya di seluruh menu lain.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r
JOIN menus m ON m.code IN ('predictive_maintenance', 'recommendations')
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'ai_bi'
WHERE r.code = 'auditor';

-- Peran operasional mendapat layar yang benar-benar dipakainya: Production &
-- Asset menjadwalkan perbaikan, Warehouse & Purchasing yang memesan barang.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, TRUE
FROM roles r
JOIN menus m ON m.module_id = (SELECT id FROM modules WHERE code = 'ai_bi')
WHERE (r.code IN ('production', 'asset')     AND m.code = 'predictive_maintenance')
   OR (r.code IN ('warehouse', 'purchasing') AND m.code = 'recommendations');
