package streaming

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Setiap test:
//   1. Seed satu atau beberapa baris di Postgres (dw_streaming_test)
//   2. Buat event JSON dengan entity_id yang tepat
//   3. Panggil handler langsung (tidak via Kafka)
//   4. Verifikasi baris muncul di ClickHouse
//
// Isolasi antar test: company_id acak (UUID) per test — baris satu test
// tidak terlihat oleh test lain, tanpa perlu TRUNCATE. Pola identik dengan
// internal/etl tests.

// ---------------------------------------------------------------------------
// Finance
// ---------------------------------------------------------------------------

func TestHandleFinanceJournalPosted(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()

	// Seed account
	var accountID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO accounts (account_code, account_name, account_type)
		VALUES ('1001', 'Cash', 'ASSET') RETURNING id`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}

	// Seed journal entry (POSTED)
	var jeID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO journal_entries (company_id, entry_number, entry_date, period, status, posted_at)
		VALUES ($1, 'JE-STREAM-001', $2, '2026-07', 'POSTED', now()) RETURNING id`,
		companyID, today()).Scan(&jeID); err != nil {
		t.Fatal(err)
	}

	// Seed 2 journal lines (debit + credit)
	mustExec(t, `INSERT INTO journal_lines (journal_id, account_id, debit_amount) VALUES ($1, $2, 1000)`, jeID, accountID)
	mustExec(t, `INSERT INTO journal_lines (journal_id, account_id, credit_amount) VALUES ($1, $2, 1000)`, jeID, accountID)

	// Panggil handler
	if err := handleFinanceJournalPosted(ctx, makeEvent(jeID), pools, chClient, nil); err != nil {
		t.Fatalf("handleFinanceJournalPosted: %v", err)
	}

	// Verifikasi 2 baris di ClickHouse
	count, err := chClient.CountRows(ctx, "fact_finance_journal_lines FINAL")
	if err != nil {
		t.Fatal(err)
	}
	// Count bisa lebih dari 2 karena test lain bisa nulis ke tabel yang sama
	// — cukup verifikasi lewat query spesifik ke company_id ini.
	row := chClient.QueryRow(ctx,
		"SELECT count(*) FROM fact_finance_journal_lines FINAL WHERE company_id = ?", companyID)
	var n uint64
	if err := row.Scan(&n); err != nil {
		t.Fatalf("count finance rows: %v", err)
	}
	if n != 2 {
		t.Errorf("want 2 finance rows for company %s, got %d (total in table: %d)", companyID, n, count)
	}
}

// ---------------------------------------------------------------------------
// Sales
// ---------------------------------------------------------------------------

