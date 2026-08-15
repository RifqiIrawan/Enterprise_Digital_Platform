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
// di internal/etl/testmain_test.go tapi hanya untuk 11 domain (tidak IoT --
// IoT masuk lewat MQTT langsung ke Postgres, tidak lewat Kafka).
const streamingSourceSchema = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Finance. Tabel "accounts" ini dipakai BERSAMA oleh dua source berbeda dalam
-- skema test ini: chart-of-accounts finance-service (account_code/account_name/
-- account_type) dan customer accounts crm-service (account_code/name) -- di
-- production keduanya tabel "accounts" di database yang berbeda, jadi tidak
-- pernah benar-benar bentrok; disatukan di sini murni demi kesederhanaan
-- harness (satu Postgres test untuk semua domain). Kolom "name" khusus CRM.
CREATE TABLE IF NOT EXISTS accounts (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	account_code VARCHAR(20) NOT NULL,
	account_name VARCHAR(200) NOT NULL DEFAULT '',
	account_type VARCHAR(20) NOT NULL DEFAULT '',
	name VARCHAR(200) DEFAULT ''
);
-- Harness ini TIDAK pernah drop database test antar run, jadi CREATE TABLE IF
-- NOT EXISTS di atas jadi no-op total di mesin yang sudah pernah menjalankan
-- versi skema sebelumnya -- kolom/DEFAULT baru tidak akan pernah sampai ke
-- tabel lamanya. ALTER idempoten di bawah inilah yang membawa database test
-- lama ikut naik versi (di CI database selalu kosong, jadi cuma no-op).
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS name VARCHAR(200) DEFAULT '';
ALTER TABLE accounts ALTER COLUMN account_name SET DEFAULT '';
ALTER TABLE accounts ALTER COLUMN account_type SET DEFAULT '';
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

-- CRM (accounts di atas dipakai ulang sebagai parent opportunities)
CREATE TABLE IF NOT EXISTS opportunities (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	opportunity_number VARCHAR(30) NOT NULL,
	account_id UUID NOT NULL REFERENCES accounts(id),
	contact_id UUID,
	name VARCHAR(200) NOT NULL,
	stage VARCHAR(20) NOT NULL DEFAULT 'PROSPECTING',
	amount NUMERIC(15,2) NOT NULL DEFAULT 0,
	probability SMALLINT NOT NULL DEFAULT 0,
	expected_close_date DATE,
	owner_user_id UUID,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ticketing
CREATE TABLE IF NOT EXISTS ticket_categories (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	category_code VARCHAR(30) NOT NULL,
	name VARCHAR(100) NOT NULL
);
CREATE TABLE IF NOT EXISTS tickets (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	ticket_number VARCHAR(30) NOT NULL,
	category_id UUID NOT NULL REFERENCES ticket_categories(id),
	subject VARCHAR(200) NOT NULL,
	priority VARCHAR(20) NOT NULL DEFAULT 'MEDIUM',
	status VARCHAR(20) NOT NULL DEFAULT 'OPEN',
	requester_name VARCHAR(200) NOT NULL,
	requester_email VARCHAR(200),
	assigned_to UUID,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	resolved_at TIMESTAMPTZ,
	closed_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- E-Commerce
CREATE TABLE IF NOT EXISTS orders (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	order_number VARCHAR(30) NOT NULL,
	customer_name VARCHAR(200) NOT NULL,
	customer_email VARCHAR(200),
	status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
	order_date DATE NOT NULL DEFAULT CURRENT_DATE,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS order_items (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	order_id UUID NOT NULL REFERENCES orders(id),
	product_id UUID NOT NULL,
	product_sku VARCHAR(50) NOT NULL,
	product_name VARCHAR(200) NOT NULL,
	unit_price NUMERIC(18,2) NOT NULL DEFAULT 0,
	quantity NUMERIC(18,3) NOT NULL DEFAULT 1,
	line_total NUMERIC(18,2) NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS vehicles (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	vehicle_code VARCHAR(30) NOT NULL,
	plate_number VARCHAR(20) NOT NULL,
	name VARCHAR(200) NOT NULL,
	vehicle_type VARCHAR(20) NOT NULL DEFAULT 'VAN',
	status VARCHAR(20) NOT NULL DEFAULT 'AVAILABLE'
);

CREATE TABLE IF NOT EXISTS drivers (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	driver_code VARCHAR(30) NOT NULL,
	name VARCHAR(200) NOT NULL,
	status VARCHAR(20) NOT NULL DEFAULT 'AVAILABLE'
);

CREATE TABLE IF NOT EXISTS delivery_orders (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	delivery_number VARCHAR(30) NOT NULL,
	vehicle_id UUID NOT NULL REFERENCES vehicles(id),
	driver_id UUID NOT NULL REFERENCES drivers(id),
	ecommerce_order_id UUID,
	reference_number VARCHAR(30),
	recipient_name VARCHAR(200) NOT NULL,
	destination_address TEXT,
	scheduled_date DATE NOT NULL DEFAULT CURRENT_DATE,
	status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
	dispatched_at TIMESTAMPTZ,
	delivered_at TIMESTAMPTZ,
	cancelled_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS projects (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	project_code VARCHAR(30) NOT NULL,
	name VARCHAR(200) NOT NULL,
	status VARCHAR(20) NOT NULL DEFAULT 'PLANNING',
	budget_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
	actual_cost NUMERIC(18,2) NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS tasks (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	project_id UUID NOT NULL REFERENCES projects(id),
	task_number VARCHAR(30) NOT NULL,
	title VARCHAR(200) NOT NULL,
	status VARCHAR(20) NOT NULL DEFAULT 'TODO'
);

CREATE TABLE IF NOT EXISTS timesheets (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	project_id UUID NOT NULL REFERENCES projects(id),
	task_id UUID REFERENCES tasks(id),
	employee_id UUID NOT NULL,
	employee_name VARCHAR(200) NOT NULL,
	work_date DATE NOT NULL DEFAULT CURRENT_DATE,
	hours NUMERIC(6,2) NOT NULL,
	hourly_rate NUMERIC(14,2) NOT NULL DEFAULT 0,
	amount NUMERIC(18,2) NOT NULL DEFAULT 0,
	status VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
	approved_at TIMESTAMPTZ,
	posted_at TIMESTAMPTZ,
	journal_entry_id UUID,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
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
	// mencakup tabel dari semua domain). Tidak perlu 11 database terpisah untuk
	// test karena satu-satunya nama tabel yang benar-benar bentrok antar domain
	// ("accounts": finance vs crm) sudah disatukan di streamingSourceSchema.
	pools = &sourcedb.Pools{
		Finance:    pool,
		Sales:      pool,
		Warehouse:  pool,
		HR:         pool,
		Purchasing: pool,
		Production: pool,
		QC:         pool,
		Asset:      pool,
		CRM:        pool,
		Ticketing:  pool,
		Ecommerce:  pool,
		Fleet:      pool,
		Project:    pool,
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
