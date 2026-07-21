package config

import "os"

type Config struct {
	Port          string
	DatabaseURL   string
	ClickHouseURL string
	KafkaBrokers  string
	KafkaGroupID  string
	OTLPEndpoint  string
}

func Load() *Config {
	return &Config{
		Port:          getEnv("PORT", "8084"),
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://platform:platform@localhost:5432/audit_service?sslmode=disable"),
		ClickHouseURL: getEnv("CLICKHOUSE_URL", "http://localhost:8123"),
		KafkaBrokers:  getEnv("KAFKA_BROKERS", "localhost:9092"),
		KafkaGroupID:  getEnv("KAFKA_GROUP_ID", "audit-service"),
		OTLPEndpoint:  getEnv("OTLP_ENDPOINT", "localhost:4318"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
