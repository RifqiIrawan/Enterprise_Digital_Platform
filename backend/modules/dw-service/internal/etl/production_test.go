package etl

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func mustSeedWorkOrder(t *testing.T, companyID uuid.UUID, quantityProduced *float64) (woID uuid.UUID, woNumber string) {
	t.Helper()
	var bomID uuid.UUID
	err := sourcePool.QueryRow(context.Background(),
		`INSERT INTO bill_of_materials (bom_code, product_id) VALUES ($1, $2) RETURNING id`,
		"BOM-"+uuid.NewString()[:8], uuid.New(),
	).Scan(&bomID)
	if err != nil {
		t.Fatalf("seed bom: %v", err)
	}

	woNumber = "WO-" + uuid.NewString()[:8]
	err = sourcePool.QueryRow(context.Background(), `
		INSERT INTO work_orders (company_id, wo_number, bom_id, product_id, warehouse_id, quantity_planned, quantity_produced, status, planned_start_date)
		VALUES ($1, $2, $3, $4, $5, 100, $6, 'IN_PROGRESS', CURRENT_DATE)
		RETURNING id`,
		companyID, woNumber, bomID, uuid.New(), uuid.New(), quantityProduced,
	).Scan(&woID)
	if err != nil {
		t.Fatalf("seed work order: %v", err)
	}
	return woID, woNumber
}

func TestSyncProduction_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	woID, woNumber := mustSeedWorkOrder(t, companyID, nil)

	n, err := SyncProduction(context.Background(), sourcePool, chClient, nil)
	if err != nil {
		t.Fatalf("SyncProduction: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var gotWONumber string
	var gotPlanned decimal.Decimal
	var gotProduced *decimal.Decimal
	row := chClient.QueryRow(context.Background(),
		"SELECT wo_number, quantity_planned, quantity_produced FROM fact_production_work_orders FINAL WHERE wo_id = ?", woID)
	if err := row.Scan(&gotWONumber, &gotPlanned, &gotProduced); err != nil {
		t.Fatalf("query synced production row: %v", err)
	}
	if gotWONumber != woNumber {
		t.Errorf("wo_number = %q, want %q", gotWONumber, woNumber)
	}
	if !gotPlanned.Equal(decimal.NewFromInt(100)) {
		t.Errorf("quantity_planned = %v, want 100", gotPlanned)
	}
	if gotProduced != nil {
		t.Errorf("quantity_produced = %v, want nil (not yet produced)", gotProduced)
	}
}
