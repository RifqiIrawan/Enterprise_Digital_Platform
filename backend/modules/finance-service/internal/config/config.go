package config

import "os"

type Config struct {
	Port         string
	DatabaseURL  string
	KafkaBrokers string
}

func Load() *Config {
	return &Config{
		Port:         getEnv("PORT", "8085"),
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://platform:platform@localhost:5432/finance_service?sslmode=disable"),
		KafkaBrokers: getEnv("KAFKA_BROKERS", "localhost:9092"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
