package httpapi

import (
	"time"

	"github.com/google/uuid"
)

// auditEvent adalah amplop event yang dipublikasikan ke Kafka dan dikonsumsi
// oleh audit-service (lihat backend/services/audit-service/internal/model.AuditEvent).
type auditEvent struct {
	EventID       string    `json:"event_id"`
	EventType     string    `json:"event_type"`
	SourceService string    `json:"source_service"`
	OccurredAt    time.Time `json:"occurred_at"`
	ActorUserID   *string   `json:"actor_user_id,omitempty"`
	ActorEmail    *string   `json:"actor_email,omitempty"`
	Action        string    `json:"action"`
	EntityType    string    `json:"entity_type"`
	EntityID      string    `json:"entity_id"`
	Payload       any       `json:"payload,omitempty"`
}

func newAuditEvent(eventType, sourceService string, actorUserID, actorEmail *string, action, entityType, entityID string, payload any) auditEvent {
	return auditEvent{
		EventID:       uuid.NewString(),
		EventType:     eventType,
		SourceService: sourceService,
		OccurredAt:    time.Now(),
		ActorUserID:   actorUserID,
		ActorEmail:    actorEmail,
		Action:        action,
		EntityType:    entityType,
		EntityID:      entityID,
		Payload:       payload,
	}
}
