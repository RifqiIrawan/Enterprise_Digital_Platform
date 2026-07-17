package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/rbac-service/internal/eventbus"
	"github.com/enterprise-digital-platform/rbac-service/internal/metrics"
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

	mux.HandleFunc("GET /modules", h.listModules)
	mux.HandleFunc("GET /menus", h.listMenus)
	mux.HandleFunc("GET /menu-tree", h.menuTree)

	mux.HandleFunc("GET /roles", h.listRoles)
	mux.HandleFunc("POST /roles", h.createRole)
	mux.HandleFunc("PUT /roles/{id}", h.updateRole)
	mux.HandleFunc("DELETE /roles/{id}", h.deleteRole)
	mux.HandleFunc("GET /roles/{id}/permissions", h.getRolePermissions)
	mux.HandleFunc("PUT /roles/{id}/permissions", h.putRolePermissions)

	mux.HandleFunc("GET /user-roles", h.listUserRoles)
	mux.HandleFunc("POST /user-roles", h.assignUserRole)
	mux.HandleFunc("DELETE /user-roles/{id}", h.revokeUserRole)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "rbac-service"})
}

// auditEvent adalah amplop event yang dipublikasikan ke Kafka dan dikonsumsi
// oleh audit-service (lihat backend/services/audit-service/internal/model.AuditEvent).
type auditEvent struct {
	EventID       string    `json:"event_id"`
	EventType     string    `json:"event_type"`
	SourceService string    `json:"source_service"`
	OccurredAt    time.Time `json:"occurred_at"`
	CompanyID     *string   `json:"company_id,omitempty"`
	Action        string    `json:"action"`
	EntityType    string    `json:"entity_type"`
	EntityID      string    `json:"entity_id"`
	Payload       any       `json:"payload,omitempty"`
}

func newAuditEvent(eventType string, companyID *string, action, entityType, entityID string, payload any) auditEvent {
	return auditEvent{
		EventID:       uuid.NewString(),
		EventType:     eventType,
		SourceService: "rbac-service",
		OccurredAt:    time.Now(),
		CompanyID:     companyID,
		Action:        action,
		EntityType:    entityType,
		EntityID:      entityID,
		Payload:       payload,
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
