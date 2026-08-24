package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/asset-service/internal/eventbus"
	"github.com/enterprise-digital-platform/asset-service/internal/financeclient"
	"github.com/enterprise-digital-platform/asset-service/internal/metrics"
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

	mux.HandleFunc("GET /assets", h.listAssets)
	mux.HandleFunc("POST /assets", h.createAsset)
	mux.HandleFunc("PUT /assets/{id}", h.updateAsset)

	mux.HandleFunc("GET /maintenance-schedules", h.listMaintenanceSchedules)
	mux.HandleFunc("POST /maintenance-schedules", h.createMaintenanceSchedule)
	mux.HandleFunc("POST /maintenance-schedules/{id}/complete", h.completeMaintenanceSchedule)
	mux.HandleFunc("POST /maintenance-schedules/{id}/cancel", h.cancelMaintenanceSchedule)

	// Fase 5: penyusutan (dihitung per periode, lalu diposting ke GL) dan
	// kalibrasi (kewajiban berulang per aset).
	mux.HandleFunc("GET /depreciation-runs", h.listDepreciationRuns)
	mux.HandleFunc("POST /depreciation-runs", h.createDepreciationRun)
	mux.HandleFunc("GET /depreciation-runs/{id}", h.getDepreciationRun)
	mux.HandleFunc("POST /depreciation-runs/{id}/post", h.postDepreciationRun)
	mux.HandleFunc("DELETE /depreciation-runs/{id}", h.deleteDepreciationRun)

	mux.HandleFunc("GET /calibrations", h.listCalibrations)
	mux.HandleFunc("POST /calibrations", h.createCalibration)
	mux.HandleFunc("POST /calibrations/{id}/complete", h.completeCalibration)
	mux.HandleFunc("POST /calibrations/{id}/cancel", h.cancelCalibration)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "asset-service"})
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
		SourceService: "asset-service",
		OccurredAt:    time.Now(),
		ActorUserID:   actorUserID,
		CompanyID:     companyID,
		Action:        action,
		EntityType:    entityType,
		EntityID:      entityID,
		Payload:       payload,
	}
}

// headerValue meneruskan actor ke panggilan service lain, yang menerima string
// biasa alih-alih pointer.
func headerValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func isDuplicateKey(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate key")
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
