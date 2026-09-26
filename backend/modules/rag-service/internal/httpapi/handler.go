package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/rag-service/internal/llm"
	"github.com/enterprise-digital-platform/rag-service/internal/metrics"
)

// Handler menyimpan answerer sebagai ANTARMUKA yang boleh nil. nil berarti
// "tidak ada penyedia LLM yang dikonfigurasi", dan itu keadaan yang sah, bukan
// kerusakan -- lihat ask.go soal bagaimana /ask berperilaku saat itu terjadi.
type Handler struct {
	pool      *pgxpool.Pool
	answerer  llm.Answerer
	corpusDir string
	topK      int
}

func NewHandler(pool *pgxpool.Pool, answerer llm.Answerer, corpusDir string, topK int) *Handler {
	if topK <= 0 {
		topK = 5
	}
	return &Handler{pool: pool, answerer: answerer, corpusDir: corpusDir, topK: topK}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.health)
	mux.Handle("GET /metrics", metrics.Handler())

	mux.HandleFunc("POST /ingest", h.ingest)
	mux.HandleFunc("GET /documents", h.listDocuments)
	mux.HandleFunc("POST /ask", h.ask)
	mux.HandleFunc("GET /queries", h.listQueries)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	// llm_configured ikut dilaporkan supaya "kenapa chatbotnya cuma
	// mengembalikan kutipan" bisa dijawab tanpa membaca log container.
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"service":        "rag-service",
		"llm_configured": h.answerer != nil,
	})
}

func actorFromHeader(r *http.Request) *string {
	if v := r.Header.Get("X-User-Id"); v != "" {
		return &v
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
