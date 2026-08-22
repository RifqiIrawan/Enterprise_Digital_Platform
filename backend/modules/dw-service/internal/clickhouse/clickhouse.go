// Package clickhouse adalah wrapper di atas driver native
// github.com/ClickHouse/clickhouse-go/v2, dipakai untuk membuat skema (12
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

CREATE TABLE IF NOT EXISTS fact_crm_opportunities (
    opportunity_id UUID, company_id UUID, branch_id Nullable(UUID),
    opportunity_number String, account_id UUID, account_name String,
    contact_id Nullable(UUID), opportunity_name String, stage String,
    amount Decimal(15,2), probability Int32,
    expected_close_date Nullable(Date), owner_user_id Nullable(UUID),
    created_at DateTime, updated_at DateTime, synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY toYYYYMM(created_at) ORDER BY (company_id, opportunity_id);

CREATE TABLE IF NOT EXISTS fact_ticketing_tickets (
    ticket_id UUID, company_id UUID, branch_id Nullable(UUID),
    ticket_number String, category_id UUID, category_name String,
    subject String, priority String, status String,
    requester_name String, requester_email Nullable(String), assigned_to Nullable(UUID),
    created_at DateTime, resolved_at Nullable(DateTime), closed_at Nullable(DateTime),
    updated_at DateTime, synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY toYYYYMM(created_at) ORDER BY (company_id, ticket_id);

CREATE TABLE IF NOT EXISTS fact_ecommerce_order_lines (
    line_id UUID, order_id UUID, company_id UUID, branch_id Nullable(UUID),
    order_number String, order_date Date, order_status String,
    customer_name String, customer_email Nullable(String),
    product_id UUID, product_sku String, product_name String,
    quantity Decimal(18,3), unit_price Decimal(18,2), amount Decimal(18,2),
    updated_at DateTime, synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY toYYYYMM(order_date) ORDER BY (company_id, line_id);

CREATE TABLE IF NOT EXISTS fact_fleet_delivery_orders (
    delivery_id UUID, company_id UUID, branch_id Nullable(UUID),
    delivery_number String,
    vehicle_id UUID, vehicle_code String, vehicle_type String,
    driver_id UUID, driver_code String, driver_name String,
    ecommerce_order_id Nullable(UUID), reference_number Nullable(String),
    recipient_name String, scheduled_date Date, status String,
    dispatched_at Nullable(DateTime), delivered_at Nullable(DateTime), cancelled_at Nullable(DateTime),
    created_at DateTime, updated_at DateTime, synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY toYYYYMM(created_at) ORDER BY (company_id, delivery_id);

CREATE TABLE IF NOT EXISTS fact_project_timesheets (
    timesheet_id UUID, company_id UUID, branch_id Nullable(UUID),
    project_id UUID, project_code String, project_name String, project_status String,
    task_id Nullable(UUID), task_number Nullable(String),
    employee_id UUID, employee_name String, work_date Date,
    hours Decimal(18,2), hourly_rate Decimal(18,2), amount Decimal(18,2),
    status String, approved_at Nullable(DateTime), posted_at Nullable(DateTime),
    journal_entry_id Nullable(UUID),
    created_at DateTime, updated_at DateTime, synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY toYYYYMM(work_date) ORDER BY (company_id, timesheet_id);

CREATE TABLE IF NOT EXISTS fact_hr_leave_requests (
    leave_id UUID, company_id UUID, branch_id Nullable(UUID),
    employee_id UUID, employee_code String, employee_name String, department String,
    leave_type String, status String, start_date Date, end_date Date,
    total_days Int16, decided_at Nullable(DateTime),
    created_at DateTime, updated_at DateTime, synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY toYYYYMM(start_date) ORDER BY (company_id, leave_id);

CREATE TABLE IF NOT EXISTS fact_hr_kpi_reviews (
    review_id UUID, company_id UUID, branch_id Nullable(UUID),
    employee_id UUID, employee_code String, employee_name String, department String,
    period String, status String, total_score Decimal(6,2), rating String,
    decided_at Nullable(DateTime), created_at DateTime, updated_at DateTime, synced_at DateTime
) ENGINE = ReplacingMergeTree(synced_at)
PARTITION BY period ORDER BY (company_id, review_id);

CREATE TABLE IF NOT EXISTS etl_sync_state (
    source_table String, last_synced_at DateTime
) ENGINE = ReplacingMergeTree(last_synced_at) ORDER BY source_table;

-- agg_finance_monthly_line_state + mv_finance_monthly_line_state: Materialized
-- View pertama di dw-service, backing MonthlyFinanceSummary (lihat bagian
-- bawah file ini untuk kenapa desainnya BUKAN SummingMergeTree/
-- AggregatingMergeTree sum-langsung yang lebih sederhana.
CREATE TABLE IF NOT EXISTS agg_finance_monthly_line_state (
    company_id UUID, month Date, line_id UUID,
    revenue_state AggregateFunction(argMax, Decimal(18,2), DateTime),
    expense_state AggregateFunction(argMax, Decimal(18,2), DateTime)
) ENGINE = AggregatingMergeTree
PARTITION BY toYYYYMM(month) ORDER BY (company_id, month, line_id);

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_finance_monthly_line_state
TO agg_finance_monthly_line_state
AS SELECT
    company_id, toStartOfMonth(entry_date) AS month, line_id,
    argMaxState(if(account_type = 'REVENUE', credit_amount, toDecimal64(0, 2)), synced_at) AS revenue_state,
    argMaxState(if(account_type = 'EXPENSE', debit_amount, toDecimal64(0, 2)), synced_at) AS expense_state
FROM fact_finance_journal_lines
WHERE entry_status = 'POSTED'
GROUP BY company_id, month, line_id;
`

// financeMonthlyStateBackfillSQL adalah query backfill SEKALI SAJA untuk
// baris yang sudah ada di fact_finance_journal_lines SEBELUM MV ini dibuat --
// ClickHouse MATERIALIZED VIEW hanya memproses baris yang di-INSERT SETELAH
// MV dibuat, tidak backfill data historis secara otomatis (beda dari
// `POPULATE`, yang TIDAK BISA dipakai bersama `TO <target table>` di versi
// ClickHouse ini -- dikonfirmasi lewat percobaan langsung, error
// "you can't declare both 'TO ...' and 'POPULATE'"). Query ini SENGAJA
// identik dengan SELECT di definisi MV di atas -- keduanya harus tetap
// sinkron kalau salah satu diubah.
const financeMonthlyStateBackfillSQL = `
INSERT INTO agg_finance_monthly_line_state
SELECT
    company_id, toStartOfMonth(entry_date) AS month, line_id,
    argMaxState(if(account_type = 'REVENUE', credit_amount, toDecimal64(0, 2)), synced_at) AS revenue_state,
    argMaxState(if(account_type = 'EXPENSE', debit_amount, toDecimal64(0, 2)), synced_at) AS expense_state
FROM fact_finance_journal_lines
WHERE entry_status = 'POSTED'
GROUP BY company_id, month, line_id
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
	if err := c.backfillFinanceMonthlyState(ctx); err != nil {
		return fmt.Errorf("ensure schema: %w", err)
	}
	return nil
}

// backfillFinanceMonthlyState menjalankan financeMonthlyStateBackfillSQL
// SEKALI SAJA -- dijaga oleh count() check supaya EnsureSchema tetap murah
// dipanggil tiap startup service pada steady state (tanpa guard ini, tiap
// restart akan full-scan ulang fact_finance_journal_lines dan menulis baris
// state duplikat, benar secara hasil query karena argMax deterministik tapi
// boros). Guard ini cukup untuk kasus MV baru dibuat sekali di awal umur
// service; kalau MV di-drop lalu dibuat ulang secara manual di masa depan,
// backfill perlu dipicu manual juga (count() != 0 lagi setelah baris pertama
// masuk lewat sinkronisasi berikutnya).
func (c *Client) backfillFinanceMonthlyState(ctx context.Context) error {
	n, err := c.CountRows(ctx, "agg_finance_monthly_line_state")
	if err != nil {
		return fmt.Errorf("check agg_finance_monthly_line_state: %w", err)
	}
	if n > 0 {
		return nil
	}
	if err := c.conn.Exec(ctx, financeMonthlyStateBackfillSQL); err != nil {
		return fmt.Errorf("backfill agg_finance_monthly_line_state: %w", err)
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
	Month   string          `json:"month"`
	Revenue decimal.Decimal `json:"revenue"`
	Expense decimal.Decimal `json:"expense"`
}

// MonthlyFinanceSummary agregasi fact_finance_journal_lines per bulan untuk
// satu company -- query analitik pertama yang benar-benar membaca dari
// ClickHouse (bukan cuma CountRows untuk status sync).
//
// Dibaca dari agg_finance_monthly_line_state (Materialized View
// mv_finance_monthly_line_state), BUKAN langsung dari fact_finance_journal_lines
// FINAL seperti versi awal endpoint ini -- MV pre-agregasi di setiap INSERT
// (bukan di setiap query), sehingga tabel yang di-scan di sini jauh lebih
// sempit (5 kolom kecil per baris vs 17 kolom fact table penuh).
//
// Query ini SENGAJA dua tingkat, BUKAN sum() langsung satu tingkat:
//  1. Tingkat dalam: argMaxMerge per (company_id, month, line_id) --
//     menyelesaikan nilai TERAKHIR tiap baris journal line, persis makna
//     FINAL di tabel asli. Ini WAJIB ada karena dw-service dual-write ke
//     fact_finance_journal_lines (batch ETL 5 menit DAN Kafka Streaming ETL,
//     lihat internal/streaming) -- line_id yang sama bisa di-INSERT dua kali
//     dengan synced_at berbeda. MV memproses tiap event INSERT independen
//     (tidak menunggu merge ReplacingMergeTree di background), jadi tanpa
//     argMax di tingkat ini, revenue/expense akan terhitung DOBEL untuk
//     setiap baris yang di-dual-write. Dibuktikan lewat
//     TestMonthlyFinanceSummary_DualWriteDoesNotDoubleCount (2 InsertFinance-
//     JournalLines terpisah dengan line_id sama, synced_at berbeda, meniru
//     proses batch+streaming yang benar-benar terpisah).
//  2. Tingkat luar: sum() biasa per bulan dari hasil tingkat dalam yang
//     sudah di-dedup per baris.
func (c *Client) MonthlyFinanceSummary(ctx context.Context, companyID uuid.UUID) ([]MonthlyFinanceSummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			toString(month) AS month,
			sum(revenue) AS revenue,
			sum(expense) AS expense
		FROM (
			SELECT
				company_id, month, line_id,
				argMaxMerge(revenue_state) AS revenue,
				argMaxMerge(expense_state) AS expense
			FROM agg_finance_monthly_line_state
			WHERE company_id = ?
			GROUP BY company_id, month, line_id
		)
		GROUP BY month
		ORDER BY month
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query monthly finance summary: %w", err)
	}
	defer rows.Close()

	out := []MonthlyFinanceSummaryRow{}
	for rows.Next() {
		var r MonthlyFinanceSummaryRow
		if err := rows.Scan(&r.Month, &r.Revenue, &r.Expense); err != nil {
			return nil, fmt.Errorf("scan monthly finance summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MonthlyStockMovementSummaryRow adalah satu baris hasil agregasi bulanan
// stok masuk (movement_type IN) dan stok keluar (movement_type OUT) dari
// fact_inventory_movements.
type MonthlyStockMovementSummaryRow struct {
	Month    string          `json:"month"`
	StockIn  decimal.Decimal `json:"stock_in"`
	StockOut decimal.Decimal `json:"stock_out"`
}

// MonthlyStockMovementSummary agregasi fact_inventory_movements per bulan
// untuk satu company -- pola query SENGAJA identik dengan
// MonthlyFinanceSummary versi AWAL (sebelum MV): FINAL langsung di sini,
// BUKAN lewat Materialized View pre-agregasi. dw-service dual-write ke
// fact_inventory_movements juga (batch ETL + Kafka Streaming ETL, sama
// seperti fact_finance_journal_lines), jadi FINAL tetap WAJIB untuk
// korektnes -- tapi belum ada bukti query ini butuh percepatan MV di bawah
// beban nyata (endpoint baru, belum ada traffic sama sekali), jadi
// mengikuti pola bertahap yang sama: query dulu, MV menyusul kalau
// terbukti perlu (persis keputusan MonthlyFinanceSummary di sesi
// sebelumnya, sebelum MV-nya dibangun).
//
// fact_inventory_movements TIDAK punya kolom status (beda dari
// fact_finance_journal_lines yang punya entry_status DRAFT/POSTED) --
// setiap baris di sini merepresentasikan pergerakan stok yang SUDAH
// terjadi (PO RECEIVED, SO FULFILLED, stock transfer/opname), jadi tidak
// ada filter status yang perlu diterapkan.
func (c *Client) MonthlyStockMovementSummary(ctx context.Context, companyID uuid.UUID) ([]MonthlyStockMovementSummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			toString(toStartOfMonth(movement_date)) AS month,
			sumIf(quantity, movement_type = 'IN') AS stock_in,
			sumIf(quantity, movement_type = 'OUT') AS stock_out
		FROM fact_inventory_movements FINAL
		WHERE company_id = ?
		GROUP BY month
		ORDER BY month
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query monthly stock movement summary: %w", err)
	}
	defer rows.Close()

	out := []MonthlyStockMovementSummaryRow{}
	for rows.Next() {
		var r MonthlyStockMovementSummaryRow
		if err := rows.Scan(&r.Month, &r.StockIn, &r.StockOut); err != nil {
			return nil, fmt.Errorf("scan monthly stock movement summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MonthlySalesSummaryRow adalah satu baris hasil agregasi bulanan nilai
// sales order (bukan revenue GL -- ini order value dari fact_sales_order_lines,
// beda sumber dari MonthlyFinanceSummary yang baca fact_finance_journal_lines).
type MonthlySalesSummaryRow struct {
	Month      string          `json:"month"`
	SalesValue decimal.Decimal `json:"sales_value"`
}

// MonthlySalesSummary agregasi fact_sales_order_lines per bulan untuk satu
// company -- pola query IDENTIK dengan MonthlyStockMovementSummary: FINAL
// langsung, BUKAN Materialized View, karena endpoint baru ini belum punya
// bukti traffic yang butuh percepatan (pola bertahap yang sama, lihat
// komentar MonthlyFinanceSummary/MonthlyStockMovementSummary di atas).
// fact_sales_order_lines JUGA dual-write lewat batch ETL + Kafka Streaming
// ETL (event order.fulfilled/order.invoiced, lihat internal/streaming),
// jadi FINAL tetap wajib untuk korektnes.
//
// DRAFT dan CANCELLED SENGAJA dikecualikan -- baris DRAFT belum jadi
// komitmen penjualan sungguhan (paralel dengan filter entry_status='POSTED'
// di MonthlyFinanceSummary), dan CANCELLED sudah dibatalkan jadi bukan
// penjualan yang benar-benar terjadi. CONFIRMED/FULFILLED/INVOICED
// dihitung -- ketiganya order yang sudah committed, beda tahap fulfillment
// tapi sama-sama "sales value" yang nyata.
func (c *Client) MonthlySalesSummary(ctx context.Context, companyID uuid.UUID) ([]MonthlySalesSummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			toString(toStartOfMonth(order_date)) AS month,
			sum(amount) AS sales_value
		FROM fact_sales_order_lines FINAL
		WHERE company_id = ? AND order_status NOT IN ('DRAFT', 'CANCELLED')
		GROUP BY month
		ORDER BY month
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query monthly sales summary: %w", err)
	}
	defer rows.Close()

	out := []MonthlySalesSummaryRow{}
	for rows.Next() {
		var r MonthlySalesSummaryRow
		if err := rows.Scan(&r.Month, &r.SalesValue); err != nil {
			return nil, fmt.Errorf("scan monthly sales summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type FleetDeliveryMonthlySummaryRow struct {
	Month            string   `json:"month"`
	TotalDeliveries  uint64   `json:"total_deliveries"`
	DeliveredCount   uint64   `json:"delivered_count"`
	CancelledCount   uint64   `json:"cancelled_count"`
	AvgDeliveryHours *float64 `json:"avg_delivery_hours"`
}

// FleetDeliveryMonthlySummary meringkas surat jalan per bulan dari
// fact_fleet_delivery_orders.
//
// Bulannya diambil dari scheduled_date (kapan pengiriman DIRENCANAKAN), bukan
// created_at: itu tanggal yang dipakai orang operasional saat bicara "berapa
// pengiriman bulan ini", dan surat jalan bisa dibuat di bulan sebelumnya untuk
// jadwal bulan berikutnya.
//
// avg_delivery_hours adalah alasan sesi fact table menyimpan dispatched_at/
// delivered_at RAW alih-alih membekukan durasi di ETL: definisi "lama
// pengiriman" baru ditentukan DI SINI, yaitu jam antara berangkat dan sampai,
// dihitung HANYA untuk surat jalan yang benar-benar DELIVERED dan punya kedua
// timestamp. Nullable karena bulan yang belum punya satu pun pengiriman
// selesai memang tidak punya rata-rata -- 0 akan terbaca sebagai "sampai
// seketika", bukan "belum ada data".
//
// Selisihnya diambil dalam MENIT lalu dibagi 60, bukan `dateDiff('hour', ...)`
// langsung: dateDiff memotong ke satuan penuh, jadi pengiriman 45 menit akan
// terbaca 0 jam. Ketahuan saat verifikasi end-to-end (data demo punya
// pengiriman yang berangkat dan tiba dalam hitungan menit, dan hasilnya benar
// benar 0), bukan dari membaca dokumentasi.
func (c *Client) FleetDeliveryMonthlySummary(ctx context.Context, companyID uuid.UUID) ([]FleetDeliveryMonthlySummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			toString(toStartOfMonth(scheduled_date)) AS month,
			count() AS total_deliveries,
			countIf(status = 'DELIVERED') AS delivered_count,
			countIf(status = 'CANCELLED') AS cancelled_count,
			avgOrNullIf(
				dateDiff('minute', dispatched_at, delivered_at) / 60,
				status = 'DELIVERED' AND dispatched_at IS NOT NULL AND delivered_at IS NOT NULL
			) AS avg_delivery_hours
		FROM fact_fleet_delivery_orders FINAL
		WHERE company_id = ?
		GROUP BY month
		ORDER BY month
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query fleet delivery monthly summary: %w", err)
	}
	defer rows.Close()

	out := []FleetDeliveryMonthlySummaryRow{}
	for rows.Next() {
		var r FleetDeliveryMonthlySummaryRow
		if err := rows.Scan(&r.Month, &r.TotalDeliveries, &r.DeliveredCount, &r.CancelledCount, &r.AvgDeliveryHours); err != nil {
			return nil, fmt.Errorf("scan fleet delivery monthly summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type HRLeaveMonthlySummaryRow struct {
	Month         string `json:"month"`
	TotalRequests uint64 `json:"total_requests"`
	ApprovedCount uint64 `json:"approved_count"`
	// Int64, bukan uint64: sumIf() atas kolom Int16 di ClickHouse menghasilkan
	// Int64, dan driver-nya menolak memindahkannya ke uint64.
	AnnualDays int64 `json:"annual_days"`
	SickDays   int64 `json:"sick_days"`
	UnpaidDays int64 `json:"unpaid_days"`
	OtherDays  int64 `json:"other_days"`
}

// HRLeaveMonthlySummary meringkas pengajuan cuti per bulan dari
// fact_hr_leave_requests.
//
// Jumlah pengajuan menghitung SELURUH status (termasuk yang ditolak dan
// dibatalkan) -- itu memang beban administrasi yang nyata. Sebaliknya jumlah
// HARI hanya dihitung dari yang APPROVED: cuti yang ditolak tidak pernah
// benar-benar diambil, dan menjumlahkannya akan membuat rekap "berapa hari
// karyawan tidak masuk" jadi salah.
//
// Bulannya diambil dari start_date, bukan tanggal pengajuan: yang menarik untuk
// dianalisis adalah kapan orangnya tidak masuk, bukan kapan formulirnya diisi.
func (c *Client) HRLeaveMonthlySummary(ctx context.Context, companyID uuid.UUID) ([]HRLeaveMonthlySummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			toString(toStartOfMonth(start_date)) AS month,
			count() AS total_requests,
			countIf(status = 'APPROVED') AS approved_count,
			sumIf(total_days, status = 'APPROVED' AND leave_type = 'ANNUAL') AS annual_days,
			sumIf(total_days, status = 'APPROVED' AND leave_type = 'SICK') AS sick_days,
			sumIf(total_days, status = 'APPROVED' AND leave_type = 'UNPAID') AS unpaid_days,
			sumIf(total_days, status = 'APPROVED' AND leave_type NOT IN ('ANNUAL', 'SICK', 'UNPAID')) AS other_days
		FROM fact_hr_leave_requests FINAL
		WHERE company_id = ?
		GROUP BY month
		ORDER BY month
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query hr leave monthly summary: %w", err)
	}
	defer rows.Close()

	out := []HRLeaveMonthlySummaryRow{}
	for rows.Next() {
		var r HRLeaveMonthlySummaryRow
		if err := rows.Scan(&r.Month, &r.TotalRequests, &r.ApprovedCount,
			&r.AnnualDays, &r.SickDays, &r.UnpaidDays, &r.OtherDays); err != nil {
			return nil, fmt.Errorf("scan hr leave monthly summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type HRKPISummaryRow struct {
	Period        string `json:"period"`
	ReviewCount   uint64 `json:"review_count"`
	ApprovedCount uint64 `json:"approved_count"`
	// *float64, bukan *decimal.Decimal: avg() atas kolom Decimal menghasilkan
	// Float64 di ClickHouse -- pola yang sama dengan AvgDeliveryHours di fleet.
	AvgScore        *float64 `json:"avg_score"`
	SangatBaikCount uint64   `json:"sangat_baik_count"`
	BaikCount       uint64   `json:"baik_count"`
	CukupCount      uint64   `json:"cukup_count"`
	PerluPerbaikan  uint64   `json:"perlu_perbaikan_count"`
}

// HRKPISummary meringkas penilaian KPI per periode dari fact_hr_kpi_reviews.
//
// Rata-rata nilai DAN sebaran rating dihitung HANYA dari penilaian APPROVED.
// Penilaian yang masih DRAFT nilainya belum final (angkanya berubah tiap kali
// realisasi diisi ulang), dan yang REJECTED justru dinyatakan tidak sah oleh
// penyetujunya -- memasukkan keduanya akan menggeser rata-rata perusahaan
// dengan angka yang belum tentu pernah berlaku.
//
// AvgScore Nullable: periode yang belum punya satu pun penilaian disetujui
// memang tidak punya rata-rata. 0 akan terbaca sebagai "semua orang nol",
// bukan "belum ada yang final".
func (c *Client) HRKPISummary(ctx context.Context, companyID uuid.UUID) ([]HRKPISummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			period,
			count() AS review_count,
			countIf(status = 'APPROVED') AS approved_count,
			avgOrNullIf(total_score, status = 'APPROVED') AS avg_score,
			countIf(status = 'APPROVED' AND rating = 'SANGAT BAIK') AS sangat_baik_count,
			countIf(status = 'APPROVED' AND rating = 'BAIK') AS baik_count,
			countIf(status = 'APPROVED' AND rating = 'CUKUP') AS cukup_count,
			countIf(status = 'APPROVED' AND rating = 'PERLU PERBAIKAN') AS perlu_perbaikan_count
		FROM fact_hr_kpi_reviews FINAL
		WHERE company_id = ?
		GROUP BY period
		ORDER BY period
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query hr kpi summary: %w", err)
	}
	defer rows.Close()

	out := []HRKPISummaryRow{}
	for rows.Next() {
		var r HRKPISummaryRow
		if err := rows.Scan(&r.Period, &r.ReviewCount, &r.ApprovedCount, &r.AvgScore,
			&r.SangatBaikCount, &r.BaikCount, &r.CukupCount, &r.PerluPerbaikan); err != nil {
			return nil, fmt.Errorf("scan hr kpi summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type HRKPIDepartmentSummaryRow struct {
	Period        string `json:"period"`
	Department    string `json:"department"`
	ApprovedCount uint64 `json:"approved_count"`
	// Perhatikan bedanya: avg() atas kolom Decimal menghasilkan Float64,
	// sementara min()/max() MEMPERTAHANKAN Decimal. Jadi tiga kolom yang
	// secara konsep sama-sama "nilai" justru butuh dua tipe Go berbeda.
	AvgScore *float64         `json:"avg_score"`
	MinScore *decimal.Decimal `json:"min_score"`
	MaxScore *decimal.Decimal `json:"max_score"`
}

// LatestKPIPeriod mengembalikan periode terakhir yang punya penilaian
// APPROVED, atau "" kalau belum ada sama sekali. Dipakai supaya pemanggil
// tidak perlu menebak periode mana yang sudah final.
func (c *Client) LatestKPIPeriod(ctx context.Context, companyID uuid.UUID) (string, error) {
	var period string
	row := c.conn.QueryRow(ctx, `
		SELECT max(period)
		FROM fact_hr_kpi_reviews FINAL
		WHERE company_id = ? AND status = 'APPROVED'
	`, companyID)
	if err := row.Scan(&period); err != nil {
		return "", fmt.Errorf("query latest kpi period: %w", err)
	}
	return period, nil
}

// HRKPIDepartmentSummary membandingkan nilai KPI antar departemen pada SATU
// periode. Perbandingan lintas departemen hanya masuk akal dalam periode yang
// sama -- target dan bobot indikator bisa berbeda antar periode, jadi
// menggabungkan beberapa periode dalam satu batang akan membandingkan angka
// yang tidak sebanding.
//
// Min & max ikut dikembalikan, bukan hanya rata-rata: departemen dengan
// rata-rata 80 yang isinya 79-81 sangat berbeda dari yang isinya 60-100, dan
// perbedaan itu justru yang biasanya perlu ditindak.
//
// Karyawan tanpa departemen dikelompokkan sebagai "(tanpa departemen)" alih-alih
// dibuang: kalau dibuang, jumlah orang di grafik tidak akan cocok dengan jumlah
// penilaian di ringkasan periode yang sama.
func (c *Client) HRKPIDepartmentSummary(ctx context.Context, companyID uuid.UUID, period string) ([]HRKPIDepartmentSummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			period,
			if(department = '', '(tanpa departemen)', department) AS dept,
			count() AS approved_count,
			avg(total_score) AS avg_score,
			min(total_score) AS min_score,
			max(total_score) AS max_score
		FROM fact_hr_kpi_reviews FINAL
		WHERE company_id = ? AND status = 'APPROVED' AND period = ?
		GROUP BY period, dept
		ORDER BY avg_score DESC
	`, companyID, period)
	if err != nil {
		return nil, fmt.Errorf("query hr kpi department summary: %w", err)
	}
	defer rows.Close()

	out := []HRKPIDepartmentSummaryRow{}
	for rows.Next() {
		var r HRKPIDepartmentSummaryRow
		if err := rows.Scan(&r.Period, &r.Department, &r.ApprovedCount, &r.AvgScore, &r.MinScore, &r.MaxScore); err != nil {
			return nil, fmt.Errorf("scan hr kpi department summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type QCMonthlySummaryRow struct {
	Month          string          `json:"month"`
	InspectionsQty uint64          `json:"inspection_count"`
	PassCount      uint64          `json:"pass_count"`
	FailCount      uint64          `json:"fail_count"`
	PartialCount   uint64          `json:"partial_count"`
	InspectedQty   decimal.Decimal `json:"inspected_quantity"`
	FailedQty      decimal.Decimal `json:"failed_quantity"`
	DefectRatePct  *float64        `json:"defect_rate_pct"`
}

// QCMonthlySummary meringkas inspeksi kualitas per bulan dari
// fact_qc_inspections.
//
// Cacah hasil (PASS/FAIL/PARTIAL) DAN kuantitasnya sama-sama dikembalikan
// karena keduanya menjawab pertanyaan berbeda: berapa banyak inspeksi yang
// bermasalah, versus berapa banyak barang yang benar-benar gagal. Satu inspeksi
// PARTIAL atas 1.000 unit bukan hal yang sama dengan satu inspeksi FAIL atas 2
// unit.
//
// defect_rate_pct dihitung dari KUANTITAS (failed/inspected), bukan dari cacah
// inspeksi, dan Nullable untuk bulan yang belum menginspeksi apa pun -- 0% akan
// terbaca "tidak ada cacat", bukan "belum ada data".
func (c *Client) QCMonthlySummary(ctx context.Context, companyID uuid.UUID) ([]QCMonthlySummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			toString(toStartOfMonth(inspection_date)) AS month,
			count() AS inspection_count,
			countIf(result = 'PASS') AS pass_count,
			countIf(result = 'FAIL') AS fail_count,
			countIf(result = 'PARTIAL') AS partial_count,
			sum(inspected_quantity) AS inspected_total,
			sum(failed_quantity) AS failed_total,
			-- Alias TIDAK boleh sama dengan nama kolom: "sum(x) AS x" membuat
			-- referensi x berikutnya menunjuk ke alias (yang sudah agregat), dan
			-- ClickHouse menolaknya sebagai agregat di dalam agregat.
			if(inspected_total > 0, toFloat64(failed_total) / toFloat64(inspected_total) * 100, NULL) AS defect_rate_pct
		FROM fact_qc_inspections FINAL
		WHERE company_id = ?
		GROUP BY month
		ORDER BY month
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query qc monthly summary: %w", err)
	}
	defer rows.Close()

	out := []QCMonthlySummaryRow{}
	for rows.Next() {
		var r QCMonthlySummaryRow
		if err := rows.Scan(&r.Month, &r.InspectionsQty, &r.PassCount, &r.FailCount, &r.PartialCount,
			&r.InspectedQty, &r.FailedQty, &r.DefectRatePct); err != nil {
			return nil, fmt.Errorf("scan qc monthly summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type ProductionMonthlySummaryRow struct {
	Month           string          `json:"month"`
	WorkOrderCount  uint64          `json:"work_order_count"`
	CompletedCount  uint64          `json:"completed_count"`
	QuantityPlanned decimal.Decimal `json:"quantity_planned"`
	QuantityDone    decimal.Decimal `json:"quantity_produced"`
	AchievementPct  *float64        `json:"achievement_pct"`
}

// ProductionMonthlySummary membandingkan rencana vs realisasi produksi per bulan
// dari fact_production_work_orders.
//
// Realisasi dijumlahkan HANYA dari work order COMPLETED: quantity_produced pada
// WO yang masih berjalan belum final (dan NULL untuk yang belum mulai), jadi
// memasukkannya membuat pencapaian bulan berjalan terlihat lebih rendah dari
// yang sebenarnya. Rencana sebaliknya dihitung dari SELURUH work order bulan itu
// -- yang belum selesai tetap rencana yang sudah dijanjikan.
//
// Bulannya diambil dari planned_start_date: yang dibandingkan adalah rencana
// bulan itu, bukan kapan barangnya akhirnya selesai.
func (c *Client) ProductionMonthlySummary(ctx context.Context, companyID uuid.UUID) ([]ProductionMonthlySummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			toString(toStartOfMonth(planned_start_date)) AS month,
			count() AS work_order_count,
			countIf(status = 'COMPLETED') AS completed_count,
			sum(quantity_planned) AS planned_total,
			sumIf(ifNull(quantity_produced, toDecimal64(0, 2)), status = 'COMPLETED') AS produced_total,
			-- Alias sengaja beda dari nama kolom, alasannya sama seperti di
			-- QCMonthlySummary.
			if(planned_total > 0, toFloat64(produced_total) / toFloat64(planned_total) * 100, NULL) AS achievement_pct
		FROM fact_production_work_orders FINAL
		WHERE company_id = ?
		GROUP BY month
		ORDER BY month
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query production monthly summary: %w", err)
	}
	defer rows.Close()

	out := []ProductionMonthlySummaryRow{}
	for rows.Next() {
		var r ProductionMonthlySummaryRow
		if err := rows.Scan(&r.Month, &r.WorkOrderCount, &r.CompletedCount,
			&r.QuantityPlanned, &r.QuantityDone, &r.AchievementPct); err != nil {
			return nil, fmt.Errorf("scan production monthly summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type PurchasingSupplierSummaryRow struct {
	SupplierCode string          `json:"supplier_code"`
	SupplierName string          `json:"supplier_name"`
	OrderCount   uint64          `json:"order_count"`
	LineCount    uint64          `json:"line_count"`
	TotalSpend   decimal.Decimal `json:"total_spend"`
}

// PurchasingSupplierSummary meringkas belanja per supplier dari
// fact_purchasing_order_lines.
//
// Yang dihitung hanya PO berstatus RECEIVED atau INVOICED -- itu belanja yang
// benar-benar terjadi. PO yang masih DRAFT/CONFIRMED baru rencana, dan
// mencampurnya membuat "belanja ke supplier ini" jadi angka yang tidak bisa
// dipakai untuk negosiasi.
//
// order_count memakai uniqExact atas purchase_order_id, bukan count() baris:
// satu PO dengan 10 baris tetap SATU order.
func (c *Client) PurchasingSupplierSummary(ctx context.Context, companyID uuid.UUID) ([]PurchasingSupplierSummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			supplier_code,
			any(supplier_name) AS supplier_name,
			uniqExact(purchase_order_id) AS order_count,
			count() AS line_count,
			sum(amount) AS total_spend
		FROM fact_purchasing_order_lines FINAL
		WHERE company_id = ? AND order_status IN ('RECEIVED', 'INVOICED')
		GROUP BY supplier_code
		ORDER BY total_spend DESC
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query purchasing supplier summary: %w", err)
	}
	defer rows.Close()

	out := []PurchasingSupplierSummaryRow{}
	for rows.Next() {
		var r PurchasingSupplierSummaryRow
		if err := rows.Scan(&r.SupplierCode, &r.SupplierName, &r.OrderCount, &r.LineCount, &r.TotalSpend); err != nil {
			return nil, fmt.Errorf("scan purchasing supplier summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type TicketingMonthlySummaryRow struct {
	Month           string   `json:"month"`
	TicketCount     uint64   `json:"ticket_count"`
	ResolvedCount   uint64   `json:"resolved_count"`
	OpenCount       uint64   `json:"open_count"`
	UrgentCount     uint64   `json:"urgent_count"`
	AvgResolveHours *float64 `json:"avg_resolve_hours"`
}

// TicketingMonthlySummary meringkas tiket per bulan dari fact_ticketing_tickets.
//
// Lama penyelesaian dihitung dari created_at ke resolved_at dalam MENIT lalu
// dibagi 60 -- bukan dateDiff('hour', ...) yang memotong ke jam penuh sehingga
// tiket yang selesai 45 menit terbaca 0 jam (jebakan yang sama pernah ketahuan
// di fleet, lihat FleetDeliveryMonthlySummary).
//
// open_count memakai definisi "belum punya resolved_at", bukan daftar status
// tertentu: status bisa bertambah kelak, tapi "belum selesai" akan tetap berarti
// belum ada waktu penyelesaiannya.
func (c *Client) TicketingMonthlySummary(ctx context.Context, companyID uuid.UUID) ([]TicketingMonthlySummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			toString(toStartOfMonth(created_at)) AS month,
			count() AS ticket_count,
			countIf(resolved_at IS NOT NULL) AS resolved_count,
			countIf(resolved_at IS NULL) AS open_count,
			countIf(priority = 'URGENT') AS urgent_count,
			avgOrNullIf(dateDiff('minute', created_at, resolved_at) / 60, resolved_at IS NOT NULL) AS avg_resolve_hours
		FROM fact_ticketing_tickets FINAL
		WHERE company_id = ?
		GROUP BY month
		ORDER BY month
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query ticketing monthly summary: %w", err)
	}
	defer rows.Close()

	out := []TicketingMonthlySummaryRow{}
	for rows.Next() {
		var r TicketingMonthlySummaryRow
		if err := rows.Scan(&r.Month, &r.TicketCount, &r.ResolvedCount, &r.OpenCount,
			&r.UrgentCount, &r.AvgResolveHours); err != nil {
			return nil, fmt.Errorf("scan ticketing monthly summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type PayrollPeriodSummaryRow struct {
	Period        string          `json:"period"`
	EmployeeCount uint64          `json:"employee_count"`
	TotalGross    decimal.Decimal `json:"total_gross"`
	TotalDeducted decimal.Decimal `json:"total_deduction"`
	TotalNet      decimal.Decimal `json:"total_net"`
	AvgNet        *float64        `json:"avg_net"`
	AttendancePct *float64        `json:"attendance_pct"`
}

// PayrollPeriodSummary meringkas payroll per periode dari
// fact_hr_payroll_details.
//
// Hanya payroll run berstatus POSTED yang dihitung: itu satu-satunya status
// yang angkanya sudah masuk jurnal GL dan tidak akan berubah lagi. Run yang
// masih DRAFT bisa dihapus dan diproses ulang, jadi memasukkannya membuat
// "biaya gaji bulan ini" berubah-ubah setiap kali HR mencoba ulang.
//
// attendance_pct = hari hadir / hari kerja seluruh karyawan pada periode itu,
// bukan rata-rata dari persentase per orang: karyawan yang baru masuk di tengah
// bulan tidak boleh menarik turun angka perusahaan sebanyak karyawan penuh.
func (c *Client) PayrollPeriodSummary(ctx context.Context, companyID uuid.UUID) ([]PayrollPeriodSummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			period,
			count() AS employee_count,
			sum(gross_salary) AS gross_total,
			sum(total_deduction) AS deduction_total,
			sum(net_salary) AS net_total,
			avg(net_salary) AS avg_net,
			if(sum(working_days) > 0, toFloat64(sum(present_days)) / toFloat64(sum(working_days)) * 100, NULL) AS attendance_pct
		FROM fact_hr_payroll_details FINAL
		WHERE company_id = ? AND run_status = 'POSTED'
		GROUP BY period
		ORDER BY period
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query payroll period summary: %w", err)
	}
	defer rows.Close()

	out := []PayrollPeriodSummaryRow{}
	for rows.Next() {
		var r PayrollPeriodSummaryRow
		if err := rows.Scan(&r.Period, &r.EmployeeCount, &r.TotalGross, &r.TotalDeducted,
			&r.TotalNet, &r.AvgNet, &r.AttendancePct); err != nil {
			return nil, fmt.Errorf("scan payroll period summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type AssetMaintenanceSummaryRow struct {
	Month          string   `json:"month"`
	ScheduledCount uint64   `json:"scheduled_count"`
	CompletedCount uint64   `json:"completed_count"`
	CancelledCount uint64   `json:"cancelled_count"`
	OverdueCount   uint64   `json:"overdue_count"`
	AvgDelayDays   *float64 `json:"avg_delay_days"`
}

// AssetMaintenanceSummary meringkas jadwal perawatan aset per bulan dari
// fact_asset_maintenance.
//
// overdue_count memakai definisi "sudah lewat tanggalnya dan belum selesai atau
// dibatalkan" -- dihitung SAAT QUERY (today()), bukan dibekukan di ETL. Kalau
// dibekukan, angka "terlambat" akan berhenti bertambah setiap kali sync tidak
// jalan, padahal keterlambatan justru bertambah dengan sendirinya.
//
// avg_delay_days hanya dari yang sudah selesai: selisih tanggal selesai dengan
// tanggal jadwal, boleh negatif kalau dikerjakan lebih awal. Nullable untuk
// bulan yang belum punya satu pun perawatan selesai.
func (c *Client) AssetMaintenanceSummary(ctx context.Context, companyID uuid.UUID) ([]AssetMaintenanceSummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			toString(toStartOfMonth(scheduled_date)) AS month,
			count() AS scheduled_count,
			countIf(status = 'COMPLETED') AS completed_count,
			countIf(status = 'CANCELLED') AS cancelled_count,
			countIf(status NOT IN ('COMPLETED', 'CANCELLED') AND scheduled_date < today()) AS overdue_count,
			avgOrNullIf(toFloat64(dateDiff('day', scheduled_date, completed_date)), completed_date IS NOT NULL) AS avg_delay_days
		FROM fact_asset_maintenance FINAL
		WHERE company_id = ?
		GROUP BY month
		ORDER BY month
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query asset maintenance summary: %w", err)
	}
	defer rows.Close()

	out := []AssetMaintenanceSummaryRow{}
	for rows.Next() {
		var r AssetMaintenanceSummaryRow
		if err := rows.Scan(&r.Month, &r.ScheduledCount, &r.CompletedCount, &r.CancelledCount,
			&r.OverdueCount, &r.AvgDelayDays); err != nil {
			return nil, fmt.Errorf("scan asset maintenance summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type IoTDeviceSummaryRow struct {
	DeviceCode   string `json:"device_code"`
	DeviceType   string `json:"device_type"`
	ReadingType  string `json:"reading_type"`
	ReadingCount uint64 `json:"reading_count"`
	// Lagi-lagi beda tipe untuk tiga kolom yang konsepnya sama: avg() atas
	// Decimal menghasilkan Float64, min()/max() mempertahankan Decimal (jebakan
	// yang sama sudah tercatat di HRKPIDepartmentSummaryRow).
	AvgValue   *float64         `json:"avg_value"`
	MinValue   *decimal.Decimal `json:"min_value"`
	MaxValue   *decimal.Decimal `json:"max_value"`
	LastReadAt string           `json:"last_read_at"`
}

// IoTDeviceSummary meringkas pembacaan sensor per device DAN per jenis
// pembacaan dari fact_iot_readings.
//
// Dikelompokkan per (device, reading_type), bukan per device saja: satu device
// bisa mengirim suhu dan kelembapan sekaligus, dan merata-ratakan keduanya
// menghasilkan angka yang tidak berarti apa-apa.
//
// Hanya pembacaan numerik yang diringkas (value_numeric IS NOT NULL) --
// pembacaan berbentuk teks (mis. status ON/OFF) tidak punya rata-rata.
// last_read_at ikut dikembalikan karena pertanyaan pertama tentang sebuah
// sensor biasanya "masih hidup atau tidak", bukan berapa nilainya.
func (c *Client) IoTDeviceSummary(ctx context.Context, companyID uuid.UUID) ([]IoTDeviceSummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			device_code,
			any(device_type) AS device_type,
			reading_type,
			count() AS reading_count,
			avg(value_numeric) AS avg_value,
			min(value_numeric) AS min_value,
			max(value_numeric) AS max_value,
			toString(max(recorded_at)) AS last_read_at
		FROM fact_iot_readings FINAL
		WHERE company_id = ? AND value_numeric IS NOT NULL
		GROUP BY device_code, reading_type
		ORDER BY device_code, reading_type
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query iot device summary: %w", err)
	}
	defer rows.Close()

	out := []IoTDeviceSummaryRow{}
	for rows.Next() {
		var r IoTDeviceSummaryRow
		if err := rows.Scan(&r.DeviceCode, &r.DeviceType, &r.ReadingType, &r.ReadingCount,
			&r.AvgValue, &r.MinValue, &r.MaxValue, &r.LastReadAt); err != nil {
			return nil, fmt.Errorf("scan iot device summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type EcommerceMonthlySummaryRow struct {
	Month        string          `json:"month"`
	OrderCount   uint64          `json:"order_count"`
	LineCount    uint64          `json:"line_count"`
	ItemsSold    decimal.Decimal `json:"items_sold"`
	Revenue      decimal.Decimal `json:"revenue"`
	AvgOrderSize *float64        `json:"avg_order_value"`
}

// EcommerceMonthlySummary meringkas penjualan online per bulan dari
// fact_ecommerce_order_lines.
//
// Order yang CANCELLED dibuang: barangnya tidak pernah dikirim dan uangnya
// tidak pernah masuk. Yang masih PENDING tetap dihitung -- itu penjualan yang
// sedang berjalan, dan membuangnya membuat bulan berjalan selalu terlihat sepi.
//
// avg_order_value dihitung per ORDER (revenue / jumlah order unik), bukan per
// baris: rata-rata per baris hanya menjawab "berapa harga satu jenis barang",
// bukan "berapa besar satu keranjang belanja".
func (c *Client) EcommerceMonthlySummary(ctx context.Context, companyID uuid.UUID) ([]EcommerceMonthlySummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			toString(toStartOfMonth(order_date)) AS month,
			uniqExact(order_id) AS order_count,
			count() AS line_count,
			sum(quantity) AS items_sold,
			sum(amount) AS revenue_total,
			if(uniqExact(order_id) > 0, toFloat64(sum(amount)) / uniqExact(order_id), NULL) AS avg_order_value
		FROM fact_ecommerce_order_lines FINAL
		WHERE company_id = ? AND order_status <> 'CANCELLED'
		GROUP BY month
		ORDER BY month
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query ecommerce monthly summary: %w", err)
	}
	defer rows.Close()

	out := []EcommerceMonthlySummaryRow{}
	for rows.Next() {
		var r EcommerceMonthlySummaryRow
		if err := rows.Scan(&r.Month, &r.OrderCount, &r.LineCount, &r.ItemsSold,
			&r.Revenue, &r.AvgOrderSize); err != nil {
			return nil, fmt.Errorf("scan ecommerce monthly summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type ProjectCostSummaryRow struct {
	ProjectCode    string          `json:"project_code"`
	ProjectName    string          `json:"project_name"`
	ProjectStatus  string          `json:"project_status"`
	TimesheetCount uint64          `json:"timesheet_count"`
	PostedHours    decimal.Decimal `json:"posted_hours"`
	PostedAmount   decimal.Decimal `json:"posted_amount"`
}

// ProjectCostSummary meringkas biaya tenaga kerja per proyek dari
// fact_project_timesheets.
//
// Hanya timesheet berstatus POSTED yang dihitung -- itu satu-satunya status
// yang biayanya benar-benar sudah masuk jurnal finance-service, jadi angka di
// sini bisa direkonsiliasi dengan GL. DRAFT/APPROVED sengaja DIKECUALIKAN
// (biaya yang belum diakui di pembukuan) dan REJECTED jelas tidak dihitung.
// Prinsip yang sama seperti entry_status='POSTED' di MonthlyFinanceSummary dan
// pengecualian DRAFT/CANCELLED di MonthlySalesSummary.
//
// FINAL dipakai karena dw-service dual-write ke fact table lewat batch ETL DAN
// Kafka Streaming ETL: baris timesheet yang sama bisa ter-INSERT dua kali
// dengan synced_at berbeda, dan tanpa FINAL keduanya akan ikut terjumlah.
func (c *Client) ProjectCostSummary(ctx context.Context, companyID uuid.UUID) ([]ProjectCostSummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			project_code,
			any(project_name) AS project_name,
			any(project_status) AS project_status,
			count() AS timesheet_count,
			sum(hours) AS posted_hours,
			sum(amount) AS posted_amount
		FROM fact_project_timesheets FINAL
		WHERE company_id = ? AND status = 'POSTED'
		GROUP BY project_code
		ORDER BY posted_amount DESC
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("query project cost summary: %w", err)
	}
	defer rows.Close()

	out := []ProjectCostSummaryRow{}
	for rows.Next() {
		var r ProjectCostSummaryRow
		if err := rows.Scan(&r.ProjectCode, &r.ProjectName, &r.ProjectStatus, &r.TimesheetCount, &r.PostedHours, &r.PostedAmount); err != nil {
			return nil, fmt.Errorf("scan project cost summary row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type CRMPipelineSummaryRow struct {
	Stage            string          `json:"stage"`
	OpportunityCount uint64          `json:"opportunity_count"`
	TotalAmount      decimal.Decimal `json:"total_amount"`
	WeightedAmount   decimal.Decimal `json:"weighted_amount"`
}

// crmPipelineStageOrder adalah urutan stage sepanjang pipeline sales, dipakai
// untuk ORDER BY. TIDAK bisa mengurutkan by nama stage -- alfabetis akan
// menghasilkan LOST/NEGOTIATION/PROPOSAL/PROSPECTING/QUALIFICATION/WON, urutan
// yang tidak berarti apa-apa untuk pembaca funnel. Daftar ini cocok dengan
// CHECK constraint stage di crm-service/migrations/001_init.sql.
var crmPipelineStageOrder = []string{"PROSPECTING", "QUALIFICATION", "PROPOSAL", "NEGOTIATION", "WON", "LOST"}

// CRMPipelineSummary agregasi fact_crm_opportunities per stage untuk satu
// company -- query analitik keempat di dw-service, dan yang PERTAMA yang
// bukan time series bulanan (tiga sebelumnya semua per bulan). Grain fact-nya
// satu baris per opportunity, jadi count() setelah FINAL memang jumlah
// opportunity yang sebenarnya, bukan jumlah baris detail.
//
// Pola staged yang SAMA dengan tiga endpoint analitik sebelumnya: FINAL
// langsung, TANPA Materialized View, karena endpoint baru ini belum punya
// bukti traffic yang butuh percepatan. FINAL sendiri tetap WAJIB untuk
// korektnes -- fact_crm_opportunities dual-write lewat batch ETL DAN Kafka
// Streaming ETL (event crm.opportunity.won/lost, lihat internal/streaming),
// jadi satu opportunity bisa punya beberapa baris yang belum ter-merge
// background; tanpa FINAL, satu deal yang di-sync dua kali akan terhitung
// dua kali baik di count maupun di sum.
//
// SEMUA stage dikembalikan, termasuk WON dan LOST -- keputusan sengaja: yang
// menarik dari data ini justru perbandingan nilai yang masih terbuka vs yang
// sudah menutup, dan menyembunyikan LOST akan membuat total di chart terbaca
// seolah-olah semua deal masih hidup. Stage tanpa satupun opportunity tidak
// muncul sebagai baris nol (konsisten dengan tiga endpoint lain yang juga
// tidak memunculkan bulan kosong).
//
// weighted_amount = amount * probability/100, nilai pipeline yang sudah
// dibobot peluang menang -- angka yang lazim dipakai forecasting sales, dan
// alasan kolom `probability` ikut dimasukkan ke fact table sejak awal.
func (c *Client) CRMPipelineSummary(ctx context.Context, companyID uuid.UUID) ([]CRMPipelineSummaryRow, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT
			stage,
			count() AS opportunity_count,
			sum(amount) AS total_amount,
			sum(amount * probability / 100) AS weighted_amount
		FROM fact_crm_opportunities FINAL
		WHERE company_id = ?
		GROUP BY stage
		ORDER BY indexOf(?, stage)
	`, companyID, crmPipelineStageOrder)
	if err != nil {
		return nil, fmt.Errorf("query crm pipeline summary: %w", err)
	}
	defer rows.Close()

	out := []CRMPipelineSummaryRow{}
	for rows.Next() {
		var r CRMPipelineSummaryRow
		if err := rows.Scan(&r.Stage, &r.OpportunityCount, &r.TotalAmount, &r.WeightedAmount); err != nil {
			return nil, fmt.Errorf("scan crm pipeline summary row: %w", err)
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

// HRLeaveRow adalah satu pengajuan cuti di fact_hr_leave_requests. Seluruh
// status ikut disalin (bukan hanya APPROVED) supaya analitik bisa membedakan
// "diajukan" dari "disetujui" -- penyaringannya dilakukan di query ringkasan,
// bukan dengan membuang data di ETL.
type HRLeaveRow struct {
	LeaveID      uuid.UUID
	CompanyID    uuid.UUID
	BranchID     *uuid.UUID
	EmployeeID   uuid.UUID
	EmployeeCode string
	EmployeeName string
	Department   string
	LeaveType    string
	Status       string
	StartDate    time.Time
	EndDate      time.Time
	TotalDays    int16
	DecidedAt    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (c *Client) InsertHRLeaveRequests(ctx context.Context, rows []HRLeaveRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_hr_leave_requests")
	if err != nil {
		return fmt.Errorf("prepare hr leave batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.LeaveID, r.CompanyID, r.BranchID, r.EmployeeID, r.EmployeeCode, r.EmployeeName,
			r.Department, r.LeaveType, r.Status, r.StartDate, r.EndDate, r.TotalDays,
			r.DecidedAt, r.CreatedAt, r.UpdatedAt, syncedAt,
		); err != nil {
			return fmt.Errorf("append hr leave row %s: %w", r.LeaveID, err)
		}
	}
	return batch.Send()
}

// HRKPIReviewRow adalah satu penilaian KPI di fact_hr_kpi_reviews. Yang disalin
// hanya kepala penilaiannya (nilai total & rating), bukan rincian per
// indikator: bobot dan target indikator berbeda antar periode, jadi rincian
// tidak bisa dibandingkan lintas periode tanpa konteksnya sendiri.
type HRKPIReviewRow struct {
	ReviewID     uuid.UUID
	CompanyID    uuid.UUID
	BranchID     *uuid.UUID
	EmployeeID   uuid.UUID
	EmployeeCode string
	EmployeeName string
	Department   string
	Period       string
	Status       string
	TotalScore   float64
	Rating       string
	DecidedAt    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (c *Client) InsertHRKPIReviews(ctx context.Context, rows []HRKPIReviewRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_hr_kpi_reviews")
	if err != nil {
		return fmt.Errorf("prepare hr kpi batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.ReviewID, r.CompanyID, r.BranchID, r.EmployeeID, r.EmployeeCode, r.EmployeeName,
			r.Department, r.Period, r.Status, toDecimal(r.TotalScore), r.Rating,
			r.DecidedAt, r.CreatedAt, r.UpdatedAt, syncedAt,
		); err != nil {
			return fmt.Errorf("append hr kpi row %s: %w", r.ReviewID, err)
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

type CRMOpportunityRow struct {
	OpportunityID     uuid.UUID
	CompanyID         uuid.UUID
	BranchID          *uuid.UUID
	OpportunityNumber string
	AccountID         uuid.UUID
	AccountName       string
	ContactID         *uuid.UUID
	OpportunityName   string
	Stage             string
	Amount            float64
	Probability       int32
	ExpectedCloseDate *time.Time
	OwnerUserID       *uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (c *Client) InsertCRMOpportunities(ctx context.Context, rows []CRMOpportunityRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_crm_opportunities")
	if err != nil {
		return fmt.Errorf("prepare crm batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.OpportunityID, r.CompanyID, r.BranchID, r.OpportunityNumber, r.AccountID, r.AccountName,
			r.ContactID, r.OpportunityName, r.Stage, toDecimal(r.Amount), r.Probability,
			r.ExpectedCloseDate, r.OwnerUserID, r.CreatedAt, r.UpdatedAt, syncedAt,
		); err != nil {
			return fmt.Errorf("append crm row %s: %w", r.OpportunityID, err)
		}
	}
	return batch.Send()
}

type TicketingTicketRow struct {
	TicketID       uuid.UUID
	CompanyID      uuid.UUID
	BranchID       *uuid.UUID
	TicketNumber   string
	CategoryID     uuid.UUID
	CategoryName   string
	Subject        string
	Priority       string
	Status         string
	RequesterName  string
	RequesterEmail *string
	AssignedTo     *uuid.UUID
	CreatedAt      time.Time
	ResolvedAt     *time.Time
	ClosedAt       *time.Time
	UpdatedAt      time.Time
}

func (c *Client) InsertTicketingTickets(ctx context.Context, rows []TicketingTicketRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_ticketing_tickets")
	if err != nil {
		return fmt.Errorf("prepare ticketing batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.TicketID, r.CompanyID, r.BranchID, r.TicketNumber, r.CategoryID, r.CategoryName,
			r.Subject, r.Priority, r.Status, r.RequesterName, r.RequesterEmail, r.AssignedTo,
			r.CreatedAt, r.ResolvedAt, r.ClosedAt, r.UpdatedAt, syncedAt,
		); err != nil {
			return fmt.Errorf("append ticketing row %s: %w", r.TicketID, err)
		}
	}
	return batch.Send()
}

// FleetDeliveryOrderRow -- grain satu baris per surat jalan. Kolom kendaraan/
// pengemudi sudah terdenormalisasi dari JOIN di internal/etl/fleet.go.
type FleetDeliveryOrderRow struct {
	DeliveryID       uuid.UUID
	CompanyID        uuid.UUID
	BranchID         *uuid.UUID
	DeliveryNumber   string
	VehicleID        uuid.UUID
	VehicleCode      string
	VehicleType      string
	DriverID         uuid.UUID
	DriverCode       string
	DriverName       string
	EcommerceOrderID *uuid.UUID
	ReferenceNumber  *string
	RecipientName    string
	ScheduledDate    time.Time
	Status           string
	DispatchedAt     *time.Time
	DeliveredAt      *time.Time
	CancelledAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (c *Client) InsertFleetDeliveryOrders(ctx context.Context, rows []FleetDeliveryOrderRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_fleet_delivery_orders")
	if err != nil {
		return fmt.Errorf("prepare fleet batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.DeliveryID, r.CompanyID, r.BranchID, r.DeliveryNumber,
			r.VehicleID, r.VehicleCode, r.VehicleType,
			r.DriverID, r.DriverCode, r.DriverName,
			r.EcommerceOrderID, r.ReferenceNumber, r.RecipientName,
			r.ScheduledDate, r.Status, r.DispatchedAt, r.DeliveredAt, r.CancelledAt,
			r.CreatedAt, r.UpdatedAt, syncedAt,
		); err != nil {
			return fmt.Errorf("append fleet row %s: %w", r.DeliveryID, err)
		}
	}
	return batch.Send()
}

// ProjectTimesheetRow -- grain satu baris per timesheet. Hours/HourlyRate/
// Amount float64 di sini (hasil scan pgx dari NUMERIC) dan dikonversi lewat
// toDecimal saat append, sama seperti fact ecommerce/sales.
type ProjectTimesheetRow struct {
	TimesheetID    uuid.UUID
	CompanyID      uuid.UUID
	BranchID       *uuid.UUID
	ProjectID      uuid.UUID
	ProjectCode    string
	ProjectName    string
	ProjectStatus  string
	TaskID         *uuid.UUID
	TaskNumber     *string
	EmployeeID     uuid.UUID
	EmployeeName   string
	WorkDate       time.Time
	Hours          float64
	HourlyRate     float64
	Amount         float64
	Status         string
	ApprovedAt     *time.Time
	PostedAt       *time.Time
	JournalEntryID *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (c *Client) InsertProjectTimesheets(ctx context.Context, rows []ProjectTimesheetRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_project_timesheets")
	if err != nil {
		return fmt.Errorf("prepare project batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.TimesheetID, r.CompanyID, r.BranchID,
			r.ProjectID, r.ProjectCode, r.ProjectName, r.ProjectStatus,
			r.TaskID, r.TaskNumber,
			r.EmployeeID, r.EmployeeName, r.WorkDate,
			toDecimal(r.Hours), toDecimal(r.HourlyRate), toDecimal(r.Amount),
			r.Status, r.ApprovedAt, r.PostedAt, r.JournalEntryID,
			r.CreatedAt, r.UpdatedAt, syncedAt,
		); err != nil {
			return fmt.Errorf("append project row %s: %w", r.TimesheetID, err)
		}
	}
	return batch.Send()
}

type EcommerceOrderLineRow struct {
	LineID        uuid.UUID
	OrderID       uuid.UUID
	CompanyID     uuid.UUID
	BranchID      *uuid.UUID
	OrderNumber   string
	OrderDate     time.Time
	OrderStatus   string
	CustomerName  string
	CustomerEmail *string
	ProductID     uuid.UUID
	ProductSKU    string
	ProductName   string
	Quantity      float64
	UnitPrice     float64
	Amount        float64
	UpdatedAt     time.Time
}

func (c *Client) InsertEcommerceOrderLines(ctx context.Context, rows []EcommerceOrderLineRow, syncedAt time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO fact_ecommerce_order_lines")
	if err != nil {
		return fmt.Errorf("prepare ecommerce batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.LineID, r.OrderID, r.CompanyID, r.BranchID, r.OrderNumber, r.OrderDate, r.OrderStatus,
			r.CustomerName, r.CustomerEmail, r.ProductID, r.ProductSKU, r.ProductName,
			toDecimal(r.Quantity), toDecimal(r.UnitPrice), toDecimal(r.Amount), r.UpdatedAt, syncedAt,
		); err != nil {
			return fmt.Errorf("append ecommerce row %s: %w", r.LineID, err)
		}
	}
	return batch.Send()
}
