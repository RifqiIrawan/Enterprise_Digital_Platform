package config

import "os"

type Config struct {
	Port                 string
	AuthServiceURL       string
	CompanyServiceURL    string
	RBACServiceURL       string
	AuditServiceURL      string
	FinanceServiceURL    string
	HRServiceURL         string
	SalesServiceURL      string
	PurchasingServiceURL string
	JWTSecret            string
	CORSAllowedOrigin    string
}

func Load() *Config {
	return &Config{
		Port:                 getEnv("PORT", "8079"),
		AuthServiceURL:       getEnv("AUTH_SERVICE_URL", "http://localhost:8081"),
		CompanyServiceURL:    getEnv("COMPANY_SERVICE_URL", "http://localhost:8082"),
		RBACServiceURL:       getEnv("RBAC_SERVICE_URL", "http://localhost:8083"),
		AuditServiceURL:      getEnv("AUDIT_SERVICE_URL", "http://localhost:8084"),
		FinanceServiceURL:    getEnv("FINANCE_SERVICE_URL", "http://localhost:8085"),
		HRServiceURL:         getEnv("HR_SERVICE_URL", "http://localhost:8086"),
		SalesServiceURL:      getEnv("SALES_SERVICE_URL", "http://localhost:8087"),
		PurchasingServiceURL: getEnv("PURCHASING_SERVICE_URL", "http://localhost:8088"),
		JWTSecret:            getEnv("JWT_SECRET", "change-me"),
		CORSAllowedOrigin:    getEnv("CORS_ALLOWED_ORIGIN", "http://localhost:3000"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
