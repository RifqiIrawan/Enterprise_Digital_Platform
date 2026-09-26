package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

type askRequest struct {
	Question  string  `json:"question"`
	CompanyID *string `json:"company_id"`
	TopK      int     `json:"top_k"`
}

type askResponse struct {
	Question string `json:"question"`
	// Outcome membedakan tiga keadaan yang di UI mudah tertukar: ANSWERED
	// (model menjawab), RETRIEVED_ONLY (kutipan ketemu, tapi tidak ada LLM
	// yang dikonfigurasi), NO_MATCH (korpus tidak punya bahannya).
	Outcome      string           `json:"outcome"`
	Answer       string           `json:"answer"`
	Sources      []retrievedChunk `json:"sources"`
	Model        string           `json:"model,omitempty"`
	InputTokens  int              `json:"input_tokens,omitempty"`
	OutputTokens int              `json:"output_tokens,omitempty"`
	LatencyMS    int              `json:"latency_ms"`
	// Note selalu terisi untuk outcome selain ANSWERED, dan berisi kalimat
	// yang layak ditampilkan apa adanya ke pengguna -- bukan kode galat yang
	// harus diterjemahkan frontend.
	Note string `json:"note,omitempty"`
	// RetrievalNote menyebut batas metode pencariannya di SETIAP jawaban.
	// Ditaruh di respons, bukan cuma di dokumentasi, karena yang perlu tahu
	// "ini pencarian kata, bukan pemahaman makna" adalah orang yang sedang
	// membaca jawabannya.
	RetrievalNote string `json:"retrieval_note"`
}

const (
	outcomeAnswered      = "ANSWERED"
	outcomeRetrievedOnly = "RETRIEVED_ONLY"
	outcomeNoMatch       = "NO_MATCH"
	outcomeError         = "ERROR"
)

