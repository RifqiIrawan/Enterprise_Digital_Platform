// Package clickhouse adalah wrapper di atas driver native
// github.com/ClickHouse/clickhouse-go/v2, dipakai untuk membuat skema (3
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
