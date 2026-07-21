package config

import "os"

type Config struct {
	Port                string
	DatabaseURL         string
	KafkaBrokers        string
	WarehouseServiceURL string
	OTLPEndpoint        string
}

func Load() *Config {
	return &Config{
		Port:                getEnv("PORT", "8090"),
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://platform:platform@localhost:5432/production_service?sslmode=disable"),
		KafkaBrokers:        getEnv("KAFKA_BROKERS", "localhost:9092"),
		WarehouseServiceURL: getEnv("WAREHOUSE_SERVICE_URL", "http://localhost:8089"),
		OTLPEndpoint:        getEnv("OTLP_ENDPOINT", "localhost:4318"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
