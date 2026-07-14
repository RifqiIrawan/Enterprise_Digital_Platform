package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port                     string
	DatabaseURL              string
	KafkaBrokers             string
	MQTTBrokerURL            string
	SimulatorEnabled         bool
	SimulatorIntervalSeconds int
}

func Load() *Config {
	return &Config{
		Port:                     getEnv("PORT", "8094"),
		DatabaseURL:              getEnv("DATABASE_URL", "postgres://platform:platform@localhost:5432/iot_service?sslmode=disable"),
		KafkaBrokers:             getEnv("KAFKA_BROKERS", "localhost:9092"),
		MQTTBrokerURL:            getEnv("MQTT_BROKER_URL", "tcp://localhost:1883"),
		SimulatorEnabled:         getEnv("IOT_SIMULATOR_ENABLED", "true") == "true",
		SimulatorIntervalSeconds: getEnvInt("IOT_SIMULATOR_INTERVAL_SECONDS", 15),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
