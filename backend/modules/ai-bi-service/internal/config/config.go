package config

import "os"

// Config tidak punya DatabaseURL/KafkaBrokers seperti service lain --
// ai-bi-service tidak punya data transaksi sendiri, cuma agregasi live lewat
// HTTP dari 8 service lain (lihat internal/httpapi/summary.go).
type Config struct {
	Port                 string
	SalesServiceURL      string
	PurchasingServiceURL string
	FinanceServiceURL    string
	WarehouseServiceURL  string
	ProductionServiceURL string
	QCServiceURL         string
	HRServiceURL         string
	AssetServiceURL      string
}

func Load() *Config {
	return &Config{
		Port:                 getEnv("PORT", "8093"),
		SalesServiceURL:      getEnv("SALES_SERVICE_URL", "http://localhost:8087"),
		PurchasingServiceURL: getEnv("PURCHASING_SERVICE_URL", "http://localhost:8088"),
		FinanceServiceURL:    getEnv("FINANCE_SERVICE_URL", "http://localhost:8085"),
		WarehouseServiceURL:  getEnv("WAREHOUSE_SERVICE_URL", "http://localhost:8089"),
		ProductionServiceURL: getEnv("PRODUCTION_SERVICE_URL", "http://localhost:8090"),
		QCServiceURL:         getEnv("QC_SERVICE_URL", "http://localhost:8091"),
		HRServiceURL:         getEnv("HR_SERVICE_URL", "http://localhost:8086"),
		AssetServiceURL:      getEnv("ASSET_SERVICE_URL", "http://localhost:8092"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
