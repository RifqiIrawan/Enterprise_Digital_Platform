package etl

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func mustSeedOrderItem(t *testing.T, companyID uuid.UUID) (lineID uuid.UUID, productSKU string) {
	t.Helper()
	var orderID uuid.UUID
	err := sourcePool.QueryRow(context.Background(), `
		INSERT INTO orders (company_id, order_number, customer_name, status)
		VALUES ($1, $2, 'Test Customer', 'PAID')
		RETURNING id`,
		companyID, "ORD-"+uuid.NewString()[:8],
	).Scan(&orderID)
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}

	productSKU = "SKU-" + uuid.NewString()[:8]
	err = sourcePool.QueryRow(context.Background(), `
		INSERT INTO order_items (order_id, product_id, product_sku, product_name, unit_price, quantity, line_total)
		VALUES ($1, $2, $3, 'Test Product', 25000, 4, 100000)
		RETURNING id`,
		orderID, uuid.New(), productSKU,
	).Scan(&lineID)
	if err != nil {
		t.Fatalf("seed order item: %v", err)
	}
	return lineID, productSKU
}

func TestSyncEcommerce_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	lineID, productSKU := mustSeedOrderItem(t, companyID)

	n, err := SyncEcommerce(context.Background(), sourcePool, chClient, nil)
	if err != nil {
		t.Fatalf("SyncEcommerce: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var gotProductSKU, gotOrderStatus string
	var gotAmount decimal.Decimal
	row := chClient.QueryRow(context.Background(),
		"SELECT product_sku, order_status, amount FROM fact_ecommerce_order_lines FINAL WHERE line_id = ?", lineID)
	if err := row.Scan(&gotProductSKU, &gotOrderStatus, &gotAmount); err != nil {
		t.Fatalf("query synced ecommerce row: %v", err)
	}
	if gotProductSKU != productSKU {
		t.Errorf("product_sku = %q, want %q", gotProductSKU, productSKU)
	}
	if gotOrderStatus != "PAID" {
		t.Errorf("order_status = %q, want PAID", gotOrderStatus)
	}
	// unit_price 25000 * quantity 4 = 100000
	if !gotAmount.Equal(decimal.NewFromInt(100000)) {
		t.Errorf("amount = %v, want 100000", gotAmount)
	}
}
