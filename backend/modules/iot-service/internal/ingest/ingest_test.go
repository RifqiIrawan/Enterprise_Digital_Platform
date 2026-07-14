package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCheckThreshold(t *testing.T) {
	cases := []struct {
		name             string
		value, min, max  float64
		wantOK           bool
		wantSeverity     string
		wantDirectionSet bool
	}{
		{"within range", 25, 20, 30, false, "", false},
		{"at min boundary (inclusive)", 20, 20, 30, false, "", false},
		{"at max boundary (inclusive)", 30, 20, 30, false, "", false},
		{"just below min, small breach", 19, 20, 30, true, "MEDIUM", true},
		{"far below min, large breach", 0, 20, 30, true, "HIGH", true},
		{"just above max, small breach", 31, 20, 30, true, "MEDIUM", true},
		{"far above max, large breach", 100, 20, 30, true, "HIGH", true},
		{"exactly at 50% breach fraction is not > 0.5, stays MEDIUM", 35, 20, 30, true, "MEDIUM", true},
		{"just over 50% breach fraction is HIGH", 35.1, 20, 30, true, "HIGH", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			direction, severity, ok := checkThreshold(c.value, c.min, c.max)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if !c.wantOK {
				return
			}
			if severity != c.wantSeverity {
				t.Errorf("severity = %q, want %q", severity, c.wantSeverity)
			}
			if c.wantDirectionSet && direction == "" {
				t.Error("expected a non-empty breach direction")
			}
		})
	}
}

func mustInsertDevice(t *testing.T, deviceType string, min, max *float64) (deviceID, companyID string) {
	t.Helper()
	companyID = uuid.NewString()
	code := "DEV-" + uuid.NewString()[:8]
	err := pool.QueryRow(context.Background(), `
		INSERT INTO devices (company_id, device_code, device_type, name, threshold_min, threshold_max)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		companyID, code, deviceType, "Test Device "+code, min, max,
	).Scan(&deviceID)
	if err != nil {
		t.Fatalf("insert device: %v", err)
	}
	return deviceID, companyID
}

func countAlerts(t *testing.T, deviceID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM alerts WHERE device_id = $1`, deviceID).Scan(&n); err != nil {
		t.Fatalf("count alerts: %v", err)
	}
	return n
}

func TestIngest_NumericBreach_CreatesAlert(t *testing.T) {
	min, max := 20.0, 30.0
	deviceID, _ := mustInsertDevice(t, "TEMPERATURE", &min, &max)

	value := 45.0 // far above max -> HIGH severity
	if err := Ingest(context.Background(), pool, nil, Payload{DeviceID: deviceID, ValueNumeric: &value, RecordedAt: time.Now()}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	if n := countAlerts(t, deviceID); n != 1 {
		t.Fatalf("expected exactly 1 alert, got %d", n)
	}

	var severity, status string
	err := pool.QueryRow(context.Background(), `SELECT severity, status FROM alerts WHERE device_id = $1`, deviceID).Scan(&severity, &status)
	if err != nil {
		t.Fatalf("query alert: %v", err)
	}
	if severity != "HIGH" || status != "OPEN" {
		t.Errorf("alert = {severity: %q, status: %q}, want {HIGH, OPEN}", severity, status)
	}
}

func TestIngest_NumericInRange_NoAlert(t *testing.T) {
	min, max := 20.0, 30.0
	deviceID, _ := mustInsertDevice(t, "TEMPERATURE", &min, &max)

	value := 25.0
	if err := Ingest(context.Background(), pool, nil, Payload{DeviceID: deviceID, ValueNumeric: &value, RecordedAt: time.Now()}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if n := countAlerts(t, deviceID); n != 0 {
		t.Fatalf("expected 0 alerts for an in-range reading, got %d", n)
	}
}

func TestIngest_NumericNoThresholdConfigured_NoAlert(t *testing.T) {
	deviceID, _ := mustInsertDevice(t, "VIBRATION", nil, nil)

	value := 999.0
	if err := Ingest(context.Background(), pool, nil, Payload{DeviceID: deviceID, ValueNumeric: &value, RecordedAt: time.Now()}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if n := countAlerts(t, deviceID); n != 0 {
		t.Fatalf("expected 0 alerts when device has no threshold configured, got %d", n)
	}
}

func TestIngest_NonNumericType_NeverAlerts(t *testing.T) {
	deviceID, _ := mustInsertDevice(t, "RFID", nil, nil)

	tag := "TAG-XYZ"
	if err := Ingest(context.Background(), pool, nil, Payload{DeviceID: deviceID, ValueText: &tag, RecordedAt: time.Now()}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if n := countAlerts(t, deviceID); n != 0 {
		t.Fatalf("expected 0 alerts for a non-numeric device type, got %d", n)
	}
}

func TestIngest_WrongValueKindForDeviceType_ReturnsError(t *testing.T) {
	numDeviceID, _ := mustInsertDevice(t, "TEMPERATURE", nil, nil)
	textDeviceID, _ := mustInsertDevice(t, "BARCODE", nil, nil)

	text := "not-a-number"
	if err := Ingest(context.Background(), pool, nil, Payload{DeviceID: numDeviceID, ValueText: &text, RecordedAt: time.Now()}); err == nil {
		t.Error("expected error passing value_text to a numeric device type")
	}

	num := 1.0
	if err := Ingest(context.Background(), pool, nil, Payload{DeviceID: textDeviceID, ValueNumeric: &num, RecordedAt: time.Now()}); err == nil {
		t.Error("expected error passing value_numeric to a non-numeric device type")
	}
}

func TestIngest_UnknownDevice_ReturnsError(t *testing.T) {
	value := 1.0
	err := Ingest(context.Background(), pool, nil, Payload{DeviceID: uuid.NewString(), ValueNumeric: &value, RecordedAt: time.Now()})
	if err == nil {
		t.Error("expected error for unknown device id")
	}
}
