package httpapi

import (
	"context"
	"encoding/json"
	"log"

	"github.com/enterprise-digital-platform/audit-service/internal/model"
)

// Ingest menyimpan satu AuditEvent (hasil konsumsi Kafka) ke tabel audit_logs.
func (h *Handler) Ingest(ctx context.Context, topic string, raw []byte) {
	var evt model.AuditEvent
	if err := json.Unmarshal(raw, &evt); err != nil {
		log.Printf("audit-service: failed to decode event from topic %s: %v", topic, err)
		return
	}

	var payload []byte
	if evt.Payload != nil {
		payload, _ = json.Marshal(evt.Payload)
	}

	_, err := h.pool.Exec(ctx, `
		INSERT INTO audit_logs (event_id, event_type, source_service, actor_user_id, actor_email,
		                         company_id, branch_id, action, entity_type, entity_id, payload, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		nullIfEmpty(evt.EventID), evt.EventType, evt.SourceService, evt.ActorUserID, evt.ActorEmail,
		evt.CompanyID, evt.BranchID, evt.Action, evt.EntityType, evt.EntityID, payload, evt.OccurredAt,
	)
	if err != nil {
		log.Printf("audit-service: failed to store event from topic %s: %v", topic, err)
	}
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
