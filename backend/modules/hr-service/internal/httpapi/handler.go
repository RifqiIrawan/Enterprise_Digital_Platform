package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/hr-service/internal/eventbus"
	"github.com/enterprise-digital-platform/hr-service/internal/financeclient"
	"github.com/enterprise-digital-platform/hr-service/internal/metrics"
)

type Handler struct {
	pool    *pgxpool.Pool
	events  *eventbus.Publisher
	finance *financeclient.Client
}

func NewHandler(pool *pgxpool.Pool, events *eventbus.Publisher, finance *financeclient.Client) *Handler {
	return &Handler{pool: pool, events: events, finance: finance}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.health)
	mux.Handle("GET /metrics", metrics.Handler())

	mux.HandleFunc("GET /employees", h.listEmployees)
	mux.HandleFunc("POST /employees", h.createEmployee)
	mux.HandleFunc("GET /employees/{id}", h.getEmployee)
	mux.HandleFunc("PUT /employees/{id}", h.updateEmployee)

	mux.HandleFunc("GET /attendance", h.listAttendance)
	mux.HandleFunc("POST /attendance", h.createAttendance)
	mux.HandleFunc("PUT /attendance/{id}", h.updateAttendance)

	mux.HandleFunc("GET /leave-requests", h.listLeaveRequests)
	mux.HandleFunc("POST /leave-requests", h.createLeaveRequest)
	mux.HandleFunc("PUT /leave-requests/{id}", h.updateLeaveRequest)
	mux.HandleFunc("POST /leave-requests/{id}/submit", h.submitLeaveRequest)
	mux.HandleFunc("POST /leave-requests/{id}/approve", h.approveLeaveRequest)
	mux.HandleFunc("POST /leave-requests/{id}/reject", h.rejectLeaveRequest)
	mux.HandleFunc("POST /leave-requests/{id}/cancel", h.cancelLeaveRequest)

	mux.HandleFunc("GET /overtime", h.listOvertime)
	mux.HandleFunc("POST /overtime", h.createOvertime)
	mux.HandleFunc("PUT /overtime/{id}", h.updateOvertime)
	mux.HandleFunc("POST /overtime/{id}/approve", h.approveOvertime)
	mux.HandleFunc("POST /overtime/{id}/reject", h.rejectOvertime)

	mux.HandleFunc("GET /holidays", h.listHolidays)
	mux.HandleFunc("POST /holidays", h.createHoliday)
	mux.HandleFunc("DELETE /holidays/{id}", h.deleteHoliday)

	mux.HandleFunc("GET /leave-quotas", h.listLeaveQuotas)
	mux.HandleFunc("PUT /leave-quotas", h.putLeaveQuota)

	mux.HandleFunc("GET /kpi-indicators", h.listKPIIndicators)
	mux.HandleFunc("POST /kpi-indicators", h.createKPIIndicator)
	mux.HandleFunc("PUT /kpi-indicators/{id}", h.updateKPIIndicator)
	mux.HandleFunc("DELETE /kpi-indicators/{id}", h.deleteKPIIndicator)

	mux.HandleFunc("GET /kpi-reviews", h.listKPIReviews)
	mux.HandleFunc("POST /kpi-reviews", h.createKPIReview)
	mux.HandleFunc("GET /kpi-reviews/{id}", h.getKPIReview)
	mux.HandleFunc("PUT /kpi-reviews/{id}/scores", h.putKPIScores)
	mux.HandleFunc("POST /kpi-reviews/{id}/submit", h.submitKPIReview)
	mux.HandleFunc("POST /kpi-reviews/{id}/approve", h.approveKPIReview)
	mux.HandleFunc("POST /kpi-reviews/{id}/reject", h.rejectKPIReview)

	mux.HandleFunc("GET /payroll-runs", h.listPayrollRuns)
	mux.HandleFunc("POST /payroll-runs", h.processPayroll)
	mux.HandleFunc("GET /payroll-runs/{id}", h.getPayrollRun)
	mux.HandleFunc("POST /payroll-runs/{id}/post", h.postPayrollRun)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "hr-service"})
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
		SourceService: "hr-service",
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
