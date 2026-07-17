package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/sales-service/internal/eventbus"
	"github.com/enterprise-digital-platform/sales-service/internal/financeclient"
	"github.com/enterprise-digital-platform/sales-service/internal/metrics"
	"github.com/enterprise-digital-platform/sales-service/internal/warehouseclient"
)

type Handler struct {
	pool      *pgxpool.Pool
	events    *eventbus.Publisher
	finance   *financeclient.Client
	warehouse *warehouseclient.Client
}

func NewHandler(pool *pgxpool.Pool, events *eventbus.Publisher, finance *financeclient.Client, warehouse *warehouseclient.Client) *Handler {
	return &Handler{pool: pool, events: events, finance: finance, warehouse: warehouse}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.health)
	mux.Handle("GET /metrics", metrics.Handler())

	mux.HandleFunc("GET /customers", h.listCustomers)
	mux.HandleFunc("POST /customers", h.createCustomer)
	mux.HandleFunc("PUT /customers/{id}", h.updateCustomer)

	mux.HandleFunc("GET /quotations", h.listQuotations)
	mux.HandleFunc("POST /quotations", h.createQuotation)
	mux.HandleFunc("GET /quotations/{id}", h.getQuotation)
	mux.HandleFunc("POST /quotations/{id}/send", h.sendQuotation)
	mux.HandleFunc("POST /quotations/{id}/accept", h.acceptQuotation)
	mux.HandleFunc("POST /quotations/{id}/reject", h.rejectQuotation)
	mux.HandleFunc("POST /quotations/{id}/convert", h.convertQuotation)

	mux.HandleFunc("GET /sales-orders", h.listSalesOrders)
	mux.HandleFunc("POST /sales-orders", h.createSalesOrder)
	mux.HandleFunc("GET /sales-orders/{id}", h.getSalesOrder)
	mux.HandleFunc("POST /sales-orders/{id}/confirm", h.confirmSalesOrder)
	mux.HandleFunc("POST /sales-orders/{id}/fulfill", h.fulfillSalesOrder)
	mux.HandleFunc("POST /sales-orders/{id}/invoice", h.invoiceSalesOrder)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "sales-service"})
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
		SourceService: "sales-service",
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
