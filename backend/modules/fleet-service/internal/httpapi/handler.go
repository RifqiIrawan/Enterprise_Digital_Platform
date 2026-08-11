package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/fleet-service/internal/ecommerceclient"
	"github.com/enterprise-digital-platform/fleet-service/internal/eventbus"
	"github.com/enterprise-digital-platform/fleet-service/internal/metrics"
)

type Handler struct {
	pool      *pgxpool.Pool
	events    *eventbus.Publisher
	ecommerce *ecommerceclient.Client
}

func NewHandler(pool *pgxpool.Pool, events *eventbus.Publisher, ecommerce *ecommerceclient.Client) *Handler {
	return &Handler{pool: pool, events: events, ecommerce: ecommerce}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.health)
	mux.Handle("GET /metrics", metrics.Handler())

	mux.HandleFunc("GET /vehicles", h.listVehicles)
	mux.HandleFunc("POST /vehicles", h.createVehicle)
	mux.HandleFunc("PUT /vehicles/{id}", h.updateVehicle)

	mux.HandleFunc("GET /drivers", h.listDrivers)
	mux.HandleFunc("POST /drivers", h.createDriver)
	mux.HandleFunc("PUT /drivers/{id}", h.updateDriver)

	mux.HandleFunc("GET /delivery-orders", h.listDeliveryOrders)
	mux.HandleFunc("POST /delivery-orders", h.createDeliveryOrder)
	mux.HandleFunc("GET /delivery-orders/{id}", h.getDeliveryOrder)
	mux.HandleFunc("PUT /delivery-orders/{id}", h.updateDeliveryOrder)
	mux.HandleFunc("POST /delivery-orders/{id}/dispatch", h.dispatchDeliveryOrder)
	mux.HandleFunc("POST /delivery-orders/{id}/deliver", h.deliverDeliveryOrder)
	mux.HandleFunc("POST /delivery-orders/{id}/cancel", h.cancelDeliveryOrder)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "fleet-service"})
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
		SourceService: "fleet-service",
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
