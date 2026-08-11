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
