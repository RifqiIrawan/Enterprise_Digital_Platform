package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/finance-service/internal/eventbus"
	"github.com/enterprise-digital-platform/finance-service/internal/metrics"
)

type Handler struct {
	pool   *pgxpool.Pool
	events *eventbus.Publisher
}

func NewHandler(pool *pgxpool.Pool, events *eventbus.Publisher) *Handler {
	return &Handler{pool: pool, events: events}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.health)
	mux.Handle("GET /metrics", metrics.Handler())

	mux.HandleFunc("GET /accounts", h.listAccounts)
	mux.HandleFunc("POST /accounts", h.createAccount)
	mux.HandleFunc("PUT /accounts/{id}", h.updateAccount)

	mux.HandleFunc("GET /journal-entries", h.listJournalEntries)
	mux.HandleFunc("POST /journal-entries", h.createJournalEntry)
	mux.HandleFunc("GET /journal-entries/{id}", h.getJournalEntry)
	mux.HandleFunc("POST /journal-entries/{id}/post", h.postJournalEntry)

	mux.HandleFunc("GET /invoices", h.listInvoices)
	mux.HandleFunc("POST /invoices", h.createInvoice)
	mux.HandleFunc("GET /invoices/{id}", h.getInvoice)
	mux.HandleFunc("POST /invoices/{id}/post", h.postInvoice)

	mux.HandleFunc("GET /ar-ap-summary", h.arApSummary)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "finance-service"})
}

// auditEvent adalah amplop event yang dipublikasikan ke Kafka dan dikonsumsi
// oleh audit-service (lihat backend/services/audit-service/internal/model.AuditEvent).
type auditEvent struct {
	EventID       string    `json:"event_id"`
	EventType     string    `json:"event_type"`
	SourceService string    `json:"source_service"`
	OccurredAt    time.Time `json:"occurred_at"`
	ActorUserID   *string   `json:"actor_user_id,omitempty"`
	CompanyID     *string   `json:"company_id,omitempty"`
	Action        string    `json:"action"`
	EntityType    string    `json:"entity_type"`
	EntityID      string    `json:"entity_id"`
	Payload       any       `json:"payload,omitempty"`
}

func newAuditEvent(eventType string, actorUserID, companyID *string, action, entityType, entityID string, payload any) auditEvent {
	return auditEvent{
		EventID:       uuid.NewString(),
		EventType:     eventType,
		SourceService: "finance-service",
		OccurredAt:    time.Now(),
		ActorUserID:   actorUserID,
		CompanyID:     companyID,
		Action:        action,
		EntityType:    entityType,
		EntityID:      entityID,
		Payload:       payload,
	}
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
