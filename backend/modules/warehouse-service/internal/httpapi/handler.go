package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/warehouse-service/internal/eventbus"
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

	mux.HandleFunc("GET /products", h.listProducts)
	mux.HandleFunc("POST /products", h.createProduct)
	mux.HandleFunc("PUT /products/{id}", h.updateProduct)

	mux.HandleFunc("GET /warehouses", h.listWarehouses)
	mux.HandleFunc("POST /warehouses", h.createWarehouse)
	mux.HandleFunc("PUT /warehouses/{id}", h.updateWarehouse)

	mux.HandleFunc("GET /stock", h.listStockBalances)
	mux.HandleFunc("GET /stock-movements", h.listStockMovements)
	mux.HandleFunc("POST /stock-movements", h.createManualStockMovement)
	mux.HandleFunc("POST /stock-movements/batch", h.postStockMovementBatch)

	mux.HandleFunc("GET /stock-transfers", h.listStockTransfers)
	mux.HandleFunc("POST /stock-transfers", h.createStockTransfer)
	mux.HandleFunc("GET /stock-transfers/{id}", h.getStockTransfer)
	mux.HandleFunc("POST /stock-transfers/{id}/confirm", h.confirmStockTransfer)

	mux.HandleFunc("GET /stock-opnames", h.listStockOpnames)
	mux.HandleFunc("POST /stock-opnames", h.createStockOpname)
	mux.HandleFunc("GET /stock-opnames/{id}", h.getStockOpname)
	mux.HandleFunc("POST /stock-opnames/{id}/post", h.postStockOpname)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "warehouse-service"})
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
		SourceService: "warehouse-service",
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
