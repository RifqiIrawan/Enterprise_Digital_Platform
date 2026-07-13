package config

import (
	"log"
	"os"
)

type Config struct {
	Port            string
	AppEnv          string
	DatabaseURL     string
	RedisURL        string
	KafkaBrokers    string
	JWTSecret       string
	AccessTokenTTL  string
	RefreshTokenTTL string
}

func Load() *Config {
	cfg := &Config{
		Port:            getEnv("PORT", "8081"),
		AppEnv:          getEnv("APP_ENV", "development"),
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://platform:platform@localhost:5432/auth_service?sslmode=disable"),
		RedisURL:        getEnv("REDIS_URL", "localhost:6379"),
		KafkaBrokers:    getEnv("KAFKA_BROKERS", "localhost:9092"),
		JWTSecret:       getEnv("JWT_SECRET", "change-me"),
		AccessTokenTTL:  getEnv("ACCESS_TOKEN_TTL", "15m"),
		RefreshTokenTTL: getEnv("REFRESH_TOKEN_TTL", "168h"),
	}
	// auth-service signs every access/refresh token with JWTSecret, so the
	// "change-me" dev default reaching a real deployment is a full auth
	// bypass, not just a misconfiguration -- fail loudly instead of
	// silently issuing forgeable tokens.
	if cfg.AppEnv != "development" && cfg.JWTSecret == "change-me" {
		log.Fatalf("auth-service: JWT_SECRET wajib diset eksplisit saat APP_ENV=%s (tidak boleh memakai default 'change-me')", cfg.AppEnv)
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
