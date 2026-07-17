package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/production-service/internal/eventbus"
	"github.com/enterprise-digital-platform/production-service/internal/metrics"
	"github.com/enterprise-digital-platform/production-service/internal/warehouseclient"
)

type Handler struct {
	pool      *pgxpool.Pool
	events    *eventbus.Publisher
	warehouse *warehouseclient.Client
}

func NewHandler(pool *pgxpool.Pool, events *eventbus.Publisher, warehouse *warehouseclient.Client) *Handler {
	return &Handler{pool: pool, events: events, warehouse: warehouse}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.health)
	mux.Handle("GET /metrics", metrics.Handler())

	mux.HandleFunc("GET /boms", h.listBOMs)
	mux.HandleFunc("POST /boms", h.createBOM)
	mux.HandleFunc("GET /boms/{id}", h.getBOM)
	mux.HandleFunc("PUT /boms/{id}", h.updateBOM)

	mux.HandleFunc("GET /work-orders", h.listWorkOrders)
	mux.HandleFunc("POST /work-orders", h.createWorkOrder)
	mux.HandleFunc("GET /work-orders/{id}", h.getWorkOrder)
	mux.HandleFunc("POST /work-orders/{id}/start", h.startWorkOrder)
	mux.HandleFunc("POST /work-orders/{id}/complete", h.completeWorkOrder)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "production-service"})
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
		SourceService: "production-service",
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

func headerValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