// ask adalah jalur utama service ini: cari bahan, lalu (kalau ada penyedia
// LLM) susun jawaban dari bahan itu.
//
// Dua keputusan yang membentuk seluruh handler ini:
//
//   - Korpus yang tidak punya bahan TIDAK dikirim ke model. Memanggil LLM
//     dengan nol kutipan hanya memancing jawaban dari ingatan model tentang
//     ERP pada umumnya -- terdengar meyakinkan, dan tidak ada hubungannya
//     dengan platform ini. Lebih baik mengaku tidak tahu, gratis pula.
//   - Tanpa kunci API, ini bukan galat. Kutipan tetap dikembalikan; yang hilang
//     hanya paragraf naratifnya, dan alasannya disebut di `note`.
func (h *Handler) ask(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	ctx := r.Context()

	var req askRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		writeError(w, http.StatusBadRequest, "question wajib diisi")
		return
	}
	// Batas panjang pertanyaan: bukan soal token, tapi supaya kolom pertanyaan
	// tidak dipakai menempelkan dokumen utuh yang lalu ikut terkirim ke model
	// sebagai input berbayar.
	if len([]rune(req.Question)) > 1000 {
		writeError(w, http.StatusBadRequest, "question terlalu panjang (maksimal 1000 karakter)")
		return
	}
	topK := h.topK
	if req.TopK > 0 && req.TopK <= 10 {
		topK = req.TopK
	}

	chunks, err := h.retrieve(ctx, req.Question, topK)
	if err != nil {
		h.logQuery(ctx, req, actorFromHeader(r), outcomeError, nil, "", 0, 0, time.Since(started), err.Error())
		writeError(w, http.StatusInternalServerError, "Gagal mencari dokumentasi: "+err.Error())
		return
	}

	resp := askResponse{
		Question:      req.Question,
		Sources:       chunks,
		RetrievalNote: retrievalNote,
	}

	if len(chunks) == 0 {
		resp.Outcome = outcomeNoMatch
		resp.Note = "Tidak ada bagian dokumentasi yang cocok dengan pertanyaan ini, jadi pertanyaannya tidak dikirim ke model. Coba pakai istilah yang dipakai di aplikasi (mis. \"work order\", \"jurnal\", \"OEE\")."
		resp.LatencyMS = int(time.Since(started).Milliseconds())
		h.logQuery(ctx, req, actorFromHeader(r), outcomeNoMatch, chunks, "", 0, 0, time.Since(started), "")
		writeJSON(w, http.StatusOK, resp)
		return
	}

	if h.answerer == nil {
		resp.Outcome = outcomeRetrievedOnly
		resp.Note = "Penyedia LLM belum dikonfigurasi (ANTHROPIC_API_KEY kosong), jadi yang ditampilkan adalah kutipan dokumentasi yang paling cocok, tanpa jawaban yang disusun model."
		resp.LatencyMS = int(time.Since(started).Milliseconds())
		h.logQuery(ctx, req, actorFromHeader(r), outcomeRetrievedOnly, chunks, "", 0, 0, time.Since(started), "")
		writeJSON(w, http.StatusOK, resp)
		return
	}

	result, err := h.answerer.Answer(ctx, req.Question, toPassages(chunks))
	if err != nil {
		// Panggilan model gagal, tapi kutipannya sudah ada di tangan --
		// membuangnya dan mengembalikan 502 kosong akan membuang satu-satunya
		// bagian yang masih berguna.
		resp.Outcome = outcomeRetrievedOnly
		resp.Note = "Model tidak bisa dihubungi (" + err.Error() + "), jadi yang ditampilkan hanya kutipan dokumentasi yang paling cocok."
		resp.LatencyMS = int(time.Since(started).Milliseconds())
		h.logQuery(ctx, req, actorFromHeader(r), outcomeError, chunks, h.answerer.Model(), 0, 0, time.Since(started), err.Error())
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if result.Refused {
		resp.Outcome = outcomeRetrievedOnly
		resp.Note = "Model menolak menjawab pertanyaan ini (kategori: " + result.RefusalReason + "). Kutipan dokumentasinya tetap ditampilkan."
		resp.Model = result.Model
		resp.LatencyMS = int(time.Since(started).Milliseconds())
		h.logQuery(ctx, req, actorFromHeader(r), outcomeRetrievedOnly, chunks, result.Model, result.InputTokens, result.OutputTokens, time.Since(started), "refusal: "+result.RefusalReason)
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resp.Outcome = outcomeAnswered
	resp.Answer = result.Text
	resp.Model = result.Model
	resp.InputTokens = result.InputTokens
	resp.OutputTokens = result.OutputTokens
	resp.LatencyMS = int(time.Since(started).Milliseconds())
	h.logQuery(ctx, req, actorFromHeader(r), outcomeAnswered, chunks, result.Model, result.InputTokens, result.OutputTokens, time.Since(started), "")
	writeJSON(w, http.StatusOK, resp)
}

// logQuery mencatat setiap pertanyaan, termasuk yang gagal. Kegagalan
// mencatat TIDAK membatalkan jawaban yang sudah berhasil disusun -- jejak yang
// hilang jauh lebih murah daripada jawaban yang hilang.
func (h *Handler) logQuery(ctx context.Context, req askRequest, userID *string, outcome string, chunks []retrievedChunk, model string, inTok, outTok int, latency time.Duration, errMessage string) {
	ids := make([]string, 0, len(chunks))
	for _, c := range chunks {
		ids = append(ids, c.ChunkID)
	}
	_, err := h.pool.Exec(ctx, `
		INSERT INTO rag_queries (company_id, user_id, question, outcome, chunk_ids, model, input_tokens, output_tokens, latency_ms, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		req.CompanyID, userID, req.Question, outcome, ids, model, inTok, outTok, int(latency.Milliseconds()), errMessage,
	)
	if err != nil {
		// Sengaja tidak mengubah respons: pemanggil sudah (atau sedang)
		// menerima jawabannya.
		logQueryFailure(err)
	}
}

func logQueryFailure(err error) {
	log.Printf("rag-service: gagal mencatat rag_queries (jawaban tetap dikirim): %v", err)
}
