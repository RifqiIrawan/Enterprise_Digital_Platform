package httpapi

import (
	"context"

	"github.com/enterprise-digital-platform/rag-service/internal/llm"
)

// retrievedChunk adalah satu kutipan beserta ALASAN dia terpilih. Score,
// MatchType, dan MatchedTerms ikut dikirim ke UI dengan sengaja: chatbot yang
// cuma menampilkan jawaban memaksa orang mempercayainya, sementara chatbot yang
// menunjukkan potongan mana yang dipakai, kata apa yang mencocokkannya, dan
// seberapa kuat kecocokannya bisa diperiksa. Ini pola yang sama dengan skor
// predictive-maintenance di ai-bi-service: heuristik yang DIJELASKAN, bukan
// angka yang turun dari langit.
type retrievedChunk struct {
	ChunkID    string  `json:"chunk_id"`
	SourcePath string  `json:"source_path"`
	Title      string  `json:"title"`
	Heading    string  `json:"heading"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
	// Coverage: bagian bobot pertanyaan yang benar-benar ditemukan di potongan
	// ini (0..1). Skor mentah tidak bisa dibandingkan antar pertanyaan;
	// coverage bisa, dan itulah yang dipakai memutuskan "korpus ini memang
	// tidak membahasnya".
	Coverage     float64  `json:"coverage"`
	MatchType    string   `json:"match_type"`
	MatchedTerms []string `json:"matched_terms"`
}

// Pencariannya dirakit sendiri, bukan diserahkan ke ts_rank_cd, dan itu
// keputusan yang lahir dari menjalankannya terhadap dokumentasi sungguhan
// (21 berkas, 188 potongan) alih-alih korpus contoh di test:
//
//  1. websearch_to_tsquery/plainto_tsquery menyambung kata dengan DAN.
//     Pertanyaan manusia hampir selalu memuat kata yang tidak ada di dokumen
//     ("bagaimana", "kalau", "untuk apa"), jadi satu kata meleset dan seluruh
//     pencarian mengembalikan nol -- di UI itu terbaca sebagai "dokumentasinya
//     tidak membahas itu" padahal membahas.
//  2. Menyambung dengan ATAU saja memberi hasil sebaliknya: kata umum seperti
//     "cara" atau "sistem" mencocokkan hampir semua potongan, sehingga
//     "resep rendang" pun mengembalikan kutipan.
//  3. ts_rank_cd tidak tahu kata mana yang langka. Potongan yang cuma
//     mencocokkan "cara" bisa mengalahkan potongan yang mencocokkan "OEE" --
//     dan itulah yang benar-benar terjadi saat dicoba.
//
// Jadi skornya dihitung dari IDF: tiap kata pertanyaan diberi bobot
// ln(1 + jumlah_potongan / frekuensi_dokumennya), lalu skor sebuah potongan =
// jumlah bobot kata yang benar-benar dia muat. Kata yang ada di mana-mana
// bernilai mendekati nol dengan sendirinya; kata khas seperti "OEE" atau
// "kafka" mendominasi. Kata yang lebih umum dari commonLexemeShare dibuang
// lebih dulu supaya tidak ikut menyeret potongan yang tidak nyambung.
//
// headingBoost: kata yang muncul di JUDUL bagian dihitung 1,5x. Judul bagian
// ("Rumus OEE", "Penyusutan Aset") biasanya justru istilah yang diketik orang,
// dan potongan yang judulnya cocok hampir selalu lebih tepat daripada potongan
// yang cuma menyebut istilah itu sekali di tengah paragraf.
const ftsSQL = `
	WITH total AS (
	    SELECT GREATEST(count(*), 1)::float8 AS chunks FROM rag_chunks
	), stop AS (
	    -- Daftar kata tanya/penghubung DISTEM dengan analyzer yang sama seperti
	    -- pertanyaannya, jadi "dijalankan" dan "jalankan" tidak perlu ditulis
	    -- dua kali dan tidak ada risiko daftarnya berisi bentuk yang tidak
	    -- pernah cocok.
	    SELECT DISTINCT lexeme AS word FROM unnest(to_tsvector('indonesian', $5))
	), terms AS (
	    SELECT DISTINCT lexeme FROM unnest(to_tsvector('indonesian', $1))
	    WHERE lexeme NOT IN (SELECT word FROM stop)
	), weighted AS (
	    SELECT t.lexeme,
	           to_tsquery('indonesian', quote_literal(t.lexeme)) AS tsq,
	           ln(1 + total.chunks / GREATEST(COALESCE(l.doc_freq, 0), 1)::float8) AS idf
	    FROM terms t
	    LEFT JOIN rag_lexemes l ON l.lexeme = t.lexeme
	    CROSS JOIN total
	    WHERE COALESCE(l.doc_freq, 0) <= total.chunks * $3
	), mass AS (
	    -- Bobot maksimum yang bisa diraih pertanyaan ini: jumlah IDF SELURUH
	    -- katanya, termasuk kata yang ternyata tidak ada di korpus sama sekali.
	    SELECT GREATEST(sum(idf), 0.0001) AS total_idf FROM weighted
	), scored AS (
	    SELECT c.id,
	           sum(w.idf * CASE WHEN to_tsvector('indonesian', c.heading) @@ w.tsq THEN $4::float8 ELSE 1 END) AS score,
	           array_agg(w.lexeme ORDER BY w.idf DESC) AS matched
	    FROM rag_chunks c
	    JOIN weighted w ON c.search_vector @@ w.tsq
	    GROUP BY c.id
	)
	SELECT c.id::text, d.source_path, d.title, c.heading, c.content, s.score, s.matched,
	       (s.score / mass.total_idf)::float8 AS coverage
	FROM scored s
	JOIN rag_chunks c ON c.id = s.id
	JOIN rag_documents d ON d.id = c.document_id
	CROSS JOIN mass
	ORDER BY s.score DESC, c.char_count ASC
	LIMIT $2`

// commonLexemeShare: hanya kata yang muncul di LEBIH DARI 60% potongan yang
// dibuang. Ambang ini sempat 25%, dan itu keliru: "kafka" muncul di banyak
// bagian dokumentasi justru KARENA dia topik yang sering dibahas, lalu ikut
// terbuang -- pertanyaan tentang Kafka kehilangan kata kuncinya sendiri.
// Setelah daftar kata tanya dipisahkan (questionWords), penyaringan frekuensi
// tidak perlu galak lagi: kata umum yang tersisa sudah ditekan sendiri oleh
// bobot IDF-nya yang kecil. Yang masih pantas dibuang hanya kata yang ada di
// hampir setiap potongan, yang IDF-nya mendekati nol dan cuma menambah
// kandidat tanpa menambah sinyal.
const commonLexemeShare = 0.6

const headingBoost = 1.5

// questionWords: konfigurasi 'indonesian' bawaan Postgres melakukan stemming
// tapi TIDAK punya daftar stopword -- ini ketahuan saat pencariannya dicoba
// terhadap dokumentasi sungguhan: "apa", "itu", "kapan", "cara" muncul di
// belasan potongan saja (di bawah ambang commonLexemeShare), jadi mereka lolos
// sebagai kata "jarang" lalu mendominasi skor. Akibatnya pertanyaan "apa itu
// work order" menemukan bagian berjudul "Kapan Data Ditulis ke Lake".
//
// Menurunkan commonLexemeShare tidak bisa memperbaikinya: ambang yang cukup
// rendah untuk membuang "cara" juga membuang istilah khas seperti "kafka".
// Yang membedakan keduanya bukan frekuensi, melainkan JENIS katanya -- kata
// tanya dan kata penghubung tidak pernah jadi sinyal tentang isi dokumen.
// Daftarnya sengaja hanya berisi kata fungsi, bukan istilah domain apa pun.
const questionWords = "apa apakah siapa kapan dimana mana bagaimana gimana kenapa mengapa berapa " +
	"itu ini yang dan atau kalau jika untuk dari ke pada dengan dalam oleh " +
	"cara saja juga bisa dapat ada adalah akan sudah tolong jelaskan sebutkan " +
	"nya nya-nya tentang terhadap sebuah suatu para lalu kemudian"

// Ambang 0,25: di bawah itu trigram mulai mencocokkan kata yang kebetulan
// berbagi tiga huruf, dan kutipan yang tidak nyambung lebih buruk daripada
// mengaku tidak menemukan apa-apa.
const trigramSQL = `
	SELECT c.id::text, d.source_path, d.title, c.heading, c.content,
	       similarity(c.heading, $1)::float8, ARRAY[]::text[], similarity(c.heading, $1)::float8
	FROM rag_chunks c
	JOIN rag_documents d ON d.id = c.document_id
	WHERE similarity(c.heading, $1) > 0.25
	ORDER BY 6 DESC
	LIMIT $2`

// retrieve mencari dua tahap. Tahap kedua (kemiripan trigram pada judul
// bagian, untuk salah ketik seperti "depresisi") hanya berjalan kalau tahap
// pertama tidak menemukan apa pun. Dipisah sebagai cadangan, bukan digabung
// jadi satu skor, karena kedua angka itu tidak sebanding -- menjumlahkannya
// menghasilkan peringkat yang tidak berarti apa-apa, dan MatchType di respons
// jadi bohong.
func (h *Handler) retrieve(ctx context.Context, question string, topK int) ([]retrievedChunk, error) {
	out, err := h.queryChunks(ctx, ftsSQL, "fts", question, topK, commonLexemeShare, headingBoost, questionWords)
	if err != nil {
		return nil, err
	}
	if len(out) > 0 {
		// Potongan terbaik yang cuma menutup sebagian kecil bobot pertanyaan
		// berarti korpusnya menyerempet, bukan menjawab -- mis. pertanyaan
		// tentang OEE yang hanya mencocokkan kata "hitung". Mengembalikannya
		// sebagai kutipan membuat orang mengira dokumentasinya sudah dibaca
		// dan memang tidak memuat jawabannya, padahal yang terjadi adalah
		// pencariannya yang meleset.
		if out[0].Coverage < minCoverage {
			return []retrievedChunk{}, nil
		}
		return dropWeakMatches(out), nil
	}
	return h.queryChunks(ctx, trigramSQL, "trigram", question, topK)
}

// relativeScoreFloor: karena kata disambung dengan ATAU, potongan yang hanya
// mencocokkan satu kata berbobot rendah ikut terjaring. Ambangnya RELATIF
// terhadap hasil teratas, bukan angka mutlak -- skor IDF tidak punya satuan
// yang bisa dibandingkan antar pertanyaan, jadi "sepertiga sekuat yang
// terbaik" berarti sesuatu sementara "di atas 2,0" tidak.
const relativeScoreFloor = 0.34

// minCoverage: potongan teratas wajib menutup setidaknya 45% bobot pertanyaan.
// Di bawah itu yang cocok biasanya cuma satu kata kerja umum, dan jawaban
// "tidak ditemukan" lebih jujur daripada kutipan yang menyerempet.
const minCoverage = 0.45

func dropWeakMatches(chunks []retrievedChunk) []retrievedChunk {
	if len(chunks) == 0 {
		return chunks
	}
	best := chunks[0].Score
	if best <= 0 {
		return chunks
	}
	kept := make([]retrievedChunk, 0, len(chunks))
	for _, c := range chunks {
		if c.Score >= best*relativeScoreFloor {
			kept = append(kept, c)
		}
	}
	return kept
}

func (h *Handler) queryChunks(ctx context.Context, sql, matchType, question string, topK int, extraArgs ...any) ([]retrievedChunk, error) {
	args := append([]any{question, topK}, extraArgs...)
	rows, err := h.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []retrievedChunk{}
	for rows.Next() {
		var c retrievedChunk
		if err := rows.Scan(&c.ChunkID, &c.SourcePath, &c.Title, &c.Heading, &c.Content, &c.Score, &c.MatchedTerms, &c.Coverage); err != nil {
			return nil, err
		}
		c.MatchType = matchType
		out = append(out, c)
	}
	return out, rows.Err()
}

// toPassages memberi nomor kutipan ([1], [2], ...) sesuai urutan peringkat.
// Nomor ditentukan di sini, bukan oleh model, supaya "[2]" di teks jawaban
// selalu menunjuk sumber kedua yang ditampilkan UI.
func toPassages(chunks []retrievedChunk) []llm.Passage {
	passages := make([]llm.Passage, 0, len(chunks))
	for i, c := range chunks {
		passages = append(passages, llm.Passage{
			Ref:        i + 1,
			SourcePath: c.SourcePath,
			Title:      c.Title,
			Heading:    c.Heading,
			Content:    c.Content,
		})
	}
	return passages
}
