-- Fase 10 (AI) -- RAG Chatbot, potongan terakhir fase ini. Catatan di
-- 022_seed_ai_menus.sql menyebut fitur ini "belum dikerjakan (butuh penyedia
-- LLM & penyimpan vektor)"; yang akhirnya dibangun memakai Claude untuk
-- menyusun jawaban dan full-text search Postgres untuk mencarinya, tanpa
-- penyimpan vektor sama sekali (lihat rag-service/migrations/001_init.sql).
--
-- Dua menu, bukan satu, dan pemisahannya disengaja:
--
--   * `chatbot` -- bertanya. Ini bacaan dokumentasi, jadi hampir semua peran
--     boleh. sort_order 55 menempatkannya setelah Rekomendasi (50) dan sebelum
--     dashboard peran (60+), mengikuti urutan yang dirapikan migrasi 023:
--     alat analitik dulu, baru dashboard.
--   * `chatbot_queries` -- riwayat pertanyaan SEMUA orang, termasuk berapa
--     token yang terpakai. Itu dua hal yang tidak otomatis boleh dibaca siapa
--     saja yang boleh bertanya: pertanyaan orang lain memperlihatkan apa yang
--     sedang mereka kerjakan, dan angka biayanya urusan yang mengelola
--     platform. Karena itu menunya terpisah dan hanya untuk pengelola.
--
-- Hak `can_create` di menu `chatbot` BUKAN "boleh bertanya" -- bertanya cuma
-- butuh view. Create di sini memetakan ke POST /api/rag/ingest, yang menulis
-- ulang seluruh index yang jadi dasar jawaban chatbot (lihat policy.go).

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'chatbot', 'Chatbot Dokumentasi', '/ai-bi/chatbot', 'bi-chat-dots', 55 FROM modules WHERE code = 'ai_bi';

INSERT INTO menus (module_id, code, name, path, icon, sort_order)
SELECT id, 'chatbot_queries', 'Riwayat Chatbot', '/ai-bi/chatbot-queries', 'bi-clock-history', 56 FROM modules WHERE code = 'ai_bi';

-- Bertanya ke dokumentasi: seluruh peran fungsional. Dokumentasi platform
-- bukan data perusahaan -- menyembunyikan cara kerja sistem dari orang yang
-- memakainya setiap hari tidak melindungi apa pun, cuma memperlambat mereka.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, FALSE
FROM roles r
JOIN menus m ON m.code = 'chatbot'
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'ai_bi'
WHERE r.code IN ('finance', 'hr', 'sales', 'purchasing', 'warehouse', 'production',
                 'qc', 'asset', 'ai_analyst', 'branch_manager', 'auditor', 'executive');

-- Pengelola platform: boleh bertanya DAN boleh menyuruh index dibaca ulang
-- setelah dokumentasinya diperbarui.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, TRUE, FALSE, FALSE, FALSE, TRUE
FROM roles r
JOIN menus m ON m.code = 'chatbot'
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'ai_bi'
WHERE r.code IN ('super_admin', 'company_admin');

-- Riwayat & biaya: pengelola platform dan auditor saja.
INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_create, can_update, can_delete, can_approve, can_export)
SELECT r.id, m.id, TRUE, FALSE, FALSE, FALSE, FALSE, TRUE
FROM roles r
JOIN menus m ON m.code = 'chatbot_queries'
JOIN modules mod ON mod.id = m.module_id AND mod.code = 'ai_bi'
WHERE r.code IN ('super_admin', 'company_admin', 'auditor');
