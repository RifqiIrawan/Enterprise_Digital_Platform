package etl

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func mustSeedReading(t *testing.T, companyID uuid.UUID, valueNumeric float64) (readingID uuid.UUID, deviceCode string) {
	t.Helper()
	deviceCode = "DEV-" + uuid.NewString()[:8]
	var deviceID uuid.UUID
	err := sourcePool.QueryRow(context.Background(),
		`INSERT INTO devices (device_code, device_type) VALUES ($1, $2) RETURNING id`,
		deviceCode, "TEMPERATURE",
	).Scan(&deviceID)
	if err != nil {
		t.Fatalf("seed device: %v", err)
	}

	err = sourcePool.QueryRow(context.Background(), `
		INSERT INTO readings (device_id, company_id, reading_type, value_numeric, recorded_at)
		VALUES ($1, $2, 'TEMPERATURE', $3, $4)
		RETURNING id`,
		deviceID, companyID, valueNumeric, time.Now(),
	).Scan(&readingID)
	if err != nil {
		t.Fatalf("seed reading: %v", err)
	}
	return readingID, deviceCode
}

func TestSyncIoT_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	readingID, deviceCode := mustSeedReading(t, companyID, 36.5)

	n, err := SyncIoT(context.Background(), sourcePool, chClient)
	if err != nil {
		t.Fatalf("SyncIoT: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var gotDeviceCode, gotReadingType string
	row := chClient.QueryRow(context.Background(),
		"SELECT device_code, reading_type FROM fact_iot_readings FINAL WHERE reading_id = ?", readingID)
	if err := row.Scan(&gotDeviceCode, &gotReadingType); err != nil {
		t.Fatalf("query synced iot row: %v", err)
	}
	if gotDeviceCode != deviceCode {
		t.Errorf("device_code = %q, want %q", gotDeviceCode, deviceCode)
	}
	if gotReadingType != "TEMPERATURE" {
		t.Errorf("reading_type = %q, want TEMPERATURE", gotReadingType)
	}
}
