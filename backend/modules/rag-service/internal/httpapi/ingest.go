package httpapi

import (
	"context"
	"net/http"
	"os"
	"strconv"

	"github.com/enterprise-digital-platform/rag-service/internal/corpus"
)

type ingestFileResult struct {
	SourcePath string `json:"source_path"`
	Title      string `json:"title"`
	Chunks     int    `json:"chunks"`
	// Status: INDEXED (baru), REINDEXED (isinya berubah), atau UNCHANGED
	// (hash-nya sama, tidak disentuh). Membedakan ketiganya membuat ingest
	// ulang bisa dibaca sebagai "tidak ada yang berubah", bukan sebagai
	// pekerjaan yang entah terjadi atau tidak.
	Status string `json:"status"`
}

type ingestResponse struct {
	CorpusDir     string             `json:"corpus_dir"`
	Documents     int                `json:"documents"`
	Chunks        int                `json:"chunks"`
	Changed       int                `json:"changed"`
	RemovedDocs   int                `json:"removed_documents"`
	Files         []ingestFileResult `json:"files"`
	RetrievalNote string             `json:"retrieval_note"`
}

const retrievalNote = "Pencarian memakai full-text search bahasa Indonesia bawaan Postgres (leksikal, bukan semantik): pertanyaan yang tidak memakai satu pun kata dari dokumennya bisa meleset."

