package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/enterprise-digital-platform/ai-bi-service/internal/config"
)

type Handler struct {
	cfg    *config.Config
	client *http.Client
}

func NewHandler(cfg *config.Config) *Handler {
	return &Handler{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /dashboards/summary", h.dashboardSummary)
	mux.HandleFunc("GET /forecasting/summary", h.forecastingSummary)
	mux.HandleFunc("GET /anomaly-detection/scan", h.anomalyScan)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "ai-bi-service"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
