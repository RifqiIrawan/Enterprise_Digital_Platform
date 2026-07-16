package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port                  string
	FinanceDatabaseURL    string
	SalesDatabaseURL      string
	WarehouseDatabaseURL  string
	HRDatabaseURL         string
	PurchasingDatabaseURL string
	ProductionDatabaseURL string
	QCDatabaseURL         string
	AssetDatabaseURL      string
	IoTDatabaseURL        string
	ClickHouseAddr        string
	ClickHouseUser        string
	ClickHousePassword    string
	ClickHouseDatabase    string
	MinIOEndpoint         string
	MinIOAccessKey        string
	MinIOSecretKey        string
	MinIOBucket           string
	MinIOUseSSL           bool
	SyncEnabled           bool
	SyncIntervalSeconds   int
}

func Load() *Config {
	return &Config{
		Port:                  getEnv("PORT", "8095"),
		FinanceDatabaseURL:    getEnv("FINANCE_DATABASE_URL", "postgres://platform:platform@localhost:5432/finance_service?sslmode=disable"),
		SalesDatabaseURL:      getEnv("SALES_DATABASE_URL", "postgres://platform:platform@localhost:5432/sales_service?sslmode=disable"),
		WarehouseDatabaseURL:  getEnv("WAREHOUSE_DATABASE_URL", "postgres://platform:platform@localhost:5432/warehouse_service?sslmode=disable"),
		HRDatabaseURL:         getEnv("HR_DATABASE_URL", "postgres://platform:platform@localhost:5432/hr_service?sslmode=disable"),
		PurchasingDatabaseURL: getEnv("PURCHASING_DATABASE_URL", "postgres://platform:platform@localhost:5432/purchasing_service?sslmode=disable"),
		ProductionDatabaseURL: getEnv("PRODUCTION_DATABASE_URL", "postgres://platform:platform@localhost:5432/production_service?sslmode=disable"),
		QCDatabaseURL:         getEnv("QC_DATABASE_URL", "postgres://platform:platform@localhost:5432/qc_service?sslmode=disable"),
		AssetDatabaseURL:      getEnv("ASSET_DATABASE_URL", "postgres://platform:platform@localhost:5432/asset_service?sslmode=disable"),
		IoTDatabaseURL:        getEnv("IOT_DATABASE_URL", "postgres://platform:platform@localhost:5432/iot_service?sslmode=disable"),
		ClickHouseAddr:        getEnv("CLICKHOUSE_ADDR", "localhost:9101"),
		ClickHouseUser:        getEnv("CLICKHOUSE_USER", "default"),
		ClickHousePassword:    getEnv("CLICKHOUSE_PASSWORD", "clickhouse"),
		ClickHouseDatabase:    getEnv("CLICKHOUSE_DATABASE", "dw"),
		MinIOEndpoint:         getEnv("MINIO_ENDPOINT", "localhost:9004"),
		MinIOAccessKey:        getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:        getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinIOBucket:           getEnv("MINIO_BUCKET", "dw-lake"),
		MinIOUseSSL:           getEnv("MINIO_USE_SSL", "false") == "true",
		SyncEnabled:           getEnv("DW_SYNC_ENABLED", "true") == "true",
		SyncIntervalSeconds:   getEnvInt("DW_SYNC_INTERVAL_SECONDS", 300),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
