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
// sales_order_lines/customers (sales-service), dan stock_movements/
// warehouses/products (warehouse-service) -- HANYA kolom yang benar-benar
// dipakai extract SQL di finance.go/sales.go/inventory.go. Sengaja TIDAK
// mengimpor package migrations milik modul lain (itu akan jadi dependency
// test-time lintas modul yang tidak biasa untuk codebase ini) -- skema di
// sini independen, dites terhadap SQL extract yang sama persis dipakai
// produksi, bukan terhadap skema modul lain yang bisa berubah sendiri.
const sourceSchema = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

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
