package httpapi

import (
	"net/http"
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
	CompanyID     *string   `json:"company_id,omitempty"`
	Action        string    `json:"action"`
	EntityType    string    `json:"entity_type"`
	EntityID      string    `json:"entity_id"`
	Payload       any       `json:"payload,omitempty"`
}

func newAuditEvent(r *http.Request, eventType string, companyID *string, action, entityType, entityID string, payload any) auditEvent {
	actorUserID, actorEmail := actorFromHeader(r)
	return auditEvent{
		EventID:       uuid.NewString(),
		EventType:     eventType,
		SourceService: "company-service",
		OccurredAt:    time.Now(),
		ActorUserID:   actorUserID,
		ActorEmail:    actorEmail,
		CompanyID:     companyID,
		Action:        action,
		EntityType:    entityType,
		EntityID:      entityID,
		Payload:       payload,
	}
}

// actorFromHeader membaca identitas pemanggil dari header yang dipasang
// api-gateway setelah memverifikasi JWT (lihat X-User-Id/X-User-Email di
// internal/gateway). Pola yang sama dipakai finance-service dkk.
//
// Keduanya nil-able dengan sengaja: service ini juga bisa dipanggil langsung
// (tanpa lewat gateway) oleh tooling internal, dan event yang tidak punya
// aktor lebih jujur daripada event yang mengarang satu. Sebelumnya amplop di
// atas TIDAK punya kolom aktor sama sekali, sehingga setiap perubahan
// company/branch/department tercatat di audit log tanpa keterangan siapa yang
// melakukannya -- justru pada permukaan admin yang paling butuh atribusi itu.
func actorFromHeader(r *http.Request) (userID, email *string) {
	if r == nil {
		return nil, nil
	}
	if v := r.Header.Get("X-User-Id"); v != "" {
		userID = &v
	}
	if v := r.Header.Get("X-User-Email"); v != "" {
		email = &v
	}
	return userID, email
}
