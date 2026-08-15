package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/project-service/internal/eventbus"
	"github.com/enterprise-digital-platform/project-service/internal/financeclient"
	"github.com/enterprise-digital-platform/project-service/internal/hrclient"
	"github.com/enterprise-digital-platform/project-service/internal/metrics"
)

type Handler struct {
	pool    *pgxpool.Pool
	events  *eventbus.Publisher
	hr      *hrclient.Client
	finance *financeclient.Client
}

func NewHandler(pool *pgxpool.Pool, events *eventbus.Publisher, hr *hrclient.Client, finance *financeclient.Client) *Handler {
	return &Handler{pool: pool, events: events, hr: hr, finance: finance}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.health)
	mux.Handle("GET /metrics", metrics.Handler())

	mux.HandleFunc("GET /projects", h.listProjects)
	mux.HandleFunc("POST /projects", h.createProject)
	mux.HandleFunc("GET /projects/{id}", h.getProject)
	mux.HandleFunc("PUT /projects/{id}", h.updateProject)
	mux.HandleFunc("POST /projects/{id}/activate", h.activateProject)
	mux.HandleFunc("POST /projects/{id}/hold", h.holdProject)
	mux.HandleFunc("POST /projects/{id}/complete", h.completeProject)
	mux.HandleFunc("POST /projects/{id}/cancel", h.cancelProject)
	mux.HandleFunc("POST /projects/{id}/post-cost", h.postProjectCost)

	mux.HandleFunc("GET /tasks", h.listTasks)
	mux.HandleFunc("POST /tasks", h.createTask)
	mux.HandleFunc("PUT /tasks/{id}", h.updateTask)

	mux.HandleFunc("GET /timesheets", h.listTimesheets)
	mux.HandleFunc("POST /timesheets", h.createTimesheet)
	mux.HandleFunc("POST /timesheets/{id}/approve", h.approveTimesheet)
	mux.HandleFunc("POST /timesheets/{id}/reject", h.rejectTimesheet)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "project-service"})
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
		SourceService: "project-service",
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
