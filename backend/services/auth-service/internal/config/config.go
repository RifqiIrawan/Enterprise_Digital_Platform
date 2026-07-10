package config

import "os"

type Config struct {
	Port            string
	DatabaseURL     string
	RedisURL        string
	KafkaBrokers    string
	JWTSecret       string
	AccessTokenTTL  string
	RefreshTokenTTL string
}

func Load() *Config {
	return &Config{
		Port:            getEnv("PORT", "8081"),
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://platform:platform@localhost:5432/auth_service?sslmode=disable"),
		RedisURL:        getEnv("REDIS_URL", "localhost:6379"),
		KafkaBrokers:    getEnv("KAFKA_BROKERS", "localhost:9092"),
		JWTSecret:       getEnv("JWT_SECRET", "change-me"),
		AccessTokenTTL:  getEnv("ACCESS_TOKEN_TTL", "15m"),
		RefreshTokenTTL: getEnv("REFRESH_TOKEN_TTL", "168h"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
