-- RAG Service (Fase 10) -- chatbot yang menjawab pertanyaan tentang platform
-- ini DARI DOKUMENTASINYA SENDIRI, dengan kutipan sumber.
--
-- Dua hal yang perlu diingat soal bentuk data di sini:
--
-- 1. Korpusnya BUKAN data transaksi, jadi TIDAK ber-company_id. Dokumentasi
--    platform sama untuk semua perusahaan yang memakainya -- memberinya
--    company_id akan menyiratkan isolasi yang tidak ada, dan membuat 21 salinan
--    dokumen yang sama begitu ada 21 tenant. Yang ber-company_id justru
--    PERTANYAANNYA (rag_queries), karena itu memang jejak aktivitas orang.
--
-- 2. Pencarian memakai full-text search bawaan Postgres dengan konfigurasi
--    'indonesian' (stemmer Snowball yang sudah ada di Postgres 18, tidak perlu
--    ekstensi apa pun), ditambah pg_trgm sebagai jalur cadangan untuk salah
--    ketik. Ini pencarian LEKSIKAL, bukan semantik: pertanyaan yang
--    diparafrase total tanpa satu pun kata yang sama dengan dokumennya akan
--    meleset. Batas itu disebut apa adanya di respons /ask dan di dokumentasi,
--    bukan disembunyikan di balik kata "AI".
--
--    pgvector sengaja tidak dipakai: ekstensinya tidak tersedia di instance
--    Postgres platform ini, dan Anthropic tidak menyediakan endpoint embedding
--    (Claude hanya generasi), jadi jalur vektor akan menambah DUA dependency
--    baru sekaligus. Antarmuka pencariannya (internal/httpapi/retrieve.go)
--    dibuat supaya backend vektor bisa masuk belakangan tanpa mengubah /ask.

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Satu baris = satu berkas dokumentasi. source_path relatif terhadap
-- RAG_CORPUS_DIR supaya isi tabel tidak berubah hanya karena foldernya
-- dipindah. content_hash membuat ingest idempotent: berkas yang tidak berubah
-- tidak di-chunk ulang, jadi menjalankan /ingest dua kali tidak menggandakan
-- apa pun dan tidak membuang pekerjaan.
CREATE TABLE rag_documents (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_path  TEXT NOT NULL UNIQUE,
    title        TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    chunk_count  INTEGER NOT NULL DEFAULT 0,
    byte_size    INTEGER NOT NULL DEFAULT 0,
    indexed_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Satu baris = satu potongan dokumen, dipotong per heading markdown. Heading
-- ikut disimpan DAN ikut masuk search_vector dengan bobot lebih tinggi: judul
-- bagian ("Rumus OEE", "Penyusutan") biasanya justru kata yang diketik orang.
--
-- search_vector adalah kolom GENERATED, bukan kolom yang diisi aplikasi.
-- Alasannya sama seperti CHECK constraint di modul lain: kalau pengisiannya
-- diserahkan ke kode Go, satu jalur tulis yang lupa memperbaruinya akan
-- membuat chunk yang tidak pernah muncul di hasil pencarian -- dan itu gagal
-- diam-diam, tanpa galat.
CREATE TABLE rag_chunks (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id   UUID NOT NULL REFERENCES rag_documents(id) ON DELETE CASCADE,
    chunk_index   INTEGER NOT NULL,
    heading       TEXT NOT NULL DEFAULT '',
    content       TEXT NOT NULL,
    char_count    INTEGER NOT NULL,
    search_vector TSVECTOR GENERATED ALWAYS AS (
        setweight(to_tsvector('indonesian', coalesce(heading, '')), 'A') ||
        setweight(to_tsvector('indonesian', content), 'B')
    ) STORED,
    UNIQUE (document_id, chunk_index)
);

CREATE INDEX idx_rag_chunks_search_vector ON rag_chunks USING GIN (search_vector);
-- Jalur cadangan salah ketik: trigram atas heading saja, bukan atas content.
-- Indeks trigram pada seluruh isi dokumen jauh lebih besar daripada nilainya
-- di sini -- yang biasanya salah ketik adalah istilah yang dicari, dan istilah
-- itu ada di heading.
CREATE INDEX idx_rag_chunks_heading_trgm ON rag_chunks USING GIN (heading gin_trgm_ops);

-- Jejak pertanyaan. Ini yang membuat biaya dan perilaku chatbot bisa diperiksa
-- belakangan, bukan cuma dirasakan: berapa yang benar-benar dijawab model,
-- berapa yang hanya mengembalikan kutipan karena LLM tidak dikonfigurasi,
-- berapa token yang terpakai, dan chunk mana yang jadi dasar jawabannya.
CREATE TABLE rag_queries (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id     UUID,
    user_id        UUID,
    question       TEXT NOT NULL,
    -- ANSWERED = model menjawab; RETRIEVED_ONLY = kutipan ditemukan tapi LLM
    -- tidak tersedia/ tidak dikonfigurasi; NO_MATCH = korpus tidak punya
    -- bahan untuk menjawab, dan model sengaja TIDAK dipanggil untuk itu.
    outcome        VARCHAR(20) NOT NULL CHECK (outcome IN ('ANSWERED', 'RETRIEVED_ONLY', 'NO_MATCH', 'ERROR')),
    chunk_ids      UUID[] NOT NULL DEFAULT '{}',
    model          TEXT NOT NULL DEFAULT '',
    input_tokens   INTEGER NOT NULL DEFAULT 0,
    output_tokens  INTEGER NOT NULL DEFAULT 0,
    latency_ms     INTEGER NOT NULL DEFAULT 0,
    error_message  TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_rag_queries_company_created ON rag_queries (company_id, created_at DESC);
CREATE INDEX idx_rag_queries_outcome ON rag_queries (outcome, created_at DESC);
