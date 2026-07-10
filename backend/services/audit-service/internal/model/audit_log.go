package model

import "time"

type AuditLog struct {
	ID            string    `json:"id" db:"id"`
	EventID       *string   `json:"event_id" db:"event_id"`
	EventType     string    `json:"event_type" db:"event_type"`
	SourceService string    `json:"source_service" db:"source_service"`
	ActorUserID   *string   `json:"actor_user_id" db:"actor_user_id"`
	ActorEmail    *string   `json:"actor_email" db:"actor_email"`
	CompanyID     *string   `json:"company_id" db:"company_id"`
	BranchID      *string   `json:"branch_id" db:"branch_id"`
	Action        string    `json:"action" db:"action"`
	EntityType    string    `json:"entity_type" db:"entity_type"`
	EntityID      string    `json:"entity_id" db:"entity_id"`
	Payload       []byte    `json:"payload" db:"payload"`
	OccurredAt    time.Time `json:"occurred_at" db:"occurred_at"`
	RecordedAt    time.Time `json:"recorded_at" db:"recorded_at"`
}

// AuditEvent adalah amplop event yang dipublikasikan oleh service lain
// (auth-service, company-service, rbac-service) ke Kafka dan dikonsumsi
// di sini. Bentuknya sama persis dengan struct AuditEvent yang didefinisikan
// mandiri di masing-masing service publisher (tidak ada shared package
// lintas service, konsisten dengan prinsip microservices di dokumentasi).
type AuditEvent struct {
	EventID       string    `json:"event_id"`
	EventType     string    `json:"event_type"`
	SourceService string    `json:"source_service"`
	OccurredAt    time.Time `json:"occurred_at"`
	ActorUserID   *string   `json:"actor_user_id,omitempty"`
	ActorEmail    *string   `json:"actor_email,omitempty"`
	CompanyID     *string   `json:"company_id,omitempty"`
	BranchID      *string   `json:"branch_id,omitempty"`
	Action        string    `json:"action"`
	EntityType    string    `json:"entity_type"`
	EntityID      string    `json:"entity_id"`
	Payload       any       `json:"payload,omitempty"`
}
