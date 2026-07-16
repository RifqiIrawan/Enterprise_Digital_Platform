package etl

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func mustSeedMaintenanceSchedule(t *testing.T, companyID uuid.UUID) (scheduleID uuid.UUID, assetCode string) {
	t.Helper()
	assetCode = "AST-" + uuid.NewString()[:8]
	var assetID uuid.UUID
	err := sourcePool.QueryRow(context.Background(),
		`INSERT INTO assets (asset_code, name) VALUES ($1, $2) RETURNING id`,
		assetCode, "Test Asset "+assetCode,
	).Scan(&assetID)
	if err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	err = sourcePool.QueryRow(context.Background(), `
		INSERT INTO maintenance_schedules (company_id, asset_id, maintenance_type, scheduled_date, status)
		VALUES ($1, $2, 'Rutin', CURRENT_DATE, 'SCHEDULED')
		RETURNING id`,
		companyID, assetID,
	).Scan(&scheduleID)
	if err != nil {
		t.Fatalf("seed maintenance schedule: %v", err)
	}
	return scheduleID, assetCode
}

func TestSyncAsset_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	scheduleID, assetCode := mustSeedMaintenanceSchedule(t, companyID)

	n, err := SyncAsset(context.Background(), sourcePool, chClient)
	if err != nil {
		t.Fatalf("SyncAsset: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var gotAssetCode, gotStatus string
	row := chClient.QueryRow(context.Background(),
		"SELECT asset_code, status FROM fact_asset_maintenance FINAL WHERE schedule_id = ?", scheduleID)
	if err := row.Scan(&gotAssetCode, &gotStatus); err != nil {
		t.Fatalf("query synced asset row: %v", err)
	}
	if gotAssetCode != assetCode {
		t.Errorf("asset_code = %q, want %q", gotAssetCode, assetCode)
	}
	if gotStatus != "SCHEDULED" {
		t.Errorf("status = %q, want SCHEDULED", gotStatus)
	}
}
