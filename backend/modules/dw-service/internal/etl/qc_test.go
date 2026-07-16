package etl

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func mustSeedQualityInspection(t *testing.T, companyID uuid.UUID) (inspectionID uuid.UUID, standardCode string) {
	t.Helper()
	standardCode = "QS-" + uuid.NewString()[:8]
	var standardID uuid.UUID
	err := sourcePool.QueryRow(context.Background(),
		`INSERT INTO quality_standards (standard_code, product_id) VALUES ($1, $2) RETURNING id`,
		standardCode, uuid.New(),
	).Scan(&standardID)
	if err != nil {
		t.Fatalf("seed quality standard: %v", err)
	}

	err = sourcePool.QueryRow(context.Background(), `
		INSERT INTO quality_inspections (company_id, inspection_number, standard_id, product_id, inspected_quantity, passed_quantity, failed_quantity, result, inspection_date)
		VALUES ($1, $2, $3, $4, 10, 8, 2, 'PARTIAL', CURRENT_DATE)
		RETURNING id`,
		companyID, "INS-"+uuid.NewString()[:8], standardID, uuid.New(),
	).Scan(&inspectionID)
	if err != nil {
		t.Fatalf("seed quality inspection: %v", err)
	}
	return inspectionID, standardCode
}

func TestSyncQC_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	inspectionID, standardCode := mustSeedQualityInspection(t, companyID)

	n, err := SyncQC(context.Background(), sourcePool, chClient)
	if err != nil {
		t.Fatalf("SyncQC: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var gotStandardCode, gotResult string
	var gotFailed decimal.Decimal
	row := chClient.QueryRow(context.Background(),
		"SELECT standard_code, result, failed_quantity FROM fact_qc_inspections FINAL WHERE inspection_id = ?", inspectionID)
	if err := row.Scan(&gotStandardCode, &gotResult, &gotFailed); err != nil {
		t.Fatalf("query synced qc row: %v", err)
	}
	if gotStandardCode != standardCode {
		t.Errorf("standard_code = %q, want %q", gotStandardCode, standardCode)
	}
	if gotResult != "PARTIAL" {
		t.Errorf("result = %q, want PARTIAL", gotResult)
	}
	if !gotFailed.Equal(decimal.NewFromInt(2)) {
		t.Errorf("failed_quantity = %v, want 2", gotFailed)
	}
}
