package config

import (
	"log"
	"os"
	"strconv"
	"time"
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
	CRMServiceURL        string
	TicketingServiceURL  string
	EcommerceServiceURL  string
	FleetServiceURL      string
	ProjectServiceURL    string
	JWTSecret            string
	CORSAllowedOrigin    string
	OTLPEndpoint         string

	// Penegakan hak akses (lihat internal/authz).
	AuthzEnforce    bool
	AuthzCacheTTL   time.Duration
	AuthzStaleGrace time.Duration
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
		CRMServiceURL:        getEnv("CRM_SERVICE_URL", "http://localhost:8096"),
		TicketingServiceURL:  getEnv("TICKETING_SERVICE_URL", "http://localhost:8097"),
		EcommerceServiceURL:  getEnv("ECOMMERCE_SERVICE_URL", "http://localhost:8098"),
		FleetServiceURL:      getEnv("FLEET_SERVICE_URL", "http://localhost:8099"),
		ProjectServiceURL:    getEnv("PROJECT_SERVICE_URL", "http://localhost:8100"),
		JWTSecret:            getEnv("JWT_SECRET", "change-me"),
		CORSAllowedOrigin:    getEnv("CORS_ALLOWED_ORIGIN", "http://localhost:3000"),
		OTLPEndpoint:         getEnv("OTLP_ENDPOINT", "localhost:4318"),

		// Default menyala. AUTHZ_ENFORCE=false hanya ada sebagai jalan keluar
		// darurat kalau sebuah endpoint baru terlanjur diluncurkan tanpa
		// terdaftar di tabel kebijakan dan seluruh halaman ikut terkunci --
		// mematikannya mengembalikan platform ke keadaan sebelum penegakan
		// ada, yaitu token valid = boleh apa saja.
		AuthzEnforce:    getBool("AUTHZ_ENFORCE", true),
		AuthzCacheTTL:   getDuration("AUTHZ_CACHE_TTL", 30*time.Second),
		AuthzStaleGrace: getDuration("AUTHZ_STALE_GRACE", 5*time.Minute),
	}
	if !cfg.AuthzEnforce {
		log.Print("api-gateway: PERINGATAN -- AUTHZ_ENFORCE=false, hak akses TIDAK ditegakkan; setiap token valid boleh memanggil endpoint apa pun")
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

func getBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		log.Printf("api-gateway: nilai %s=%q tidak terbaca sebagai boolean, memakai %v", key, v, fallback)
		return fallback
	}
	return parsed
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("api-gateway: nilai %s=%q tidak terbaca sebagai durasi, memakai %v", key, v, fallback)
		return fallback
	}
	return parsed
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
