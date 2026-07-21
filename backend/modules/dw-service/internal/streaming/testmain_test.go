package streaming

// Test package streaming menggunakan database Postgres TERPISAH
// ("dw_streaming_test", bukan "dw_service_test" yang dipakai internal/etl)
// untuk menghindari DDL race saat `go test ./...` menjalankan kedua package
// secara paralel — CREATE TABLE/CREATE EXTENSION IS NOT EXISTS masih bisa
// race dari 2 proses berbeda ke database yang sama (pelajaran Known Issues
// #13 dari sesi IoT, berlaku di sini juga).
//
// Semua test di package ini memanggil handler langsung (tidak via Kafka)
// mengikuti pola internal/etl: seed Postgres → panggil handler → verifikasi
// baris di ClickHouse. Tidak butuh Kafka untuk test ini.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
	"github.com/enterprise-digital-platform/dw-service/internal/sourcedb"
)

var (
	pool     *pgxpool.Pool   // koneksi ke dw_streaming_test
	pools    *sourcedb.Pools // wrapper sourcedb, semua pools menunjuk ke pool yang sama
	chClient *ch.Client
)

const (
	adminDatabaseURL    = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	streamingTestDBURL  = "postgres://platform:platform@localhost:5432/dw_streaming_test?sslmode=disable"
	streamingTestDBName = "dw_streaming_test"
)

