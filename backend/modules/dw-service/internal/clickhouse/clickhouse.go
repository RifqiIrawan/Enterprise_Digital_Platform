// Package clickhouse adalah wrapper di atas driver native
// github.com/ClickHouse/clickhouse-go/v2, dipakai untuk membuat skema (9
// fact table + 1 tabel state watermark) dan batch-insert hasil ETL.
// ClickHouse dipilih sebagai destinasi (bukan Postgres) karena ini kolom-
// store OLAP -- tabel fact di sini sengaja denormalized (pre-joined ke
// context dimensinya saat extract), bukan star schema dengan JOIN saat
// query, mengikuti best practice ClickHouse.
package clickhouse

import (
	"context"
	"fmt"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// toDecimal mengonversi float64 (hasil scan pgx dari kolom NUMERIC Postgres)
// ke decimal.Decimal, satu-satunya tipe Go yang bisa di-bind ke kolom
// Decimal(P,S) ClickHouse lewat driver ini (float64 sengaja TIDAK didukung
// oleh clickhouse-go/v2 untuk kolom Decimal, cuma untuk Float64/Float32 --
// dikonfirmasi lewat TYPES.md driver ini setelah error "converting float64
// to Decimal(18, 2) is unsupported" muncul di test).
func toDecimal(f float64) decimal.Decimal {
	return decimal.NewFromFloat(f)
}

// toDecimalPtr is toDecimal's Nullable(Decimal(P,S)) counterpart -- a nil
// input (Postgres NULL) must stay nil here, not become a zero-value
// decimal.Decimal, or a genuinely-absent value (e.g. work_orders not yet
// COMPLETED, so quantity_produced is still unset) would render as "0" in
// the warehouse instead of NULL.
func toDecimalPtr(f *float64) *decimal.Decimal {
	if f == nil {
		return nil
	}
	d := decimal.NewFromFloat(*f)
	return &d
}

type Client struct {
	conn ch.Conn
}

// Connect membuka koneksi awal ke database "default" untuk memastikan
// database tujuan ada (CREATE DATABASE IF NOT EXISTS), lalu membuka koneksi
// kedua dengan database itu terpilih supaya nama tabel di query lain tidak
// perlu di-qualify. user/password wajib diisi eksplisit -- image resmi
// ClickHouse MEMATIKAN akses network sama sekali untuk user "default" kalau
// CLICKHOUSE_USER/CLICKHOUSE_PASSWORD tidak diset di container (bukan cuma
// butuh password, request ditolak total), jadi tidak ada default "tanpa
// auth" yang benar-benar valid di sini seperti Kafka/Redis/Mosquitto.
func Connect(ctx context.Context, addr, user, password, database string) (*Client, error) {
	bootstrap, err := ch.Open(&ch.Options{
		Addr: []string{addr},
		Auth: ch.Auth{Database: "default", Username: user, Password: password},
	})
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}
	if err := bootstrap.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping clickhouse: %w", err)
	}
	if err := bootstrap.Exec(ctx, "CREATE DATABASE IF NOT EXISTS "+database); err != nil {
		return nil, fmt.Errorf("create database %s: %w", database, err)
	}
	_ = bootstrap.Close()

	conn, err := ch.Open(&ch.Options{
		Addr: []string{addr},
		Auth: ch.Auth{Database: database, Username: user, Password: password},
	})
	if err != nil {
		return nil, fmt.Errorf("open clickhouse database %s: %w", database, err)
	}
	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping clickhouse database %s: %w", database, err)
	}
	return &Client{conn: conn}, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

