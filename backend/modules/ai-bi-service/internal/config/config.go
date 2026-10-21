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
	OTLPEndpoint         string
}

func Load() *Config {
	return &Config{
		Port:                 getEnv("PORT", "28093"),
		OTLPEndpoint:         getEnv("OTLP_ENDPOINT", "localhost:4318"),
		SalesServiceURL:      getEnv("SALES_SERVICE_URL", "http://localhost:28087"),
		PurchasingServiceURL: getEnv("PURCHASING_SERVICE_URL", "http://localhost:28088"),
		FinanceServiceURL:    getEnv("FINANCE_SERVICE_URL", "http://localhost:28085"),
		WarehouseServiceURL:  getEnv("WAREHOUSE_SERVICE_URL", "http://localhost:28089"),
		ProductionServiceURL: getEnv("PRODUCTION_SERVICE_URL", "http://localhost:28090"),
		QCServiceURL:         getEnv("QC_SERVICE_URL", "http://localhost:28091"),
		HRServiceURL:         getEnv("HR_SERVICE_URL", "http://localhost:28086"),
		AssetServiceURL:      getEnv("ASSET_SERVICE_URL", "http://localhost:28092"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
