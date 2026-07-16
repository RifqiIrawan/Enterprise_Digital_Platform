package etl

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func mustSeedSalesOrderWithLine(t *testing.T, companyID uuid.UUID) (lineID uuid.UUID, customerCode string) {
	t.Helper()
	customerCode = "CUST-" + uuid.NewString()[:8]
	var customerID uuid.UUID
	err := sourcePool.QueryRow(context.Background(),
		`INSERT INTO customers (customer_code, name) VALUES ($1, $2) RETURNING id`,
		customerCode, "Test Customer "+customerCode,
	).Scan(&customerID)
	if err != nil {
		t.Fatalf("seed customer: %v", err)
	}

	var soID uuid.UUID
	err = sourcePool.QueryRow(context.Background(), `
		INSERT INTO sales_orders (company_id, so_number, customer_id, order_date, status)
		VALUES ($1, $2, $3, CURRENT_DATE, 'CONFIRMED')
		RETURNING id`,
		companyID, "SO-"+uuid.NewString()[:8], customerID,
	).Scan(&soID)
	if err != nil {
		t.Fatalf("seed sales order: %v", err)
	}

	err = sourcePool.QueryRow(context.Background(), `
		INSERT INTO sales_order_lines (sales_order_id, product_name, quantity, unit_price, amount)
		VALUES ($1, 'Test Product', 3, 50.00, 150.00)
		RETURNING id`,
		soID,
	).Scan(&lineID)
	if err != nil {
		t.Fatalf("seed sales order line: %v", err)
	}
	return lineID, customerCode
}

func TestSyncSales_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	lineID, customerCode := mustSeedSalesOrderWithLine(t, companyID)

	n, err := SyncSales(context.Background(), sourcePool, chClient)
	if err != nil {
		t.Fatalf("SyncSales: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var gotCustomerCode string
	var gotAmount decimal.Decimal
	row := chClient.QueryRow(context.Background(),
		"SELECT customer_code, amount FROM fact_sales_order_lines FINAL WHERE line_id = ?", lineID)
	if err := row.Scan(&gotCustomerCode, &gotAmount); err != nil {
		t.Fatalf("query synced sales row: %v", err)
	}
	if gotCustomerCode != customerCode {
		t.Errorf("customer_code = %q, want %q", gotCustomerCode, customerCode)
	}
	if !gotAmount.Equal(decimal.NewFromFloat(150.00)) {
		t.Errorf("amount = %v, want 150.00", gotAmount)
	}
}