// streamingSourceSchema mendefinisikan tabel minimal yang dipakai oleh
// handler SQL di handlers.go. Skema ini meniru tabel asli di masing-masing
// service, hanya kolom yang benar-benar di-SELECT oleh handler (atau wajib
// ada karena FK/NOT NULL/DEFAULT). Sama persis polanya dengan sourceSchema
// di internal/etl/testmain_test.go tapi hanya untuk 8 domain (tidak IoT).
const streamingSourceSchema = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Finance
CREATE TABLE IF NOT EXISTS accounts (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	account_code VARCHAR(20) NOT NULL,
	account_name VARCHAR(200) NOT NULL,
	account_type VARCHAR(20) NOT NULL
);
CREATE TABLE IF NOT EXISTS journal_entries (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	entry_number VARCHAR(30) NOT NULL,
	entry_date DATE NOT NULL,
	period VARCHAR(7) NOT NULL,
	reference_type VARCHAR(30) NOT NULL DEFAULT 'manual',
	status VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
	posted_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS journal_lines (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	journal_id UUID NOT NULL REFERENCES journal_entries(id),
	account_id UUID NOT NULL REFERENCES accounts(id),
	debit_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
	credit_amount NUMERIC(18,2) NOT NULL DEFAULT 0
);

-- Sales
CREATE TABLE IF NOT EXISTS customers (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	customer_code VARCHAR(20) NOT NULL,
	name VARCHAR(200) NOT NULL
);
CREATE TABLE IF NOT EXISTS sales_orders (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	so_number VARCHAR(30) NOT NULL,
	order_date DATE NOT NULL,
	status VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
	customer_id UUID NOT NULL REFERENCES customers(id),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS sales_order_lines (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	sales_order_id UUID NOT NULL REFERENCES sales_orders(id),
	product_name VARCHAR(200) NOT NULL,
	quantity NUMERIC(12,2) NOT NULL,
	unit_price NUMERIC(15,2) NOT NULL,
	amount NUMERIC(15,2) NOT NULL
);

-- Warehouse (Inventory)
CREATE TABLE IF NOT EXISTS warehouses (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	code VARCHAR(20) NOT NULL,
	name VARCHAR(200) NOT NULL
);
CREATE TABLE IF NOT EXISTS products (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	sku VARCHAR(50) NOT NULL,
	name VARCHAR(200) NOT NULL
);
CREATE TABLE IF NOT EXISTS stock_movements (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	warehouse_id UUID NOT NULL REFERENCES warehouses(id),
	product_id UUID NOT NULL REFERENCES products(id),
	movement_type VARCHAR(10) NOT NULL,
	quantity NUMERIC(15,2) NOT NULL,
	reference_type VARCHAR(30) NOT NULL,
	reference_id UUID,
	movement_date DATE NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- HR
CREATE TABLE IF NOT EXISTS employees (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	employee_code VARCHAR(20) NOT NULL,
	department VARCHAR(100)
);
CREATE TABLE IF NOT EXISTS payroll_runs (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	period VARCHAR(7) NOT NULL,
	status VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
	posted_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS payroll_details (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	payroll_run_id UUID NOT NULL REFERENCES payroll_runs(id),
	employee_id UUID NOT NULL REFERENCES employees(id),
	employee_name VARCHAR(200) NOT NULL,
	basic_salary NUMERIC(15,2) NOT NULL,
	gross_salary NUMERIC(15,2) NOT NULL,
	total_deduction NUMERIC(15,2) NOT NULL,
	net_salary NUMERIC(15,2) NOT NULL,
	working_days SMALLINT NOT NULL,
	present_days SMALLINT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Purchasing
CREATE TABLE IF NOT EXISTS suppliers (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	supplier_code VARCHAR(20) NOT NULL,
	name VARCHAR(200) NOT NULL
);
CREATE TABLE IF NOT EXISTS purchase_orders (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	po_number VARCHAR(30) NOT NULL,
	order_date DATE NOT NULL,
	status VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
	supplier_id UUID NOT NULL REFERENCES suppliers(id),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS purchase_order_lines (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	purchase_order_id UUID NOT NULL REFERENCES purchase_orders(id),
	product_name VARCHAR(200) NOT NULL,
	quantity NUMERIC(12,2) NOT NULL,
	unit_price NUMERIC(15,2) NOT NULL,
	amount NUMERIC(15,2) NOT NULL
);

-- Production
CREATE TABLE IF NOT EXISTS work_orders (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	wo_number VARCHAR(30) NOT NULL,
	bom_id UUID NOT NULL,
	product_id UUID NOT NULL,
	warehouse_id UUID NOT NULL,
	quantity_planned NUMERIC(15,2) NOT NULL,
	quantity_produced NUMERIC(15,2),
	status VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
	planned_start_date DATE NOT NULL,
	planned_end_date DATE,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- QC
CREATE TABLE IF NOT EXISTS quality_standards (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	standard_code VARCHAR(20) NOT NULL,
	product_id UUID NOT NULL
);
CREATE TABLE IF NOT EXISTS quality_inspections (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	inspection_number VARCHAR(30) NOT NULL,
	standard_id UUID NOT NULL REFERENCES quality_standards(id),
	product_id UUID NOT NULL,
	reference_type VARCHAR(30) NOT NULL,
	reference_id UUID,
	reference_number VARCHAR(30),
	inspected_quantity NUMERIC(15,2) NOT NULL,
	passed_quantity NUMERIC(15,2) NOT NULL,
	failed_quantity NUMERIC(15,2) NOT NULL,
	result VARCHAR(10) NOT NULL,
	inspection_date DATE NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Asset
CREATE TABLE IF NOT EXISTS assets (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	asset_code VARCHAR(20) NOT NULL,
	name VARCHAR(200) NOT NULL
);
CREATE TABLE IF NOT EXISTS maintenance_schedules (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	asset_id UUID NOT NULL REFERENCES assets(id),
	maintenance_type VARCHAR(30) NOT NULL,
	scheduled_date DATE NOT NULL,
	completed_date DATE,
	status VARCHAR(20) NOT NULL DEFAULT 'SCHEDULED',
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

func TestMain(m *testing.M) {
	ctx := context.Background()

	// Connect to admin DB to create test database.
	adminURL := getEnvStr("DW_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: streaming tests need local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: streaming tests need local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE "+streamingTestDBName); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create %s database: %v\n", streamingTestDBName, err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	// Connect to test DB and apply schema.
	testURL := getEnvStr("DW_STREAMING_TEST_DATABASE_URL", streamingTestDBURL)
	pool, err = pgxpool.New(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to %s: %v\n", streamingTestDBName, err)
		os.Exit(0)
	}
	if _, err := pool.Exec(ctx, streamingSourceSchema); err != nil {
		fmt.Printf("FAIL: could not set up %s schema: %v\n", streamingTestDBName, err)
		pool.Close()
		os.Exit(1)
	}

	// Semua sourcedb.Pools menunjuk ke database test yang sama (schema sudah
	// mencakup tabel dari semua domain). Tidak perlu 8 database terpisah untuk
	// test karena tidak ada konflik nama tabel antar domain.
	pools = &sourcedb.Pools{
		Finance:    pool,
		Sales:      pool,
		Warehouse:  pool,
		HR:         pool,
		Purchasing: pool,
		Production: pool,
		QC:         pool,
		Asset:      pool,
	}

	// Connect ClickHouse.
	chAddr := getEnvStr("DW_TEST_CLICKHOUSE_ADDR", "localhost:9101")
	chUser := getEnvStr("DW_TEST_CLICKHOUSE_USER", "default")
	chPassword := getEnvStr("DW_TEST_CLICKHOUSE_PASSWORD", "clickhouse")
	chClient, err = ch.Connect(ctx, chAddr, chUser, chPassword, "dw_test")
	if err != nil {
		fmt.Printf("SKIP: streaming tests need local ClickHouse (tried %s): %v\n", chAddr, err)
		pool.Close()
		os.Exit(0)
	}
	if err := chClient.EnsureSchema(ctx); err != nil {
		fmt.Printf("FAIL: could not set up dw_test ClickHouse schema: %v\n", err)
		pool.Close()
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// makeEvent membangun raw JSON event dengan entity_id yang diberikan —
// meniru format yang dipublikasikan semua service bisnis ke Kafka.
func makeEvent(entityID uuid.UUID) []byte {
	b, _ := json.Marshal(map[string]string{"entity_id": entityID.String()})
	return b
}

// today mengembalikan tanggal hari ini dalam format yang bisa di-INSERT
// sebagai DATE di Postgres.
func today() string {
	return time.Now().Format("2006-01-02")
}

// mustExec menjalankan query dan gagal test kalau error.
func mustExec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("mustExec %q: %v", query, err)
	}
}

func getEnvStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
