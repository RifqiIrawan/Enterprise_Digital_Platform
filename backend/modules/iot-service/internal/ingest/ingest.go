// Package ingest berisi logika murni "satu reading masuk -> simpan -> cek
// threshold -> buat alert kalau perlu", dipisah dari mqttclient/simulator
// supaya bisa diuji langsung dengan payload buatan tanpa perlu broker MQTT
// sungguhan (sama prinsipnya dengan financeclient/warehouseclient yang
// di-stub di test modul lain -- di sini yang di-stub adalah transport-nya,
// bukan logic-nya).
package ingest

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/iot-service/internal/eventbus"
)

// Payload adalah satu reading yang masuk dari MQTT. ReadingType tidak
// disertakan di sini -- diambil dari devices.device_type (sumber
// kebenaran), bukan dipercaya dari payload, karena satu-satunya publisher
// saat ini (internal/simulator) sudah tahu device_type-nya dari database
// yang sama.
type Payload struct {
	DeviceID     string
	ValueNumeric *float64
	ValueText    *string
	RecordedAt   time.Time
}

var numericTypes = map[string]bool{
	"TEMPERATURE": true,
	"HUMIDITY":    true,
	"VIBRATION":   true,
}

type device struct {
	id           string
	companyID    string
	branchID     *string
	deviceCode   string
	name         string
	deviceType   string
	status       string
	thresholdMin *float64
	thresholdMax *float64
}

// event adalah amplop event yang dipublikasikan ke Kafka dan dikonsumsi
// oleh audit-service, sama shape-nya dengan auditEvent di internal/httpapi
// tapi didefinisikan terpisah di sini supaya package ini tidak perlu
// bergantung pada package httpapi (lihat komentar paket).
type event struct {
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

func newEvent(eventType, companyID, action, entityType, entityID string, payload any) event {
	return event{
		EventID:       uuid.NewString(),
		EventType:     eventType,
		SourceService: "iot-service",
		OccurredAt:    time.Now(),
		CompanyID:     &companyID,
		Action:        action,
		EntityType:    entityType,
		EntityID:      entityID,
		Payload:       payload,
	}
}

// Ingest menyimpan satu reading dan, kalau device-nya numerik dan
// thresholdnya dilanggar, membuat + mempublikasikan sebuah alert. events
// boleh nil (lihat eventbus.Publisher, nil-safe by design) -- dipakai di
// test tanpa Kafka jalan.
func Ingest(ctx context.Context, pool *pgxpool.Pool, events *eventbus.Publisher, p Payload) error {
	dev, err := getDevice(ctx, pool, p.DeviceID)
	if err != nil {
		return err
	}

	if numericTypes[dev.deviceType] {
		if p.ValueNumeric == nil || p.ValueText != nil {
			return fmt.Errorf("device_type %s butuh value_numeric (bukan value_text)", dev.deviceType)
		}
	} else {
		if p.ValueText == nil || p.ValueNumeric != nil {
			return fmt.Errorf("device_type %s butuh value_text (bukan value_numeric)", dev.deviceType)
		}
	}

	var readingID string
	err = pool.QueryRow(ctx, `
		INSERT INTO readings (device_id, company_id, branch_id, reading_type, value_numeric, value_text, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		dev.id, dev.companyID, dev.branchID, dev.deviceType, p.ValueNumeric, p.ValueText, p.RecordedAt,
	).Scan(&readingID)
	if err != nil {
		return fmt.Errorf("insert reading: %w", err)
	}

	if !numericTypes[dev.deviceType] || dev.thresholdMin == nil || dev.thresholdMax == nil || p.ValueNumeric == nil {
		return nil
	}

	breach, severity, ok := checkThreshold(*p.ValueNumeric, *dev.thresholdMin, *dev.thresholdMax)
	if !ok {
		return nil
	}

	message := fmt.Sprintf("%s: nilai %.2f di luar ambang batas [%.2f, %.2f] (%s)",
		dev.name, *p.ValueNumeric, *dev.thresholdMin, *dev.thresholdMax, breach)

	var alertID string
	err = pool.QueryRow(ctx, `
		INSERT INTO alerts (device_id, reading_id, company_id, branch_id, alert_type, severity, message)
		VALUES ($1, $2, $3, $4, 'THRESHOLD_BREACH', $5, $6)
		RETURNING id`,
		dev.id, readingID, dev.companyID, dev.branchID, severity, message,
	).Scan(&alertID)
	if err != nil {
		return fmt.Errorf("insert alert: %w", err)
	}

	events.Publish("iot.alert.triggered", newEvent("iot.alert.triggered", dev.companyID, "create", "alert", alertID, map[string]any{
		"alert_id":    alertID,
		"device_id":   dev.id,
		"device_code": dev.deviceCode,
		"severity":    severity,
		"message":     message,
	}))
	return nil
}

func getDevice(ctx context.Context, pool *pgxpool.Pool, id string) (device, error) {
	var d device
	err := pool.QueryRow(ctx, `
		SELECT id, company_id, branch_id, device_code, name, device_type, status, threshold_min, threshold_max
		FROM devices WHERE id = $1`, id,
	).Scan(&d.id, &d.companyID, &d.branchID, &d.deviceCode, &d.name, &d.deviceType, &d.status, &d.thresholdMin, &d.thresholdMax)
	if err == pgx.ErrNoRows {
		return device{}, fmt.Errorf("device %s tidak ditemukan", id)
	}
	if err != nil {
		return device{}, fmt.Errorf("load device %s: %w", id, err)
	}
	return d, nil
}

// checkThreshold mengembalikan ok=false kalau value ada di dalam [min, max]
// (tidak perlu alert). Severity HIGH kalau jarak pelanggaran melebihi 50%
// dari lebar rentang threshold, selain itu MEDIUM -- heuristik sederhana
// yang didokumentasikan apa adanya (bukan model ML), gaya yang sama dipakai
// z-score anomaly detection di ai-bi-service.
func checkThreshold(value, min, max float64) (breachDirection, severity string, ok bool) {
	var distance float64
	switch {
	case value < min:
		distance = min - value
		breachDirection = "di bawah minimum"
	case value > max:
		distance = value - max
		breachDirection = "di atas maksimum"
	default:
		return "", "", false
	}

	severity = "MEDIUM"
	if rangeWidth := max - min; rangeWidth > 0 && distance/rangeWidth > 0.5 {
		severity = "HIGH"
	}
	return breachDirection, severity, true
}
