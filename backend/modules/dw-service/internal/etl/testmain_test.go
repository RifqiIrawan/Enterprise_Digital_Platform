package etl

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
)

var sourcePool *pgxpool.Pool
var chClient *ch.Client

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/dw_service_test?sslmode=disable"
)

// sourceSchema mendefinisikan tabel-tabel MINIMAL yang meniru bentuk
// journal_entries/journal_lines/accounts (finance-service), sales_orders/
// sales_order_lines/customers (sales-service), stock_movements/warehouses/
// products (warehouse-service), employees/payroll_runs/payroll_details
// (hr-service), suppliers/purchase_orders/purchase_order_lines
// (purchasing-service), bill_of_materials/work_orders (production-service),
// quality_standards/quality_inspections (qc-service), assets/
// maintenance_schedules (asset-service), dan devices/readings
// (iot-service) -- HANYA kolom yang benar-benar dipakai extract SQL di
// finance.go/sales.go/inventory.go/hr.go/purchasing.go/production.go/
// qc.go/asset.go/iot.go (atau wajib diisi karena FK/NOT NULL saat seeding
// test). Sengaja TIDAK mengimpor package migrations milik modul lain (itu
// akan jadi dependency test-time lintas modul yang tidak biasa untuk
// codebase ini) -- skema di sini independen, dites terhadap SQL extract
// yang sama persis dipakai produksi, bukan terhadap skema modul lain yang
// bisa berubah sendiri.
const sourceSchema = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- accounts dipakai BERSAMA oleh dua source berbeda dalam skema test ini:
-- finance-service's chart-of-accounts (account_code/account_name/account_type)
-- dan crm-service's customer accounts (account_code/name/account_type) --
-- keduanya nama tabel "accounts" di databasenya masing-masing (berbeda
-- database di production, jadi tidak pernah benar-benar bentrok), disatukan
-- di sini murni demi kesederhanaan test harness (satu Postgres test DB untuk
-- semua source). Kolom "name" ditambahkan khusus untuk kebutuhan test CRM;
-- account_name/account_type diberi DEFAULT '' supaya insert test CRM (yang
-- tidak peduli kolom-kolom finance) tidak perlu mengisinya.
CREATE TABLE IF NOT EXISTS accounts (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	account_code VARCHAR(20) NOT NULL,
	account_name VARCHAR(200) NOT NULL DEFAULT '',
	account_type VARCHAR(20) NOT NULL DEFAULT '',
	name VARCHAR(200) DEFAULT ''
);

