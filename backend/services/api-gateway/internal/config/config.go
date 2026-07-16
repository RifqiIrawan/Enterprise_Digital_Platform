package config

import (
	"log"
	"os"
)

type Config struct {
	Port                 string
	AppEnv               string
	AuthServiceURL       string
	CompanyServiceURL    string
	RBACServiceURL       string
	AuditServiceURL      string
	FinanceServiceURL    string
	HRServiceURL         string
	SalesServiceURL      string
	PurchasingServiceURL string
	WarehouseServiceURL  string
	ProductionServiceURL string
	QCServiceURL         string
	AssetServiceURL      string
	AIBIServiceURL       string
	IoTServiceURL        string
	DWServiceURL         string
	JWTSecret            string
	CORSAllowedOrigin    string
}

func Load() *Config {
	cfg := &Config{
		Port:                 getEnv("PORT", "8079"),
		AppEnv:               getEnv("APP_ENV", "development"),
		AuthServiceURL:       getEnv("AUTH_SERVICE_URL", "http://localhost:8081"),
		CompanyServiceURL:    getEnv("COMPANY_SERVICE_URL", "http://localhost:8082"),
		RBACServiceURL:       getEnv("RBAC_SERVICE_URL", "http://localhost:8083"),
		AuditServiceURL:      getEnv("AUDIT_SERVICE_URL", "http://localhost:8084"),
		FinanceServiceURL:    getEnv("FINANCE_SERVICE_URL", "http://localhost:8085"),
		HRServiceURL:         getEnv("HR_SERVICE_URL", "http://localhost:8086"),
		SalesServiceURL:      getEnv("SALES_SERVICE_URL", "http://localhost:8087"),
		PurchasingServiceURL: getEnv("PURCHASING_SERVICE_URL", "http://localhost:8088"),
		WarehouseServiceURL:  getEnv("WAREHOUSE_SERVICE_URL", "http://localhost:8089"),
		ProductionServiceURL: getEnv("PRODUCTION_SERVICE_URL", "http://localhost:8090"),
		QCServiceURL:         getEnv("QC_SERVICE_URL", "http://localhost:8091"),
		AssetServiceURL:      getEnv("ASSET_SERVICE_URL", "http://localhost:8092"),
		AIBIServiceURL:       getEnv("AI_BI_SERVICE_URL", "http://localhost:8093"),
		IoTServiceURL:        getEnv("IOT_SERVICE_URL", "http://localhost:8094"),
		DWServiceURL:         getEnv("DW_SERVICE_URL", "http://localhost:8095"),
		JWTSecret:            getEnv("JWT_SECRET", "change-me"),
		CORSAllowedOrigin:    getEnv("CORS_ALLOWED_ORIGIN", "http://localhost:3000"),
	}
	// api-gateway verifies incoming JWTs with this same secret (must match
	// auth-service) -- see the matching guard/comment in
	// auth-service/internal/config/config.go for why this can't just be a
	// misconfiguration warning.
	if cfg.AppEnv != "development" && cfg.JWTSecret == "change-me" {
		log.Fatalf("api-gateway: JWT_SECRET wajib diset eksplisit saat APP_ENV=%s (tidak boleh memakai default 'change-me')", cfg.AppEnv)
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