// ingest membaca ulang seluruh korpus dan menyamakan isi database dengannya.
// Idempotent lewat content_hash: berkas yang tidak berubah tidak di-chunk
// ulang. Berkas yang HILANG dari korpus ikut dihapus dari index -- dokumentasi
// yang sudah dihapus tapi masih dikutip chatbot adalah cara paling halus untuk
// menyesatkan orang.
func (h *Handler) ingest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if _, err := os.Stat(h.corpusDir); err != nil {
		// Path-nya ikut disebut: di container ini hampir selalu berarti
		// folder dokumentasi belum di-mount, dan pesan tanpa path memaksa
		// orang menebak.
		writeError(w, http.StatusFailedDependency, "Folder korpus tidak bisa dibaca ("+h.corpusDir+"): "+err.Error())
		return
	}

	docs, err := corpus.Walk(h.corpusDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca korpus: "+err.Error())
		return
	}
	if len(docs) == 0 {
		writeError(w, http.StatusFailedDependency, "Tidak ada berkas .md di folder korpus ("+h.corpusDir+")")
		return
	}

	resp := ingestResponse{CorpusDir: h.corpusDir, Files: []ingestFileResult{}, RetrievalNote: retrievalNote}
	seen := make([]string, 0, len(docs))

	for _, doc := range docs {
		seen = append(seen, doc.SourcePath)

		var existingID, existingHash string
		err := h.pool.QueryRow(ctx,
			`SELECT id::text, content_hash FROM rag_documents WHERE source_path = $1`, doc.SourcePath,
		).Scan(&existingID, &existingHash)
		unchanged := err == nil && existingHash == doc.ContentHash
		if unchanged {
			resp.Documents++
			resp.Chunks += doc.ChunkCount()
			resp.Files = append(resp.Files, ingestFileResult{
				SourcePath: doc.SourcePath, Title: doc.Title, Chunks: doc.ChunkCount(), Status: "UNCHANGED",
			})
			continue
		}

		status := "INDEXED"
		if existingID != "" {
			status = "REINDEXED"
		}

		tx, err := h.pool.Begin(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
			return
		}

		var docID string
		// Satu dokumen = satu transaksi: potongan lama dihapus (CASCADE) dan
		// potongan baru ditulis bersama-sama, jadi tidak pernah ada momen
		// ketika sebuah dokumen ada tanpa isi yang bisa dicari.
		err = tx.QueryRow(ctx, `
			INSERT INTO rag_documents (source_path, title, content_hash, chunk_count, byte_size, indexed_at)
			VALUES ($1, $2, $3, $4, $5, now())
			ON CONFLICT (source_path) DO UPDATE
			SET title = EXCLUDED.title, content_hash = EXCLUDED.content_hash,
			    chunk_count = EXCLUDED.chunk_count, byte_size = EXCLUDED.byte_size, indexed_at = now()
			RETURNING id::text`,
			doc.SourcePath, doc.Title, doc.ContentHash, doc.ChunkCount(), doc.ByteSize,
		).Scan(&docID)
		if err != nil {
			_ = tx.Rollback(ctx)
			writeError(w, http.StatusInternalServerError, "Gagal menyimpan dokumen "+doc.SourcePath+": "+err.Error())
			return
		}
		if _, err := tx.Exec(ctx, `DELETE FROM rag_chunks WHERE document_id = $1`, docID); err != nil {
			_ = tx.Rollback(ctx)
			writeError(w, http.StatusInternalServerError, "Gagal membersihkan potongan lama: "+err.Error())
			return
		}
		for _, c := range doc.Chunks {
			if _, err := tx.Exec(ctx, `
				INSERT INTO rag_chunks (document_id, chunk_index, heading, content, char_count)
				VALUES ($1, $2, $3, $4, $5)`,
				docID, c.Index, c.Heading, c.Content, c.CharCount,
			); err != nil {
				_ = tx.Rollback(ctx)
				writeError(w, http.StatusInternalServerError, "Gagal menyimpan potongan: "+err.Error())
				return
			}
		}
		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal menyimpan dokumen "+doc.SourcePath)
			return
		}

		resp.Documents++
		resp.Chunks += doc.ChunkCount()
		resp.Changed++
		resp.Files = append(resp.Files, ingestFileResult{
			SourcePath: doc.SourcePath, Title: doc.Title, Chunks: doc.ChunkCount(), Status: status,
		})
	}

	removed, err := h.pool.Exec(ctx, `DELETE FROM rag_documents WHERE source_path <> ALL($1)`, seen)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membersihkan dokumen yang hilang dari korpus: "+err.Error())
		return
	}
	resp.RemovedDocs = int(removed.RowsAffected())

	if err := h.refreshLexemeStats(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghitung statistik kata korpus: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// refreshLexemeStats menghitung ulang berapa banyak potongan yang memuat tiap
// kata, dipakai pencarian untuk membuang kata yang terlalu umum dari
// pertanyaan (lihat retrieve.go). Dihitung sekali per ingest lewat ts_stat --
// menghitungnya per pertanyaan berarti memindai seluruh tsvector korpus setiap
// kali orang mengetik.
//
// Dijalankan dalam satu transaksi: tabel yang sempat kosong di tengah-tengah
// membuat pencarian menganggap SEMUA kata jarang, dan selama beberapa detik
// chatbot akan mengembalikan potongan acak tanpa ada yang tahu kenapa.
func (h *Handler) refreshLexemeStats(ctx context.Context) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM rag_lexemes`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO rag_lexemes (lexeme, doc_freq)
		SELECT word, ndoc FROM ts_stat('SELECT search_vector FROM rag_chunks')
		ON CONFLICT (lexeme) DO UPDATE SET doc_freq = EXCLUDED.doc_freq`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type documentView struct {
	ID         string `json:"id"`
	SourcePath string `json:"source_path"`
	Title      string `json:"title"`
	ChunkCount int    `json:"chunk_count"`
	ByteSize   int    `json:"byte_size"`
	IndexedAt  string `json:"indexed_at"`
}

func (h *Handler) listDocuments(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT id::text, source_path, title, chunk_count, byte_size, to_char(indexed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM rag_documents ORDER BY source_path ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat daftar dokumen")
		return
	}
	defer rows.Close()

	docs := []documentView{}
	for rows.Next() {
		var d documentView
		if err := rows.Scan(&d.ID, &d.SourcePath, &d.Title, &d.ChunkCount, &d.ByteSize, &d.IndexedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca daftar dokumen")
			return
		}
		docs = append(docs, d)
	}
	writeJSON(w, http.StatusOK, docs)
}

type queryView struct {
	ID           string `json:"id"`
	Question     string `json:"question"`
	Outcome      string `json:"outcome"`
	Model        string `json:"model"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	LatencyMS    int    `json:"latency_ms"`
	CreatedAt    string `json:"created_at"`
}

// listQueries membuat biaya dan perilaku chatbot bisa diperiksa: berapa yang
// benar-benar dijawab model, berapa yang tidak menemukan bahan, dan berapa
// token yang terpakai.
func (h *Handler) listQueries(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	args := []any{limit}
	where := ""
	if companyID := r.URL.Query().Get("company_id"); companyID != "" {
		args = append(args, companyID)
		where = ` WHERE company_id = $2`
	}

	// Waktu dikirim sebagai UTC dengan "Z", BUKAN to_char(..., 'OF'): 'OF'
	// menghasilkan "+07" tanpa menit, yang bukan RFC 3339 dan ditolak
	// new Date() di browser -- kolom Waktu di halaman riwayat jadi "Invalid Date".
	rows, err := h.pool.Query(r.Context(), `
		SELECT id::text, question, outcome, model, input_tokens, output_tokens, latency_ms,
		       to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM rag_queries`+where+` ORDER BY created_at DESC LIMIT $1`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat riwayat pertanyaan")
		return
	}
	defer rows.Close()

	out := []queryView{}
	for rows.Next() {
		var q queryView
		if err := rows.Scan(&q.ID, &q.Question, &q.Outcome, &q.Model, &q.InputTokens, &q.OutputTokens, &q.LatencyMS, &q.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca riwayat pertanyaan")
			return
		}
		out = append(out, q)
	}
	writeJSON(w, http.StatusOK, out)
}
