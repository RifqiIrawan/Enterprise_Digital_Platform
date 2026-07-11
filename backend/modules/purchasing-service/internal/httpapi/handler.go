package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/purchasing-service/internal/eventbus"
	"github.com/enterprise-digital-platform/purchasing-service/internal/financeclient"
	"github.com/enterprise-digital-platform/purchasing-service/internal/warehouseclient"
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

	mux.HandleFunc("GET /suppliers", h.listSuppliers)
	mux.HandleFunc("POST /suppliers", h.createSupplier)
	mux.HandleFunc("PUT /suppliers/{id}", h.updateSupplier)

	mux.HandleFunc("GET /requisitions", h.listRequisitions)
	mux.HandleFunc("POST /requisitions", h.createRequisition)
	mux.HandleFunc("GET /requisitions/{id}", h.getRequisition)
	mux.HandleFunc("POST /requisitions/{id}/submit", h.submitRequisition)
	mux.HandleFunc("POST /requisitions/{id}/approve", h.approveRequisition)
	mux.HandleFunc("POST /requisitions/{id}/reject", h.rejectRequisition)
	mux.HandleFunc("POST /requisitions/{id}/convert", h.convertRequisition)

	mux.HandleFunc("GET /purchase-orders", h.listPurchaseOrders)
	mux.HandleFunc("POST /purchase-orders", h.createPurchaseOrder)
	mux.HandleFunc("GET /purchase-orders/{id}", h.getPurchaseOrder)
	mux.HandleFunc("POST /purchase-orders/{id}/confirm", h.confirmPurchaseOrder)
	mux.HandleFunc("POST /purchase-orders/{id}/receive", h.receivePurchaseOrder)
	mux.HandleFunc("POST /purchase-orders/{id}/invoice", h.invoicePurchaseOrder)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "purchasing-service"})
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
		SourceService: "purchasing-service",
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
