package etl

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func mustSeedPurchaseOrderWithLine(t *testing.T, companyID uuid.UUID) (lineID uuid.UUID, supplierCode string) {
	t.Helper()
	supplierCode = "SUP-" + uuid.NewString()[:8]
	var supplierID uuid.UUID
	err := sourcePool.QueryRow(context.Background(),
		`INSERT INTO suppliers (supplier_code, name) VALUES ($1, $2) RETURNING id`,
		supplierCode, "Test Supplier "+supplierCode,
	).Scan(&supplierID)
	if err != nil {
		t.Fatalf("seed supplier: %v", err)
	}

	var poID uuid.UUID
	err = sourcePool.QueryRow(context.Background(), `
		INSERT INTO purchase_orders (company_id, po_number, supplier_id, order_date, status)
		VALUES ($1, $2, $3, CURRENT_DATE, 'CONFIRMED')
		RETURNING id`,
		companyID, "PO-"+uuid.NewString()[:8], supplierID,
	).Scan(&poID)
	if err != nil {
		t.Fatalf("seed purchase order: %v", err)
	}

	err = sourcePool.QueryRow(context.Background(), `
		INSERT INTO purchase_order_lines (purchase_order_id, product_name, quantity, unit_price, amount)
		VALUES ($1, 'Test Material', 10, 20.00, 200.00)
		RETURNING id`,
		poID,
	).Scan(&lineID)
	if err != nil {
		t.Fatalf("seed purchase order line: %v", err)
	}
	return lineID, supplierCode
}

func TestSyncPurchasing_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	lineID, supplierCode := mustSeedPurchaseOrderWithLine(t, companyID)

	n, err := SyncPurchasing(context.Background(), sourcePool, chClient, nil)
	if err != nil {
		t.Fatalf("SyncPurchasing: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var gotSupplierCode string
	var gotAmount decimal.Decimal
	row := chClient.QueryRow(context.Background(),
		"SELECT supplier_code, amount FROM fact_purchasing_order_lines FINAL WHERE line_id = ?", lineID)
	if err := row.Scan(&gotSupplierCode, &gotAmount); err != nil {
		t.Fatalf("query synced purchasing row: %v", err)
	}
	if gotSupplierCode != supplierCode {
		t.Errorf("supplier_code = %q, want %q", gotSupplierCode, supplierCode)
	}
	if !gotAmount.Equal(decimal.NewFromFloat(200.00)) {
		t.Errorf("amount = %v, want 200.00", gotAmount)
	}
}
