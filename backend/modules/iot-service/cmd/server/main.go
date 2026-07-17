package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/iot-service/internal/config"
	"github.com/enterprise-digital-platform/iot-service/internal/eventbus"
	"github.com/enterprise-digital-platform/iot-service/internal/httpapi"
	"github.com/enterprise-digital-platform/iot-service/internal/ingest"
	"github.com/enterprise-digital-platform/iot-service/internal/logging"
	"github.com/enterprise-digital-platform/iot-service/internal/metrics"
	"github.com/enterprise-digital-platform/iot-service/internal/mqttclient"
	"github.com/enterprise-digital-platform/iot-service/internal/requestid"
	"github.com/enterprise-digital-platform/iot-service/internal/simulator"
	"github.com/enterprise-digital-platform/iot-service/internal/store"
	"github.com/enterprise-digital-platform/iot-service/migrations"
)

func main() {
	logging.Init("iot-service")
	cfg := config.Load()
	ctx := context.Background()

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("iot-service: db connect failed: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("iot-service: migration failed: %v", err)
	}

	events := eventbus.NewPublisher(cfg.KafkaBrokers)
	defer events.Close()

	// Mosquitto adalah infra tambahan (bukan dependency inti seperti
	// Postgres) -- kegagalan konek dicatat sebagai warning, service tetap
	// jalan (endpoint CRUD devices/readings/alerts tetap bisa dipakai),
	// hanya saja tidak ada simulasi/ingest data baru sampai broker-nya naik
	// dan service ini di-restart.
	mqttClient, err := mqttclient.Connect(cfg.MQTTBrokerURL, "iot-service")
	if err != nil {
		log.Printf("iot-service: mqtt connect failed, continuing without simulator/ingest: %v", err)
	} else {
		if err := mqttClient.Subscribe("iot/+/+/reading", func(topic string, payload []byte) {
			var msg simulator.ReadingMessage
			if err := json.Unmarshal(payload, &msg); err != nil {
				log.Printf("iot-service: decode reading from %s failed: %v", topic, err)
				return
			}
			if err := ingest.Ingest(ctx, pool, events, ingest.Payload{
				DeviceID:     msg.DeviceID,
				ValueNumeric: msg.ValueNumeric,
				ValueText:    msg.ValueText,
				RecordedAt:   msg.RecordedAt,
			}); err != nil {
				log.Printf("iot-service: ingest reading from %s failed: %v", topic, err)
			}
		}); err != nil {
			log.Printf("iot-service: mqtt subscribe failed, continuing without ingest: %v", err)
		}

		if cfg.SimulatorEnabled {
			go simulator.New(pool, mqttClient, cfg.SimulatorIntervalSeconds).Start(ctx)
		}
		defer mqttClient.Close()
	}

	handler := httpapi.NewHandler(pool, events)

	mux := http.NewServeMux()
	handler.Register(mux)

	var topHandler http.Handler = metrics.Middleware(mux)
	topHandler = requestid.Middleware(topHandler)

	log.Printf("iot-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, topHandler); err != nil {
		log.Fatal(err)
	}
}
