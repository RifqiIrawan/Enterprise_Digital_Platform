package config

import "os"

type Config struct {
	Port         string
	DatabaseURL  string
	RedisURL     string
	KafkaBrokers string
}

func Load() *Config {
	return &Config{
		Port:         getEnv("PORT", "8083"),
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://platform:platform@localhost:5432/rbac_service?sslmode=disable"),
		RedisURL:     getEnv("REDIS_URL", "localhost:6379"),
		KafkaBrokers: getEnv("KAFKA_BROKERS", "localhost:9092"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