const schema = `
CREATE TABLE IF NOT EXISTS fact_finance_journal_lines (
    line_id UUID, journal_id UUID, company_id UUID, branch_id Nullable(UUID),
    entry_number String, entry_date Date, period String, reference_type String,
    entry_status String, account_id UUID, account_code String, account_name String,
    account_type String, debit_amount Decimal(18,2), credit_amount Decimal(18,2),
    posted_at Nullable(DateTime), synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY toYYYYMM(entry_date) ORDER BY (company_id, line_id);

CREATE TABLE IF NOT EXISTS fact_sales_order_lines (
    line_id UUID, sales_order_id UUID, company_id UUID, branch_id Nullable(UUID),
    so_number String, order_date Date, order_status String, customer_id UUID,
    customer_code String, customer_name String, product_name String,
    quantity Decimal(12,2), unit_price Decimal(15,2), amount Decimal(15,2),
    updated_at DateTime, synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY toYYYYMM(order_date) ORDER BY (company_id, line_id);

CREATE TABLE IF NOT EXISTS fact_inventory_movements (
    movement_id UUID, company_id UUID, branch_id Nullable(UUID), warehouse_id UUID,
    warehouse_code String, warehouse_name String, product_id UUID,
    product_sku String, product_name String, movement_type String,
    quantity Decimal(15,2), reference_type String, reference_id Nullable(UUID),
    movement_date Date, synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY toYYYYMM(movement_date) ORDER BY (company_id, movement_id);

CREATE TABLE IF NOT EXISTS fact_hr_payroll_details (
    detail_id UUID, payroll_run_id UUID, company_id UUID, branch_id Nullable(UUID),
    period String, run_status String, employee_id UUID, employee_code String,
    employee_name String, department String, basic_salary Decimal(15,2),
    gross_salary Decimal(15,2), total_deduction Decimal(15,2), net_salary Decimal(15,2),
    working_days Int16, present_days Int16, posted_at Nullable(DateTime), synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY period ORDER BY (company_id, detail_id);

CREATE TABLE IF NOT EXISTS fact_purchasing_order_lines (
    line_id UUID, purchase_order_id UUID, company_id UUID, branch_id Nullable(UUID),
    po_number String, order_date Date, order_status String, supplier_id UUID,
    supplier_code String, supplier_name String, product_name String,
    quantity Decimal(12,2), unit_price Decimal(15,2), amount Decimal(15,2),
    updated_at DateTime, synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY toYYYYMM(order_date) ORDER BY (company_id, line_id);

CREATE TABLE IF NOT EXISTS fact_production_work_orders (
    wo_id UUID, company_id UUID, branch_id Nullable(UUID), wo_number String,
    bom_id UUID, product_id UUID, warehouse_id UUID, quantity_planned Decimal(15,2),
    quantity_produced Nullable(Decimal(15,2)), status String, planned_start_date Date,
    planned_end_date Nullable(Date), updated_at DateTime, synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY toYYYYMM(planned_start_date) ORDER BY (company_id, wo_id);

CREATE TABLE IF NOT EXISTS fact_qc_inspections (
    inspection_id UUID, company_id UUID, branch_id Nullable(UUID), inspection_number String,
    standard_id UUID, standard_code String, product_id UUID, reference_type String,
    reference_id Nullable(UUID), reference_number Nullable(String),
    inspected_quantity Decimal(15,2), passed_quantity Decimal(15,2), failed_quantity Decimal(15,2),
    result String, inspection_date Date, updated_at DateTime, synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY toYYYYMM(inspection_date) ORDER BY (company_id, inspection_id);

CREATE TABLE IF NOT EXISTS fact_asset_maintenance (
    schedule_id UUID, company_id UUID, branch_id Nullable(UUID), asset_id UUID,
    asset_code String, asset_name String, maintenance_type String, scheduled_date Date,
    completed_date Nullable(Date), status String, updated_at DateTime, synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY toYYYYMM(scheduled_date) ORDER BY (company_id, schedule_id);

CREATE TABLE IF NOT EXISTS fact_iot_readings (
    reading_id UUID, company_id UUID, branch_id Nullable(UUID), device_id UUID,
    device_code String, device_type String, reading_type String,
    value_numeric Nullable(Decimal(15,4)), value_text Nullable(String),
    recorded_at DateTime, synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY toYYYYMM(recorded_at) ORDER BY (company_id, reading_id);

CREATE TABLE IF NOT EXISTS etl_sync_state (
    source_table String, last_synced_at DateTime
) ENGINE = ReplacingMergeTree(last_synced_at) ORDER BY source_table;
`

// EnsureSchema membuat tabel-tabel di atas kalau belum ada -- idempotent,
// aman dipanggil tiap kali service start, mirip store.Migrate di modul lain
// tapi untuk ClickHouse (tidak ada tabel schema_migrations di sini, "IF NOT
// EXISTS" saja cukup karena skema tidak pernah berubah lewat migrasi
// bertahap seperti Postgres).
func (c *Client) EnsureSchema(ctx context.Context) error {
	for _, stmt := range splitStatements(schema) {
		if err := c.conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("ensure schema: %w", err)
		}
	}
	return nil
}

