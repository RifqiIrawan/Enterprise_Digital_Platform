package main

import (
	"context"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/audit-service/internal/config"
	"github.com/enterprise-digital-platform/audit-service/internal/consumer"
	"github.com/enterprise-digital-platform/audit-service/internal/httpapi"
	"github.com/enterprise-digital-platform/audit-service/internal/metrics"
	"github.com/enterprise-digital-platform/audit-service/internal/store"
	"github.com/enterprise-digital-platform/audit-service/migrations"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("audit-service: db connect failed: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("audit-service: migration failed: %v", err)
	}

	handler := httpapi.NewHandler(pool)

	consumer.Start(ctx, cfg.KafkaBrokers, cfg.KafkaGroupID, func(topic string, value []byte) {
		handler.Ingest(ctx, topic, value)
	})

	mux := http.NewServeMux()
	handler.Register(mux)

	log.Printf("audit-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, metrics.Middleware(mux)); err != nil {
		log.Fatal(err)
	}
}
