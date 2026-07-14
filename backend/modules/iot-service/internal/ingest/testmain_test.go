package ingest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/iot-service/internal/store"
	"github.com/enterprise-digital-platform/iot-service/migrations"
)

var pool *pgxpool.Pool

const (
	adminDatabaseURL = "postgres://platform:platform@localhost:5432/postgres?sslmode=disable"
	testDatabaseURL  = "postgres://platform:platform@localhost:5432/iot_service_ingest_test?sslmode=disable"
)

// TestMain uses its own database (iot_service_ingest_test), separate from
// internal/httpapi's iot_service_test. `go test ./...` runs each package's
// tests as an independent concurrent process, and Postgres DDL (CREATE
// TABLE/CREATE EXTENSION, even the "IF NOT EXISTS" forms used by
// store.Migrate) isn't safe to run truly concurrently against the same
// database from two separate connections -- that raced here initially
// ("duplicate key value violates unique constraint
// pg_type_typname_nsp_index") before each package got its own DB.
func TestMain(m *testing.M) {
	ctx := context.Background()

	adminURL := getEnv("IOT_INGEST_TEST_ADMIN_DATABASE_URL", adminDatabaseURL)
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Printf("SKIP: iot-service ingest tests need a local Postgres (tried %s): %v\n", adminURL, err)
		os.Exit(0)
	}
	if err := adminPool.Ping(ctx); err != nil {
		fmt.Printf("SKIP: iot-service ingest tests need a local Postgres (tried %s): %v\n", adminURL, err)
		adminPool.Close()
		os.Exit(0)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE iot_service_ingest_test"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Printf("FAIL: could not create iot_service_ingest_test database: %v\n", err)
			adminPool.Close()
			os.Exit(1)
		}
	}
	adminPool.Close()

	testURL := getEnv("IOT_INGEST_TEST_DATABASE_URL", testDatabaseURL)
	pool, err = store.Connect(ctx, testURL)
	if err != nil {
		fmt.Printf("SKIP: could not connect to iot_service_ingest_test: %v\n", err)
		os.Exit(0)
	}
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Printf("FAIL: migration of iot_service_ingest_test failed: %v\n", err)
		pool.Close()
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
