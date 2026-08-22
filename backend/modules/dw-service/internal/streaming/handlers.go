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

// ---------------------------------------------------------------------------
// CRM — opportunity.won, opportunity.lost
// Kedua event dipetakan ke handler yang sama (stage berubah ke status
// terminal, extract ulang) -- mirror duality asset completed/cancelled.
// ---------------------------------------------------------------------------

const crmSingleSQL = `
	SELECT o.id, o.company_id, o.branch_id, o.opportunity_number, o.account_id, a.name AS account_name,
	       o.contact_id, o.name AS opportunity_name, o.stage, o.amount, o.probability,
	       o.expected_close_date, o.owner_user_id, o.created_at, o.updated_at
	FROM opportunities o
	JOIN accounts a ON a.id = o.account_id
	WHERE o.id = $1`

func handleCRMOpportunityEvent(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	rows, err := sources.CRM.Query(ctx, crmSingleSQL, id)
	if err != nil {
		return fmt.Errorf("query opportunity %s: %w", id, err)
	}
	defer rows.Close()

	var out []ch.CRMOpportunityRow
	for rows.Next() {
		var r ch.CRMOpportunityRow
		if err := rows.Scan(
			&r.OpportunityID, &r.CompanyID, &r.BranchID, &r.OpportunityNumber, &r.AccountID, &r.AccountName,
			&r.ContactID, &r.OpportunityName, &r.Stage, &r.Amount, &r.Probability,
			&r.ExpectedCloseDate, &r.OwnerUserID, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return fmt.Errorf("scan crm row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate crm rows: %w", err)
	}
	return insertAndLog(ctx, dest, lake, "opportunities", out, dest.InsertCRMOpportunities)
}

// ---------------------------------------------------------------------------
// Ticketing — ticket.closed, ticket.reopened
// Kedua event dipetakan ke handler yang sama (status terminal berubah,
// extract ulang).
// ---------------------------------------------------------------------------

const ticketingSingleSQL = `
	SELECT t.id, t.company_id, t.branch_id, t.ticket_number, t.category_id, c.name AS category_name,
	       t.subject, t.priority, t.status, t.requester_name, t.requester_email, t.assigned_to,
	       t.created_at, t.resolved_at, t.closed_at, t.updated_at
	FROM tickets t
	JOIN ticket_categories c ON c.id = t.category_id
	WHERE t.id = $1`

func handleTicketingTicketEvent(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	rows, err := sources.Ticketing.Query(ctx, ticketingSingleSQL, id)
	if err != nil {
		return fmt.Errorf("query ticket %s: %w", id, err)
	}
	defer rows.Close()

	var out []ch.TicketingTicketRow
	for rows.Next() {
		var r ch.TicketingTicketRow
		if err := rows.Scan(
			&r.TicketID, &r.CompanyID, &r.BranchID, &r.TicketNumber, &r.CategoryID, &r.CategoryName,
			&r.Subject, &r.Priority, &r.Status, &r.RequesterName, &r.RequesterEmail, &r.AssignedTo,
			&r.CreatedAt, &r.ResolvedAt, &r.ClosedAt, &r.UpdatedAt,
		); err != nil {
			return fmt.Errorf("scan ticketing row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate ticketing rows: %w", err)
	}
	return insertAndLog(ctx, dest, lake, "tickets", out, dest.InsertTicketingTickets)
}

// ---------------------------------------------------------------------------
// E-Commerce — order.paid, order.shipped
// Kedua event dipetakan ke handler yang sama (status order berubah, extract
// ulang seluruh order_items-nya) -- mirror pola sales fulfilled/invoiced.
// ---------------------------------------------------------------------------

const ecommerceSingleSQL = `
	SELECT oi.id, oi.order_id, o.company_id, o.branch_id, o.order_number, o.order_date, o.status,
	       o.customer_name, o.customer_email, oi.product_id, oi.product_sku, oi.product_name,
	       oi.quantity, oi.unit_price, oi.line_total, o.updated_at
	FROM order_items oi
	JOIN orders o ON o.id = oi.order_id
	WHERE o.id = $1`

func handleEcommerceOrderLineEvent(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	rows, err := sources.Ecommerce.Query(ctx, ecommerceSingleSQL, id)
	if err != nil {
		return fmt.Errorf("query order %s: %w", id, err)
	}
	defer rows.Close()

	var out []ch.EcommerceOrderLineRow
	for rows.Next() {
		var r ch.EcommerceOrderLineRow
		if err := rows.Scan(
			&r.LineID, &r.OrderID, &r.CompanyID, &r.BranchID, &r.OrderNumber, &r.OrderDate, &r.OrderStatus,
			&r.CustomerName, &r.CustomerEmail, &r.ProductID, &r.ProductSKU, &r.ProductName,
			&r.Quantity, &r.UnitPrice, &r.Amount, &r.UpdatedAt,
		); err != nil {
			return fmt.Errorf("scan ecommerce row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate ecommerce rows: %w", err)
	}
	return insertAndLog(ctx, dest, lake, "order_items", out, dest.InsertEcommerceOrderLines)
}

// ---------------------------------------------------------------------------
// Fleet — delivery.dispatched, delivery.delivered, delivery.cancelled
// Ketiganya dipetakan ke handler yang sama: yang berubah adalah status +
// timestamp transisi surat jalan, jadi barisnya cukup di-extract ulang.
// delivery.created SENGAJA tidak ikut -- surat jalan yang baru dibuat belum
// punya nilai analitik apa pun (belum berangkat), dan batch ETL 5 menit sudah
// cukup untuk menangkapnya.
// ---------------------------------------------------------------------------

const fleetSingleSQL = `
	SELECT d.id, d.company_id, d.branch_id, d.delivery_number,
	       d.vehicle_id, v.vehicle_code, v.vehicle_type,
	       d.driver_id, dr.driver_code, dr.name AS driver_name,
	       d.ecommerce_order_id, d.reference_number, d.recipient_name,
	       d.scheduled_date, d.status, d.dispatched_at, d.delivered_at, d.cancelled_at,
	       d.created_at, d.updated_at
	FROM delivery_orders d
	JOIN vehicles v ON v.id = d.vehicle_id
	JOIN drivers dr ON dr.id = d.driver_id
	WHERE d.id = $1`

func handleFleetDeliveryEvent(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	rows, err := sources.Fleet.Query(ctx, fleetSingleSQL, id)
	if err != nil {
		return fmt.Errorf("query delivery order %s: %w", id, err)
	}
	defer rows.Close()

	var out []ch.FleetDeliveryOrderRow
	for rows.Next() {
		var r ch.FleetDeliveryOrderRow
		if err := rows.Scan(
			&r.DeliveryID, &r.CompanyID, &r.BranchID, &r.DeliveryNumber,
			&r.VehicleID, &r.VehicleCode, &r.VehicleType,
			&r.DriverID, &r.DriverCode, &r.DriverName,
			&r.EcommerceOrderID, &r.ReferenceNumber, &r.RecipientName,
			&r.ScheduledDate, &r.Status, &r.DispatchedAt, &r.DeliveredAt, &r.CancelledAt,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return fmt.Errorf("scan fleet row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate fleet rows: %w", err)
	}
	return insertAndLog(ctx, dest, lake, "delivery_orders", out, dest.InsertFleetDeliveryOrders)
}

// ---------------------------------------------------------------------------
// Project — timesheet.approved (satu baris) dan cost.posted (banyak baris)
//
// Kedua event punya entity_id dengan ARTI BERBEDA, jadi masing-masing punya
// query sendiri: timesheet.approved membawa id TIMESHEET, sementara
// cost.posted membawa id PROYEK karena satu posting mengubah semua timesheet
// APPROVED milik proyek itu sekaligus. Memakai satu query untuk keduanya akan
// diam-diam melewatkan sebagian besar baris yang berubah saat posting.
// ---------------------------------------------------------------------------

const projectTimesheetSelect = `
	SELECT ts.id, ts.company_id, ts.branch_id, ts.project_id,
	       p.project_code, p.name AS project_name, p.status AS project_status,
	       ts.task_id, t.task_number,
	       ts.employee_id, ts.employee_name, ts.work_date,
	       ts.hours, ts.hourly_rate, ts.amount,
	       ts.status, ts.approved_at, ts.posted_at, ts.journal_entry_id,
	       ts.created_at, ts.updated_at
	FROM timesheets ts
	JOIN projects p ON p.id = ts.project_id
	LEFT JOIN tasks t ON t.id = ts.task_id
	WHERE `

const projectSingleSQL = projectTimesheetSelect + `ts.id = $1`

const projectByProjectSQL = projectTimesheetSelect + `ts.project_id = $1`

func handleProjectTimesheetEvent(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	return syncProjectTimesheets(ctx, raw, sources, dest, lake, projectSingleSQL)
}

func handleProjectCostPostedEvent(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	return syncProjectTimesheets(ctx, raw, sources, dest, lake, projectByProjectSQL)
}

func syncProjectTimesheets(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client, query string) error {
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	rows, err := sources.Project.Query(ctx, query, id)
	if err != nil {
		return fmt.Errorf("query timesheets for %s: %w", id, err)
	}
	defer rows.Close()

	var out []ch.ProjectTimesheetRow
	for rows.Next() {
		var r ch.ProjectTimesheetRow
		if err := rows.Scan(
			&r.TimesheetID, &r.CompanyID, &r.BranchID, &r.ProjectID,
			&r.ProjectCode, &r.ProjectName, &r.ProjectStatus,
			&r.TaskID, &r.TaskNumber,
			&r.EmployeeID, &r.EmployeeName, &r.WorkDate,
			&r.Hours, &r.HourlyRate, &r.Amount,
			&r.Status, &r.ApprovedAt, &r.PostedAt, &r.JournalEntryID,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return fmt.Errorf("scan project row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project rows: %w", err)
	}
	return insertAndLog(ctx, dest, lake, "timesheets", out, dest.InsertProjectTimesheets)
}

// ---------------------------------------------------------------------------
// HR — cuti & penilaian KPI
//
// Keduanya cuma berubah lewat transisi status, jadi satu handler per entitas
// cukup: baris yang sama di-extract ulang apa adanya dan ReplacingMergeTree
// yang menggantikan versi lamanya. Cuti yang DITOLAK/DIBATALKAN pun ikut
// dikirim -- fact_hr_leave_requests memang menyimpan seluruh status, dan
// penyaringannya baru dilakukan di query ringkasan.
// ---------------------------------------------------------------------------

const hrLeaveSingleSQL = `
	SELECT lr.id, lr.company_id, lr.branch_id, lr.employee_id, e.employee_code,
	       lr.employee_name, COALESCE(e.department, ''), lr.leave_type, lr.status,
	       lr.start_date, lr.end_date, lr.total_days, lr.decided_at,
	       lr.created_at, lr.updated_at
	FROM leave_requests lr
	JOIN employees e ON e.id = lr.employee_id
	WHERE lr.id = $1`

func handleHRLeaveEvent(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	rows, err := sources.HR.Query(ctx, hrLeaveSingleSQL, id)
	if err != nil {
		return fmt.Errorf("query leave request %s: %w", id, err)
	}
	defer rows.Close()

	var out []ch.HRLeaveRow
	for rows.Next() {
		var r ch.HRLeaveRow
		if err := rows.Scan(
			&r.LeaveID, &r.CompanyID, &r.BranchID, &r.EmployeeID, &r.EmployeeCode,
			&r.EmployeeName, &r.Department, &r.LeaveType, &r.Status,
			&r.StartDate, &r.EndDate, &r.TotalDays, &r.DecidedAt, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return fmt.Errorf("scan hr leave row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate hr leave rows: %w", err)
	}
	return insertAndLog(ctx, dest, lake, "hr_leave_requests", out, dest.InsertHRLeaveRequests)
}

const hrKPISingleSQL = `
	SELECT kr.id, kr.company_id, kr.branch_id, kr.employee_id, e.employee_code,
	       kr.employee_name, COALESCE(e.department, ''), kr.period, kr.status,
	       kr.total_score, kr.rating, kr.decided_at, kr.created_at, kr.updated_at
	FROM kpi_reviews kr
	JOIN employees e ON e.id = kr.employee_id
	WHERE kr.id = $1`

func handleHRKPIReviewEvent(ctx context.Context, raw []byte, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client) error {
	id, err := parseEntityID(raw)
	if err != nil {
		return err
	}
	rows, err := sources.HR.Query(ctx, hrKPISingleSQL, id)
	if err != nil {
		return fmt.Errorf("query kpi review %s: %w", id, err)
	}
	defer rows.Close()

	var out []ch.HRKPIReviewRow
	for rows.Next() {
		var r ch.HRKPIReviewRow
		if err := rows.Scan(
			&r.ReviewID, &r.CompanyID, &r.BranchID, &r.EmployeeID, &r.EmployeeCode,
			&r.EmployeeName, &r.Department, &r.Period, &r.Status,
			&r.TotalScore, &r.Rating, &r.DecidedAt, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return fmt.Errorf("scan hr kpi row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate hr kpi rows: %w", err)
	}
	return insertAndLog(ctx, dest, lake, "hr_kpi_reviews", out, dest.InsertHRKPIReviews)
}
