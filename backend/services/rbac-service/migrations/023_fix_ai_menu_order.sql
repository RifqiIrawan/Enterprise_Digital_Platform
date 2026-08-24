-- Perbaikan urutan menu modul AI & BI.
--
-- 021 (dashboard per peran) memakai sort_order 40-90, lalu 022 (Predictive
-- Maintenance & Rekomendasi) memakai 40 dan 50 lagi. Akibatnya di sidebar
-- keenam dashboard peran tercerai-berai oleh dua menu AI di tengahnya --
-- ketahuan saat menu-tree dibuka dengan data sungguhan, bukan oleh test.
--
-- Ditaruh di migrasi baru alih-alih menyunting 022: 022 sudah pernah
-- dijalankan (tercatat di schema_migrations), jadi menyuntingnya tidak akan
-- berpengaruh apa pun pada database yang sudah ada.
--
-- Urutan yang dituju: analitik dulu (BI Dashboards, Forecasting, Anomaly
-- Detection, Predictive Maintenance, Rekomendasi), baru enam dashboard peran.

UPDATE menus SET sort_order = 40
WHERE code = 'predictive_maintenance'
  AND module_id = (SELECT id FROM modules WHERE code = 'ai_bi');

UPDATE menus SET sort_order = 50
WHERE code = 'recommendations'
  AND module_id = (SELECT id FROM modules WHERE code = 'ai_bi');

UPDATE menus SET sort_order = 60 WHERE code = 'bi_executive'     AND module_id = (SELECT id FROM modules WHERE code = 'ai_bi');
UPDATE menus SET sort_order = 70 WHERE code = 'bi_sales'         AND module_id = (SELECT id FROM modules WHERE code = 'ai_bi');
UPDATE menus SET sort_order = 80 WHERE code = 'bi_finance'       AND module_id = (SELECT id FROM modules WHERE code = 'ai_bi');
UPDATE menus SET sort_order = 90 WHERE code = 'bi_warehouse'     AND module_id = (SELECT id FROM modules WHERE code = 'ai_bi');
UPDATE menus SET sort_order = 100 WHERE code = 'bi_manufacturing' AND module_id = (SELECT id FROM modules WHERE code = 'ai_bi');
UPDATE menus SET sort_order = 110 WHERE code = 'bi_hr'            AND module_id = (SELECT id FROM modules WHERE code = 'ai_bi');