-- Harness ini TIDAK pernah drop dw_service_test antar run, jadi CREATE TABLE
-- IF NOT EXISTS di atas jadi no-op total di mesin yang sudah pernah menjalankan
-- versi skema sebelumnya -- kolom "name" dan DEFAULT '' yang baru ditambahkan
-- tidak akan pernah sampai ke tabel lamanya. ALTER idempoten di bawah inilah
-- yang benar-benar membawa database test lama ikut naik versi (CI selalu mulai
-- dari database kosong, jadi di sana ini cuma no-op yang tidak berbahaya).
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
	customer_id UUID NOT NULL REFERENCES customers(id),
	order_date DATE NOT NULL,
	status VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sales_order_lines (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	sales_order_id UUID NOT NULL REFERENCES sales_orders(id),
	product_name VARCHAR(200) NOT NULL,
	quantity NUMERIC(12,2) NOT NULL DEFAULT 1,
	unit_price NUMERIC(15,2) NOT NULL DEFAULT 0,
	amount NUMERIC(15,2) NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS warehouses (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	code VARCHAR(20) NOT NULL,
	name VARCHAR(200) NOT NULL
);

CREATE TABLE IF NOT EXISTS products (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	sku VARCHAR(30) NOT NULL,
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
	reference_type VARCHAR(30) NOT NULL DEFAULT 'MANUAL',
	reference_id UUID,
	movement_date DATE NOT NULL DEFAULT CURRENT_DATE,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

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
	posted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS payroll_details (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	payroll_run_id UUID NOT NULL REFERENCES payroll_runs(id),
	employee_id UUID NOT NULL REFERENCES employees(id),
	employee_name VARCHAR(200) NOT NULL,
	basic_salary NUMERIC(15,2) NOT NULL DEFAULT 0,
	gross_salary NUMERIC(15,2) NOT NULL DEFAULT 0,
	total_deduction NUMERIC(15,2) NOT NULL DEFAULT 0,
	net_salary NUMERIC(15,2) NOT NULL DEFAULT 0,
	working_days SMALLINT NOT NULL DEFAULT 0,
	present_days SMALLINT NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

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
	supplier_id UUID NOT NULL REFERENCES suppliers(id),
	order_date DATE NOT NULL,
	status VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS purchase_order_lines (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	purchase_order_id UUID NOT NULL REFERENCES purchase_orders(id),
	product_name VARCHAR(200) NOT NULL,
	quantity NUMERIC(12,2) NOT NULL DEFAULT 1,
	unit_price NUMERIC(15,2) NOT NULL DEFAULT 0,
	amount NUMERIC(15,2) NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS bill_of_materials (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	bom_code VARCHAR(30) NOT NULL,
	product_id UUID NOT NULL
);

CREATE TABLE IF NOT EXISTS work_orders (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	wo_number VARCHAR(30) NOT NULL,
	bom_id UUID NOT NULL REFERENCES bill_of_materials(id),
	product_id UUID NOT NULL,
	warehouse_id UUID NOT NULL,
	quantity_planned NUMERIC(15,2) NOT NULL,
	quantity_produced NUMERIC(15,2),
	status VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
	planned_start_date DATE NOT NULL,
	planned_end_date DATE,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS quality_standards (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	standard_code VARCHAR(30) NOT NULL,
	product_id UUID NOT NULL
);

CREATE TABLE IF NOT EXISTS quality_inspections (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	inspection_number VARCHAR(30) NOT NULL,
	standard_id UUID NOT NULL REFERENCES quality_standards(id),
	product_id UUID NOT NULL,
	reference_type VARCHAR(20) NOT NULL DEFAULT 'MANUAL',
	reference_id UUID,
	reference_number VARCHAR(30),
	inspected_quantity NUMERIC(15,2) NOT NULL,
	passed_quantity NUMERIC(15,2) NOT NULL DEFAULT 0,
	failed_quantity NUMERIC(15,2) NOT NULL DEFAULT 0,
	result VARCHAR(10) NOT NULL,
	inspection_date DATE NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS assets (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	asset_code VARCHAR(30) NOT NULL,
	name VARCHAR(200) NOT NULL
);

CREATE TABLE IF NOT EXISTS maintenance_schedules (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	branch_id UUID,
	asset_id UUID NOT NULL REFERENCES assets(id),
	maintenance_type VARCHAR(100) NOT NULL,
	scheduled_date DATE NOT NULL,
	completed_date DATE,
	status VARCHAR(20) NOT NULL DEFAULT 'SCHEDULED',
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS devices (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	device_code VARCHAR(30) NOT NULL,
	device_type VARCHAR(20) NOT NULL
);

CREATE TABLE IF NOT EXISTS readings (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	device_id UUID NOT NULL REFERENCES devices(id),
	company_id UUID NOT NULL,
	branch_id UUID,
	reading_type VARCHAR(20) NOT NULL,
	value_numeric NUMERIC(15,4),
	value_text VARCHAR(200),
	recorded_at TIMESTAMPTZ NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

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

	adminURL := getEnv("DW_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: dw-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: dw-service tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE dw_service_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create dw_service_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("DW_TEST_DATABASE_URL", testDatabaseURL)
	sourcePool, err = pgxpool.New(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to dw_service_test: %v\n", err)
		os.Exit(0)
	}
	if _, err := sourcePool.Exec(ctx, sourceSchema); err != nil {
		fmt.Printf("FAIL: could not set up dw_service_test source schema: %v\n", err)
		sourcePool.Close()
		os.Exit(1)
	}

	chAddr := getEnv("DW_TEST_CLICKHOUSE_ADDR", "localhost:9101")
	chUser := getEnv("DW_TEST_CLICKHOUSE_USER", "default")
	chPassword := getEnv("DW_TEST_CLICKHOUSE_PASSWORD", "clickhouse")
	chClient, err = ch.Connect(ctx, chAddr, chUser, chPassword, "dw_test")
	if err != nil {
		fmt.Printf("SKIP: dw-service tests need a local ClickHouse (tried %s): %v\n", chAddr, err)
		sourcePool.Close()
		os.Exit(0)
	}
	if err := chClient.EnsureSchema(ctx); err != nil {
		fmt.Printf("FAIL: could not set up dw_test ClickHouse schema: %v\n", err)
		sourcePool.Close()
		os.Exit(1)
	}

	code := m.Run()
	sourcePool.Close()
	os.Exit(code)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
