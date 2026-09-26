-- Statistik kata korpus -- penambahan yang lahir dari menjalankan pencarian
-- terhadap dokumentasi sungguhan, bukan dari korpus contoh di test.
--
-- Masalahnya: tsquery yang menyambung seluruh kata pertanyaan dengan ATAU
-- membuat kata umum ("cara", "apa", "yang", "sistem") mencocokkan hampir
-- setiap potongan. Akibatnya dua hal yang sama-sama buruk: pertanyaan yang
-- jelas di luar topik ("resep rendang") tetap mengembalikan kutipan, dan
-- pertanyaan yang benar tenggelam di antara potongan yang cuma kebetulan
-- memuat kata "cara".
--
-- Perbaikannya bukan menyambung dengan DAN lagi (itu justru membuat satu kata
-- meleset menghapus seluruh hasil), melainkan MEMBUANG kata yang terlalu umum
-- dari pertanyaannya. Kata yang muncul di sebagian besar dokumen tidak
-- membedakan apa pun -- itu ide yang sama dengan IDF di pencarian klasik,
-- dihitung dari korpus ini sendiri, bukan dari daftar stopword yang ditebak.
--
-- Tabelnya diisi ulang setiap /ingest lewat ts_stat, jadi angkanya selalu
-- menggambarkan korpus yang benar-benar ter-index sekarang.
CREATE TABLE rag_lexemes (
    lexeme   TEXT PRIMARY KEY,
    doc_freq INTEGER NOT NULL
);
