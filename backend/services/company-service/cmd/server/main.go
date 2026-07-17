package main

import (
	"context"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/company-service/internal/config"
	"github.com/enterprise-digital-platform/company-service/internal/eventbus"
	"github.com/enterprise-digital-platform/company-service/internal/httpapi"
	"github.com/enterprise-digital-platform/company-service/internal/metrics"
	"github.com/enterprise-digital-platform/company-service/internal/store"
	"github.com/enterprise-digital-platform/company-service/migrations"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("company-service: db connect failed: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("company-service: migration failed: %v", err)
	}

	events := eventbus.NewPublisher(cfg.KafkaBrokers)
	defer events.Close()

	handler := httpapi.NewHandler(pool, events)

	mux := http.NewServeMux()
	handler.Register(mux)

	log.Printf("company-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, metrics.Middleware(mux)); err != nil {
		log.Fatal(err)
	}
}
