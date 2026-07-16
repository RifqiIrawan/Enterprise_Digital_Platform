package etl

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func mustSeedStockMovement(t *testing.T, companyID uuid.UUID) (movementID uuid.UUID, productSKU string) {
	t.Helper()
	var warehouseID uuid.UUID
	whCode := "WH-" + uuid.NewString()[:8]
	err := sourcePool.QueryRow(context.Background(),
		`INSERT INTO warehouses (code, name) VALUES ($1, $2) RETURNING id`,
		whCode, "Test Warehouse "+whCode,
	).Scan(&warehouseID)
	if err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}

	productSKU = "SKU-" + uuid.NewString()[:8]
	var productID uuid.UUID
	err = sourcePool.QueryRow(context.Background(),
		`INSERT INTO products (sku, name) VALUES ($1, $2) RETURNING id`,
		productSKU, "Test Product "+productSKU,
	).Scan(&productID)
	if err != nil {
		t.Fatalf("seed product: %v", err)
	}

	err = sourcePool.QueryRow(context.Background(), `
		INSERT INTO stock_movements (company_id, warehouse_id, product_id, movement_type, quantity, reference_type)
		VALUES ($1, $2, $3, 'IN', 25, 'MANUAL')
		RETURNING id`,
		companyID, warehouseID, productID,
	).Scan(&movementID)
	if err != nil {
		t.Fatalf("seed stock movement: %v", err)
	}
	return movementID, productSKU
}

func TestSyncInventory_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	movementID, productSKU := mustSeedStockMovement(t, companyID)

	n, err := SyncInventory(context.Background(), sourcePool, chClient)
	if err != nil {
		t.Fatalf("SyncInventory: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var gotSKU, gotType string
	var gotQty decimal.Decimal
	row := chClient.QueryRow(context.Background(),
		"SELECT product_sku, movement_type, quantity FROM fact_inventory_movements FINAL WHERE movement_id = ?", movementID)
	if err := row.Scan(&gotSKU, &gotType, &gotQty); err != nil {
		t.Fatalf("query synced inventory row: %v", err)
	}
	if gotSKU != productSKU {
		t.Errorf("product_sku = %q, want %q", gotSKU, productSKU)
	}
	if gotType != "IN" {
		t.Errorf("movement_type = %q, want IN", gotType)
	}
	if !gotQty.Equal(decimal.NewFromInt(25)) {
		t.Errorf("quantity = %v, want 25", gotQty)
	}
}
