package config

import "os"

type Config struct {
	Port         string
	DatabaseURL  string
	KafkaBrokers string
	OTLPEndpoint string
	// Dipakai untuk memposting jurnal penyusutan; panggilan langsung ke
	// finance-service, tidak lewat gateway (lihat internal/financeclient).
	FinanceServiceURL string
}

func Load() *Config {
	return &Config{
		Port:              getEnv("PORT", "8092"),
		DatabaseURL:       getEnv("DATABASE_URL", "postgres://platform:platform@localhost:5432/asset_service?sslmode=disable"),
		KafkaBrokers:      getEnv("KAFKA_BROKERS", "localhost:9092"),
		OTLPEndpoint:      getEnv("OTLP_ENDPOINT", "localhost:4318"),
		FinanceServiceURL: getEnv("FINANCE_SERVICE_URL", "http://localhost:8085"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
