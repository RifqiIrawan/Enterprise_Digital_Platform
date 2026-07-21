package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
	"github.com/enterprise-digital-platform/dw-service/internal/sourcedb"
)

// streamEvent adalah subset minimal envelope audit event yang dipublikasikan
// semua service bisnis. Kita hanya butuh entity_id untuk lookup ke Postgres —
// payload lengkap dalam event sengaja TIDAK dipakai karena tidak berisi data
// yang sudah di-JOIN (customer_name, account_code, dst) yang dibutuhkan fact
// table kita. Postgres selalu jadi source of truth.
type streamEvent struct {
	EntityID string `json:"entity_id"`
}

func parseEntityID(raw []byte) (uuid.UUID, error) {
	var evt streamEvent
	if err := json.Unmarshal(raw, &evt); err != nil {
		return uuid.UUID{}, fmt.Errorf("parse event json: %w", err)
	}
	id, err := uuid.Parse(evt.EntityID)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("parse entity_id %q: %w", evt.EntityID, err)
	}
	return id, nil
}

// insertAndLog adalah helper untuk insert ke ClickHouse + best-effort write ke
// data lake, identik polanya dengan internal/etl: lake boleh nil, kegagalan
// lake tidak menggagalkan insert ClickHouse yang sudah berhasil.
func insertAndLog[T any](
	ctx context.Context,
	dest *ch.Client,
	lake *datalake.Client,
	fact string,
	rows []T,
	insertFn func(context.Context, []T, time.Time) error,
) error {
	if len(rows) == 0 {
		return nil
	}
	syncedAt := time.Now()
	if err := insertFn(ctx, rows, syncedAt); err != nil {
		return fmt.Errorf("insert %s: %w", fact, err)
	}
	if err := lake.WriteJSONLines(ctx, fact, rows, syncedAt); err != nil {
		log.Printf("dw-streaming: lake write %s failed (ClickHouse ok): %v", fact, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Finance — journal.posted
// Satu journal entry bisa punya banyak lines. Query semua lines-nya.
// ---------------------------------------------------------------------------

const financeSingleSQL = `
	SELECT jl.id, jl.journal_id, je.company_id, je.branch_id, je.entry_number, je.entry_date,
	       je.period, je.reference_type, je.status, jl.account_id, a.account_code, a.account_name,
	       a.account_type, jl.debit_amount, jl.credit_amount, je.posted_at
	FROM journal_lines jl
	JOIN journal_entries je ON je.id = jl.journal_id
	JOIN accounts a ON a.id = jl.account_id
	WHERE je.id = $1`

func handleFinanceJournalPosted(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	rows, err := sources.Finance.Query(ctx, financeSingleSQL, id)
	if err != nil {
		return fmt.Errorf("query finance journal %s: %w", id, err)
	}
	defer rows.Close()

	var out []ch.FinanceJournalLineRow
	for rows.Next() {
		var r ch.FinanceJournalLineRow
		if err := rows.Scan(
			&r.LineID, &r.JournalID, &r.CompanyID, &r.BranchID, &r.EntryNumber, &r.EntryDate,
			&r.Period, &r.ReferenceType, &r.EntryStatus, &r.AccountID, &r.AccountCode, &r.AccountName,
			&r.AccountType, &r.DebitAmount, &r.CreditAmount, &r.PostedAt,
		); err != nil {
			return fmt.Errorf("scan finance row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate finance rows: %w", err)
	}
	return insertAndLog(ctx, dest, lake, "finance_journal_lines", out, dest.InsertFinanceJournalLines)
}

// ---------------------------------------------------------------------------
// Sales — order.fulfilled, order.invoiced
// Kedua event ini mengubah status SO → extract ulang semua lines-nya.
// ReplacingMergeTree akan upsert baris yang sama (bukan duplikat).
// ---------------------------------------------------------------------------

const salesSingleSQL = `
	SELECT sol.id, sol.sales_order_id, so.company_id, so.branch_id, so.so_number, so.order_date,
	       so.status, so.customer_id, c.customer_code, c.name, sol.product_name, sol.quantity,
	       sol.unit_price, sol.amount, so.updated_at
	FROM sales_order_lines sol
	JOIN sales_orders so ON so.id = sol.sales_order_id
	JOIN customers c ON c.id = so.customer_id
	WHERE so.id = $1`

func handleSalesOrderEvent(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	rows, err := sources.Sales.Query(ctx, salesSingleSQL, id)
	if err != nil {
		return fmt.Errorf("query sales order %s: %w", id, err)
	}
	defer rows.Close()

	var out []ch.SalesOrderLineRow
	for rows.Next() {
		var r ch.SalesOrderLineRow
		if err := rows.Scan(
			&r.LineID, &r.SalesOrderID, &r.CompanyID, &r.BranchID, &r.SONumber, &r.OrderDate,
			&r.OrderStatus, &r.CustomerID, &r.CustomerCode, &r.CustomerName, &r.ProductName,
			&r.Quantity, &r.UnitPrice, &r.Amount, &r.UpdatedAt,
		); err != nil {
			return fmt.Errorf("scan sales row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate sales rows: %w", err)
	}
	return insertAndLog(ctx, dest, lake, "sales_order_lines", out, dest.InsertSalesOrderLines)
}

// ---------------------------------------------------------------------------
// Inventory — stock.moved (single movement), stock.batch_moved (batch by ref)
// ---------------------------------------------------------------------------

func queryInventory(ctx context.Context, pool *pgxpool.Pool, query string, arg any) ([]ch.InventoryMovementRow, error) {
	rows, err := pool.Query(ctx, query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ch.InventoryMovementRow
	for rows.Next() {
		var r ch.InventoryMovementRow
		if err := rows.Scan(
			&r.MovementID, &r.CompanyID, &r.BranchID, &r.WarehouseID, &r.WarehouseCode, &r.WarehouseName,
			&r.ProductID, &r.ProductSKU, &r.ProductName, &r.MovementType, &r.Quantity, &r.ReferenceType,
			&r.ReferenceID, &r.MovementDate,
		); err != nil {
			return nil, fmt.Errorf("scan inventory row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const inventorySingleSQL = `
	SELECT sm.id, sm.company_id, sm.branch_id, sm.warehouse_id, w.code, w.name, sm.product_id,
	       p.sku, p.name, sm.movement_type, sm.quantity, sm.reference_type, sm.reference_id,
	       sm.movement_date
	FROM stock_movements sm
	JOIN warehouses w ON w.id = sm.warehouse_id
	JOIN products p ON p.id = sm.product_id
	WHERE sm.id = $1`

const inventoryBatchSQL = `
	SELECT sm.id, sm.company_id, sm.branch_id, sm.warehouse_id, w.code, w.name, sm.product_id,
	       p.sku, p.name, sm.movement_type, sm.quantity, sm.reference_type, sm.reference_id,
	       sm.movement_date
	FROM stock_movements sm
	JOIN warehouses w ON w.id = sm.warehouse_id
	JOIN products p ON p.id = sm.product_id
	WHERE sm.reference_id = $1`

func handleStockMoved(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	out, err := queryInventory(ctx, sources.Warehouse, inventorySingleSQL, id)
	if err != nil {
		return fmt.Errorf("query stock movement %s: %w", id, err)
	}
	return insertAndLog(ctx, dest, lake, "inventory_movements", out, dest.InsertInventoryMovements)
}

func handleStockBatchMoved(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	// entity_id untuk batch_moved adalah reference_id (PO/SO/WO id) —
	// query semua movements milik referensi itu.
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	out, err := queryInventory(ctx, sources.Warehouse, inventoryBatchSQL, id)
	if err != nil {
		return fmt.Errorf("query batch stock movements ref=%s: %w", id, err)
	}
	return insertAndLog(ctx, dest, lake, "inventory_movements", out, dest.InsertInventoryMovements)
}

// ---------------------------------------------------------------------------
// HR — payroll.posted
// ---------------------------------------------------------------------------

const hrSingleSQL = `
	SELECT pd.id, pd.payroll_run_id, pr.company_id, pr.branch_id, pr.period, pr.status,
	       pd.employee_id, e.employee_code, pd.employee_name, COALESCE(e.department, ''),
	       pd.basic_salary, pd.gross_salary, pd.total_deduction, pd.net_salary,
	       pd.working_days, pd.present_days, pr.posted_at
	FROM payroll_details pd
	JOIN payroll_runs pr ON pr.id = pd.payroll_run_id
	JOIN employees e ON e.id = pd.employee_id
	WHERE pr.id = $1`

func handleHRPayrollPosted(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	rows, err := sources.HR.Query(ctx, hrSingleSQL, id)
	if err != nil {
		return fmt.Errorf("query payroll run %s: %w", id, err)
	}
	defer rows.Close()

	var out []ch.HRPayrollDetailRow
	for rows.Next() {
		var r ch.HRPayrollDetailRow
		if err := rows.Scan(
			&r.DetailID, &r.PayrollRunID, &r.CompanyID, &r.BranchID, &r.Period, &r.RunStatus,
			&r.EmployeeID, &r.EmployeeCode, &r.EmployeeName, &r.Department,
			&r.BasicSalary, &r.GrossSalary, &r.TotalDeduction, &r.NetSalary,
			&r.WorkingDays, &r.PresentDays, &r.PostedAt,
		); err != nil {
			return fmt.Errorf("scan hr payroll row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate hr rows: %w", err)
	}
	return insertAndLog(ctx, dest, lake, "hr_payroll_details", out, dest.InsertHRPayrollDetails)
}

// ---------------------------------------------------------------------------
// Purchasing — order.received, order.invoiced
// Sama seperti Sales: status PO berubah → extract ulang semua lines-nya.
// ---------------------------------------------------------------------------

const purchasingSingleSQL = `
	SELECT pol.id, pol.purchase_order_id, po.company_id, po.branch_id, po.po_number, po.order_date,
	       po.status, po.supplier_id, s.supplier_code, s.name, pol.product_name, pol.quantity,
	       pol.unit_price, pol.amount, po.updated_at
	FROM purchase_order_lines pol
	JOIN purchase_orders po ON po.id = pol.purchase_order_id
	JOIN suppliers s ON s.id = po.supplier_id
	WHERE po.id = $1`

func handlePurchasingOrderEvent(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	rows, err := sources.Purchasing.Query(ctx, purchasingSingleSQL, id)
	if err != nil {
		return fmt.Errorf("query purchase order %s: %w", id, err)
	}
	defer rows.Close()

	var out []ch.PurchasingOrderLineRow
	for rows.Next() {
		var r ch.PurchasingOrderLineRow
		if err := rows.Scan(
			&r.LineID, &r.PurchaseOrderID, &r.CompanyID, &r.BranchID, &r.PONumber, &r.OrderDate,
			&r.OrderStatus, &r.SupplierID, &r.SupplierCode, &r.SupplierName, &r.ProductName,
			&r.Quantity, &r.UnitPrice, &r.Amount, &r.UpdatedAt,
		); err != nil {
			return fmt.Errorf("scan purchasing row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate purchasing rows: %w", err)
	}
	return insertAndLog(ctx, dest, lake, "purchasing_order_lines", out, dest.InsertPurchasingOrderLines)
}

// ---------------------------------------------------------------------------
// Production — work_order.completed
// work_orders adalah satu baris per WO (bukan lines), jadi QueryRow cukup.
// ---------------------------------------------------------------------------

const productionSingleSQL = `
	SELECT wo.id, wo.company_id, wo.branch_id, wo.wo_number, wo.bom_id, wo.product_id,
	       wo.warehouse_id, wo.quantity_planned, wo.quantity_produced, wo.status,
	       wo.planned_start_date, wo.planned_end_date, wo.updated_at
	FROM work_orders wo
	WHERE wo.id = $1`

func handleProductionWOCompleted(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	rows, err := sources.Production.Query(ctx, productionSingleSQL, id)
	if err != nil {
		return fmt.Errorf("query work order %s: %w", id, err)
	}
	defer rows.Close()

	var out []ch.ProductionWorkOrderRow
	for rows.Next() {
		var r ch.ProductionWorkOrderRow
		if err := rows.Scan(
			&r.WOID, &r.CompanyID, &r.BranchID, &r.WONumber, &r.BOMID, &r.ProductID,
			&r.WarehouseID, &r.QuantityPlanned, &r.QuantityProduced, &r.Status,
			&r.PlannedStartDate, &r.PlannedEndDate, &r.UpdatedAt,
		); err != nil {
			return fmt.Errorf("scan production row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate production rows: %w", err)
	}
	return insertAndLog(ctx, dest, lake, "production_work_orders", out, dest.InsertProductionWorkOrders)
}

// ---------------------------------------------------------------------------
// QC — inspection.created
// ---------------------------------------------------------------------------

const qcSingleSQL = `
	SELECT qi.id, qi.company_id, qi.branch_id, qi.inspection_number, qi.standard_id, qs.standard_code,
	       qi.product_id, qi.reference_type, qi.reference_id, qi.reference_number,
	       qi.inspected_quantity, qi.passed_quantity, qi.failed_quantity, qi.result,
	       qi.inspection_date, qi.updated_at
	FROM quality_inspections qi
	JOIN quality_standards qs ON qs.id = qi.standard_id
	WHERE qi.id = $1`

func handleQCInspectionCreated(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	rows, err := sources.QC.Query(ctx, qcSingleSQL, id)
	if err != nil {
		return fmt.Errorf("query qc inspection %s: %w", id, err)
	}
	defer rows.Close()

	var out []ch.QCInspectionRow
	for rows.Next() {
		var r ch.QCInspectionRow
		if err := rows.Scan(
			&r.InspectionID, &r.CompanyID, &r.BranchID, &r.InspectionNumber, &r.StandardID, &r.StandardCode,
			&r.ProductID, &r.ReferenceType, &r.ReferenceID, &r.ReferenceNumber,
			&r.InspectedQuantity, &r.PassedQuantity, &r.FailedQuantity, &r.Result,
			&r.InspectionDate, &r.UpdatedAt,
		); err != nil {
			return fmt.Errorf("scan qc row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate qc rows: %w", err)
	}
	return insertAndLog(ctx, dest, lake, "qc_inspections", out, dest.InsertQCInspections)
}

// ---------------------------------------------------------------------------
// Asset — maintenance.completed, maintenance.cancelled
// Kedua event dipetakan ke handler yang sama (status berubah, extract ulang).
// ---------------------------------------------------------------------------

const assetSingleSQL = `
	SELECT ms.id, ms.company_id, ms.branch_id, ms.asset_id, a.asset_code, a.name,
	       ms.maintenance_type, ms.scheduled_date, ms.completed_date, ms.status, ms.updated_at
	FROM maintenance_schedules ms
	JOIN assets a ON a.id = ms.asset_id
	WHERE ms.id = $1`

func handleAssetMaintenanceEvent(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	rows, err := sources.Asset.Query(ctx, assetSingleSQL, id)
	if err != nil {
		return fmt.Errorf("query maintenance schedule %s: %w", id, err)
	}
	defer rows.Close()

	var out []ch.AssetMaintenanceRow
	for rows.Next() {
		var r ch.AssetMaintenanceRow
		if err := rows.Scan(
			&r.ScheduleID, &r.CompanyID, &r.BranchID, &r.AssetID, &r.AssetCode, &r.AssetName,
			&r.MaintenanceType, &r.ScheduledDate, &r.CompletedDate, &r.Status, &r.UpdatedAt,
		); err != nil {
			return fmt.Errorf("scan asset row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate asset rows: %w", err)
	}
	return insertAndLog(ctx, dest, lake, "asset_maintenance", out, dest.InsertAssetMaintenance)
}