func TestHandleSalesOrderEvent(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()

	var custID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO customers (customer_code, name) VALUES ('CUST-STREAM', 'PT Stream') RETURNING id`,
	).Scan(&custID); err != nil {
		t.Fatal(err)
	}

	var soID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO sales_orders (company_id, so_number, order_date, status, customer_id, updated_at)
		VALUES ($1, 'SO-STREAM-001', $2, 'FULFILLED', $3, now()) RETURNING id`,
		companyID, today(), custID).Scan(&soID); err != nil {
		t.Fatal(err)
	}

	mustExec(t, `INSERT INTO sales_order_lines (sales_order_id, product_name, quantity, unit_price, amount)
		VALUES ($1, 'Widget A', 10, 50000, 500000)`, soID)

	if err := handleSalesOrderEvent(ctx, makeEvent(soID), pools, chClient, nil); err != nil {
		t.Fatalf("handleSalesOrderEvent: %v", err)
	}

	row := chClient.QueryRow(ctx,
		"SELECT count(*) FROM fact_sales_order_lines FINAL WHERE company_id = ?", companyID)
	var n uint64
	if err := row.Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("want 1 sales line, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// Inventory — single move
// ---------------------------------------------------------------------------

func TestHandleStockMoved(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()

	var whID, prodID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO warehouses (code, name) VALUES ('WH-S', 'Gudang Stream') RETURNING id`).Scan(&whID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO products (sku, name) VALUES ('SKU-S', 'Produk Stream') RETURNING id`).Scan(&prodID); err != nil {
		t.Fatal(err)
	}

	var mvID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO stock_movements (company_id, warehouse_id, product_id, movement_type, quantity, reference_type, movement_date)
		VALUES ($1, $2, $3, 'IN', 50, 'MANUAL', $4) RETURNING id`,
		companyID, whID, prodID, today()).Scan(&mvID); err != nil {
		t.Fatal(err)
	}

	if err := handleStockMoved(ctx, makeEvent(mvID), pools, chClient, nil); err != nil {
		t.Fatalf("handleStockMoved: %v", err)
	}

	row := chClient.QueryRow(ctx,
		"SELECT count(*) FROM fact_inventory_movements FINAL WHERE company_id = ?", companyID)
	var n uint64
	if err := row.Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("want 1 inventory movement, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// Inventory — batch move (2 movements by reference_id)
// ---------------------------------------------------------------------------

func TestHandleStockBatchMoved(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	refID := uuid.New() // PO/SO/WO id sebagai reference

	var whID, prodID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO warehouses (code, name) VALUES ('WH-B', 'Gudang Batch') RETURNING id`).Scan(&whID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO products (sku, name) VALUES ('SKU-B', 'Produk Batch') RETURNING id`).Scan(&prodID); err != nil {
		t.Fatal(err)
	}

	// 2 movements berbeda dengan reference_id yang sama
	mustExec(t, `INSERT INTO stock_movements (company_id, warehouse_id, product_id, movement_type, quantity, reference_type, reference_id, movement_date)
		VALUES ($1, $2, $3, 'IN', 30, 'PURCHASE_ORDER', $4, $5)`, companyID, whID, prodID, refID, today())
	mustExec(t, `INSERT INTO stock_movements (company_id, warehouse_id, product_id, movement_type, quantity, reference_type, reference_id, movement_date)
		VALUES ($1, $2, $3, 'IN', 20, 'PURCHASE_ORDER', $4, $5)`, companyID, whID, prodID, refID, today())

	// entity_id untuk batch_moved adalah reference_id
	if err := handleStockBatchMoved(ctx, makeEvent(refID), pools, chClient, nil); err != nil {
		t.Fatalf("handleStockBatchMoved: %v", err)
	}

	row := chClient.QueryRow(ctx,
		"SELECT count(*) FROM fact_inventory_movements FINAL WHERE company_id = ?", companyID)
	var n uint64
	if err := row.Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("want 2 inventory movements (batch), got %d", n)
	}
}

// ---------------------------------------------------------------------------
// HR — payroll posted
// ---------------------------------------------------------------------------

func TestHandleHRPayrollPosted(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()

	var empID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO employees (employee_code) VALUES ('EMP-STREAM') RETURNING id`).Scan(&empID); err != nil {
		t.Fatal(err)
	}

	var runID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO payroll_runs (company_id, period, status, posted_at) VALUES ($1, '2026-07', 'POSTED', now()) RETURNING id`,
		companyID).Scan(&runID); err != nil {
		t.Fatal(err)
	}

	mustExec(t, `INSERT INTO payroll_details (payroll_run_id, employee_id, employee_name, basic_salary, gross_salary, total_deduction, net_salary, working_days, present_days)
		VALUES ($1, $2, 'Budi Streaming', 5000000, 5500000, 500000, 5000000, 22, 20)`, runID, empID)

	if err := handleHRPayrollPosted(ctx, makeEvent(runID), pools, chClient, nil); err != nil {
		t.Fatalf("handleHRPayrollPosted: %v", err)
	}

	row := chClient.QueryRow(ctx,
		"SELECT count(*) FROM fact_hr_payroll_details FINAL WHERE company_id = ?", companyID)
	var n uint64
	if err := row.Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("want 1 payroll detail, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// Purchasing
// ---------------------------------------------------------------------------

func TestHandlePurchasingOrderEvent(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()

	var supID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO suppliers (supplier_code, name) VALUES ('SUP-STREAM', 'PT Suplai Stream') RETURNING id`).Scan(&supID); err != nil {
		t.Fatal(err)
	}

	var poID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO purchase_orders (company_id, po_number, order_date, status, supplier_id)
		VALUES ($1, 'PO-STREAM-001', $2, 'RECEIVED', $3) RETURNING id`,
		companyID, today(), supID).Scan(&poID); err != nil {
		t.Fatal(err)
	}

	mustExec(t, `INSERT INTO purchase_order_lines (purchase_order_id, product_name, quantity, unit_price, amount)
		VALUES ($1, 'Bahan Baku X', 100, 10000, 1000000)`, poID)

	if err := handlePurchasingOrderEvent(ctx, makeEvent(poID), pools, chClient, nil); err != nil {
		t.Fatalf("handlePurchasingOrderEvent: %v", err)
	}

	row := chClient.QueryRow(ctx,
		"SELECT count(*) FROM fact_purchasing_order_lines FINAL WHERE company_id = ?", companyID)
	var n uint64
	if err := row.Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("want 1 purchasing line, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// Production
// ---------------------------------------------------------------------------

func TestHandleProductionWOCompleted(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	bomID := uuid.New()
	prodID := uuid.New()
	whID := uuid.New()

	var woID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO work_orders (company_id, wo_number, bom_id, product_id, warehouse_id,
		                         quantity_planned, quantity_produced, status, planned_start_date)
		VALUES ($1, 'WO-STREAM-001', $2, $3, $4, 10, 10, 'COMPLETED', $5) RETURNING id`,
		companyID, bomID, prodID, whID, today()).Scan(&woID); err != nil {
		t.Fatal(err)
	}

	if err := handleProductionWOCompleted(ctx, makeEvent(woID), pools, chClient, nil); err != nil {
		t.Fatalf("handleProductionWOCompleted: %v", err)
	}

	row := chClient.QueryRow(ctx,
		"SELECT count(*) FROM fact_production_work_orders FINAL WHERE company_id = ?", companyID)
	var n uint64
	if err := row.Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("want 1 production WO, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// QC
// ---------------------------------------------------------------------------

func TestHandleQCInspectionCreated(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	prodID := uuid.New()

	var stdID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO quality_standards (standard_code, product_id) VALUES ('QS-STREAM', $1) RETURNING id`, prodID).Scan(&stdID); err != nil {
		t.Fatal(err)
	}

	var inspID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO quality_inspections (company_id, inspection_number, standard_id, product_id,
		                                 reference_type, inspected_quantity, passed_quantity, failed_quantity, result, inspection_date)
		VALUES ($1, 'INS-STREAM-001', $2, $3, 'MANUAL', 10, 9, 1, 'PARTIAL', $4) RETURNING id`,
		companyID, stdID, prodID, today()).Scan(&inspID); err != nil {
		t.Fatal(err)
	}

	if err := handleQCInspectionCreated(ctx, makeEvent(inspID), pools, chClient, nil); err != nil {
		t.Fatalf("handleQCInspectionCreated: %v", err)
	}

	row := chClient.QueryRow(ctx,
		"SELECT count(*) FROM fact_qc_inspections FINAL WHERE company_id = ?", companyID)
	var n uint64
	if err := row.Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("want 1 QC inspection, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// Asset
// ---------------------------------------------------------------------------

func TestHandleAssetMaintenanceEvent(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()

	var assetID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO assets (asset_code, name) VALUES ('AST-STREAM', 'Forklift Stream') RETURNING id`).Scan(&assetID); err != nil {
		t.Fatal(err)
	}

	var schedID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO maintenance_schedules (company_id, asset_id, maintenance_type, scheduled_date, completed_date, status)
		VALUES ($1, $2, 'PREVENTIVE', $3, $3, 'COMPLETED') RETURNING id`,
		companyID, assetID, today()).Scan(&schedID); err != nil {
		t.Fatal(err)
	}

	if err := handleAssetMaintenanceEvent(ctx, makeEvent(schedID), pools, chClient, nil); err != nil {
		t.Fatalf("handleAssetMaintenanceEvent: %v", err)
	}

	row := chClient.QueryRow(ctx,
		"SELECT count(*) FROM fact_asset_maintenance FINAL WHERE company_id = ?", companyID)
	var n uint64
	if err := row.Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("want 1 asset maintenance, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// parseEntityID unit test — tidak butuh Postgres/ClickHouse
// ---------------------------------------------------------------------------

func TestParseEntityID_ValidUUID(t *testing.T) {
	id := uuid.New()
	raw := makeEvent(id)
	got, err := parseEntityID(raw)
	if err != nil {
		t.Fatalf("parseEntityID error: %v", err)
	}
	if got != id {
		t.Errorf("want %s, got %s", id, got)
	}
}

func TestParseEntityID_InvalidJSON(t *testing.T) {
	if _, err := parseEntityID([]byte("not-json")); err == nil {
		t.Error("want error for invalid JSON")
	}
}

func TestParseEntityID_InvalidUUID(t *testing.T) {
	if _, err := parseEntityID([]byte(`{"entity_id":"not-a-uuid"}`)); err == nil {
		t.Error("want error for invalid UUID")
	}
}