func splitStatements(sql string) []string {
	var stmts []string
	var current string
	for _, line := range splitLines(sql) {
		current += line + "\n"
		if len(line) > 0 && line[len(line)-1] == ';' {
			stmts = append(stmts, current)
			current = ""
		}
	}
	return stmts
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// GetWatermark mengembalikan last_synced_at untuk sebuah source table, atau
// zero time.Time kalau belum pernah disync (artinya "ambil semua data dari
// awal").
func (c *Client) GetWatermark(ctx context.Context, sourceTable string) (time.Time, error) {
	row := c.conn.QueryRow(ctx, "SELECT last_synced_at FROM etl_sync_state FINAL WHERE source_table = ?", sourceTable)
	var t time.Time
	if err := row.Scan(&t); err != nil {
		return time.Time{}, nil
	}
	return t, nil
}

func (c *Client) SetWatermark(ctx context.Context, sourceTable string, t time.Time) error {
	return c.conn.Exec(ctx, "INSERT INTO etl_sync_state (source_table, last_synced_at) VALUES (?, ?)", sourceTable, t)
}

// QueryRow adalah passthrough tipis ke driver -- dipakai untuk query ad hoc
// yang tidak cukup umum untuk jadi method khusus (mis. verifikasi field di
// test, atau query status yang lebih spesifik di masa depan).
func (c *Client) QueryRow(ctx context.Context, query string, args ...any) driver.Row {
	return c.conn.QueryRow(ctx, query, args...)
}

func (c *Client) CountRows(ctx context.Context, table string) (uint64, error) {
	row := c.conn.QueryRow(ctx, "SELECT count(*) FROM "+table)
	var n uint64
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	return n, nil
}

// MonthlyFinanceSummaryRow adalah satu baris hasil agregasi bulanan revenue
// (kredit akun REVENUE) dan expense (debit akun EXPENSE) dari journal entry
// yang sudah POSTED.
type MonthlyFinanceSummaryRow struct {
	Month   string
	Revenue decimal.Decimal
	Expense decimal.Decimal
}

// MonthlyFinanceSummary agregasi fact_finance_journal_lines per bulan untuk
// satu company -- query analitik pertama yang benar-benar membaca dari
// ClickHouse (bukan cuma CountRows untuk status sync). Pakai FINAL supaya
// baris duplikat dari ReplacingMergeTree (dua jalur tulis: batch ETL 5 menit
// + Kafka Streaming ETL, lihat internal/streaming) yang belum sempat
// di-merge background tidak menghitung revenue/expense dobel.
func (c *Client) MonthlyFinanceSummary(ctx context.Context, companyID uuid.UUID) ([]MonthlyFinanceSummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			toString(toStartOfMonth(entry_date)) AS month,
			sumIf(credit_amount, account_type = 'REVENUE') AS revenue,
			sumIf(debit_amount, account_type = 'EXPENSE') AS expense
		FROM fact_finance_journal_lines FINAL
		WHERE company_id = ? AND entry_status = 'POSTED'
		GROUP BY month
		ORDER BY month
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query monthly finance summary: %w", err)
	}
	defer rows.Close()

	var out []MonthlyFinanceSummaryRow
	for rows.Next() {
		var r MonthlyFinanceSummaryRow
		if err := rows.Scan(&r.Month, &r.Revenue, &r.Expense); err != nil {
			return nil, fmt.Errorf("scan monthly finance summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type FinanceJournalLineRow struct {
	LineID        uuid.UUID
	JournalID     uuid.UUID
	CompanyID     uuid.UUID
	BranchID      *uuid.UUID
	EntryNumber   string
	EntryDate     time.Time
	Period        string
	ReferenceType string
	EntryStatus   string
	AccountID     uuid.UUID
	AccountCode   string
	AccountName   string
	AccountType   string
	DebitAmount   float64
	CreditAmount  float64
	PostedAt      *time.Time
}

func (c *Client) InsertFinanceJournalLines(ctx context.Context, rows []FinanceJournalLineRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_finance_journal_lines")
	if err != nil {
		return fmt.Errorf("prepare finance batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.LineID, r.JournalID, r.CompanyID, r.BranchID, r.EntryNumber, r.EntryDate, r.Period,
			r.ReferenceType, r.EntryStatus, r.AccountID, r.AccountCode, r.AccountName, r.AccountType,
			toDecimal(r.DebitAmount), toDecimal(r.CreditAmount), r.PostedAt, syncedAt,
		); err != nil {
			return fmt.Errorf("append finance row %s: %w", r.LineID, err)
		}
	}
	return batch.Send()
}

type SalesOrderLineRow struct {
	LineID       uuid.UUID
	SalesOrderID uuid.UUID
	CompanyID    uuid.UUID
	BranchID     *uuid.UUID
	SONumber     string
	OrderDate    time.Time
	OrderStatus  string
	CustomerID   uuid.UUID
	CustomerCode string
	CustomerName string
	ProductName  string
	Quantity     float64
	UnitPrice    float64
	Amount       float64
	UpdatedAt    time.Time
}

func (c *Client) InsertSalesOrderLines(ctx context.Context, rows []SalesOrderLineRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_sales_order_lines")
	if err != nil {
		return fmt.Errorf("prepare sales batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.LineID, r.SalesOrderID, r.CompanyID, r.BranchID, r.SONumber, r.OrderDate, r.OrderStatus,
			r.CustomerID, r.CustomerCode, r.CustomerName, r.ProductName, toDecimal(r.Quantity), toDecimal(r.UnitPrice), toDecimal(r.Amount),
			r.UpdatedAt, syncedAt,
		); err != nil {
			return fmt.Errorf("append sales row %s: %w", r.LineID, err)
		}
	}
	return batch.Send()
}

type InventoryMovementRow struct {
	MovementID    uuid.UUID
	CompanyID     uuid.UUID
	BranchID      *uuid.UUID
	WarehouseID   uuid.UUID
	WarehouseCode string
	WarehouseName string
	ProductID     uuid.UUID
	ProductSKU    string
	ProductName   string
	MovementType  string
	Quantity      float64
	ReferenceType string
	ReferenceID   *uuid.UUID
	MovementDate  time.Time
}

func (c *Client) InsertInventoryMovements(ctx context.Context, rows []InventoryMovementRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_inventory_movements")
	if err != nil {
		return fmt.Errorf("prepare inventory batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.MovementID, r.CompanyID, r.BranchID, r.WarehouseID, r.WarehouseCode, r.WarehouseName,
			r.ProductID, r.ProductSKU, r.ProductName, r.MovementType, toDecimal(r.Quantity), r.ReferenceType,
			r.ReferenceID, r.MovementDate, syncedAt,
		); err != nil {
			return fmt.Errorf("append inventory row %s: %w", r.MovementID, err)
		}
	}
	return batch.Send()
}

type HRPayrollDetailRow struct {
	DetailID       uuid.UUID
	PayrollRunID   uuid.UUID
	CompanyID      uuid.UUID
	BranchID       *uuid.UUID
	Period         string
	RunStatus      string
	EmployeeID     uuid.UUID
	EmployeeCode   string
	EmployeeName   string
	Department     string
	BasicSalary    float64
	GrossSalary    float64
	TotalDeduction float64
	NetSalary      float64
	WorkingDays    int16
	PresentDays    int16
	PostedAt       *time.Time
}

func (c *Client) InsertHRPayrollDetails(ctx context.Context, rows []HRPayrollDetailRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_hr_payroll_details")
	if err != nil {
		return fmt.Errorf("prepare hr payroll batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.DetailID, r.PayrollRunID, r.CompanyID, r.BranchID, r.Period, r.RunStatus,
			r.EmployeeID, r.EmployeeCode, r.EmployeeName, r.Department, toDecimal(r.BasicSalary),
			toDecimal(r.GrossSalary), toDecimal(r.TotalDeduction), toDecimal(r.NetSalary),
			r.WorkingDays, r.PresentDays, r.PostedAt, syncedAt,
		); err != nil {
			return fmt.Errorf("append hr payroll row %s: %w", r.DetailID, err)
		}
	}
	return batch.Send()
}

type PurchasingOrderLineRow struct {
	LineID          uuid.UUID
	PurchaseOrderID uuid.UUID
	CompanyID       uuid.UUID
	BranchID        *uuid.UUID
	PONumber        string
	OrderDate       time.Time
	OrderStatus     string
	SupplierID      uuid.UUID
	SupplierCode    string
	SupplierName    string
	ProductName     string
	Quantity        float64
	UnitPrice       float64
	Amount          float64
	UpdatedAt       time.Time
}

func (c *Client) InsertPurchasingOrderLines(ctx context.Context, rows []PurchasingOrderLineRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_purchasing_order_lines")
	if err != nil {
		return fmt.Errorf("prepare purchasing batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.LineID, r.PurchaseOrderID, r.CompanyID, r.BranchID, r.PONumber, r.OrderDate, r.OrderStatus,
			r.SupplierID, r.SupplierCode, r.SupplierName, r.ProductName, toDecimal(r.Quantity),
			toDecimal(r.UnitPrice), toDecimal(r.Amount), r.UpdatedAt, syncedAt,
		); err != nil {
			return fmt.Errorf("append purchasing row %s: %w", r.LineID, err)
		}
	}
	return batch.Send()
}

type ProductionWorkOrderRow struct {
	WOID             uuid.UUID
	CompanyID        uuid.UUID
	BranchID         *uuid.UUID
	WONumber         string
	BOMID            uuid.UUID
	ProductID        uuid.UUID
	WarehouseID      uuid.UUID
	QuantityPlanned  float64
	QuantityProduced *float64
	Status           string
	PlannedStartDate time.Time
	PlannedEndDate   *time.Time
	UpdatedAt        time.Time
}

func (c *Client) InsertProductionWorkOrders(ctx context.Context, rows []ProductionWorkOrderRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_production_work_orders")
	if err != nil {
		return fmt.Errorf("prepare production batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.WOID, r.CompanyID, r.BranchID, r.WONumber, r.BOMID, r.ProductID, r.WarehouseID,
			toDecimal(r.QuantityPlanned), toDecimalPtr(r.QuantityProduced), r.Status,
			r.PlannedStartDate, r.PlannedEndDate, r.UpdatedAt, syncedAt,
		); err != nil {
			return fmt.Errorf("append production row %s: %w", r.WOID, err)
		}
	}
	return batch.Send()
}

type QCInspectionRow struct {
	InspectionID      uuid.UUID
	CompanyID         uuid.UUID
	BranchID          *uuid.UUID
	InspectionNumber  string
	StandardID        uuid.UUID
	StandardCode      string
	ProductID         uuid.UUID
	ReferenceType     string
	ReferenceID       *uuid.UUID
	ReferenceNumber   *string
	InspectedQuantity float64
	PassedQuantity    float64
	FailedQuantity    float64
	Result            string
	InspectionDate    time.Time
	UpdatedAt         time.Time
}

func (c *Client) InsertQCInspections(ctx context.Context, rows []QCInspectionRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_qc_inspections")
	if err != nil {
		return fmt.Errorf("prepare qc batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.InspectionID, r.CompanyID, r.BranchID, r.InspectionNumber, r.StandardID, r.StandardCode,
			r.ProductID, r.ReferenceType, r.ReferenceID, r.ReferenceNumber, toDecimal(r.InspectedQuantity),
			toDecimal(r.PassedQuantity), toDecimal(r.FailedQuantity), r.Result, r.InspectionDate, r.UpdatedAt, syncedAt,
		); err != nil {
			return fmt.Errorf("append qc row %s: %w", r.InspectionID, err)
		}
	}
	return batch.Send()
}

type AssetMaintenanceRow struct {
	ScheduleID      uuid.UUID
	CompanyID       uuid.UUID
	BranchID        *uuid.UUID
	AssetID         uuid.UUID
	AssetCode       string
	AssetName       string
	MaintenanceType string
	ScheduledDate   time.Time
	CompletedDate   *time.Time
	Status          string
	UpdatedAt       time.Time
}

func (c *Client) InsertAssetMaintenance(ctx context.Context, rows []AssetMaintenanceRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_asset_maintenance")
	if err != nil {
		return fmt.Errorf("prepare asset batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.ScheduleID, r.CompanyID, r.BranchID, r.AssetID, r.AssetCode, r.AssetName,
			r.MaintenanceType, r.ScheduledDate, r.CompletedDate, r.Status, r.UpdatedAt, syncedAt,
		); err != nil {
			return fmt.Errorf("append asset row %s: %w", r.ScheduleID, err)
		}
	}
	return batch.Send()
}

type IoTReadingRow struct {
	ReadingID    uuid.UUID
	CompanyID    uuid.UUID
	BranchID     *uuid.UUID
	DeviceID     uuid.UUID
	DeviceCode   string
	DeviceType   string
	ReadingType  string
	ValueNumeric *float64
	ValueText    *string
	RecordedAt   time.Time
}

func (c *Client) InsertIoTReadings(ctx context.Context, rows []IoTReadingRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_iot_readings")
	if err != nil {
		return fmt.Errorf("prepare iot batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.ReadingID, r.CompanyID, r.BranchID, r.DeviceID, r.DeviceCode, r.DeviceType, r.ReadingType,
			toDecimalPtr(r.ValueNumeric), r.ValueText, r.RecordedAt, syncedAt,
		); err != nil {
			return fmt.Errorf("append iot row %s: %w", r.ReadingID, err)
		}
	}
	return batch.Send()
}
