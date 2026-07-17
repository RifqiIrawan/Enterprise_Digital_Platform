package main

import (
	"context"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/warehouse-service/internal/config"
	"github.com/enterprise-digital-platform/warehouse-service/internal/eventbus"
	"github.com/enterprise-digital-platform/warehouse-service/internal/httpapi"
	"github.com/enterprise-digital-platform/warehouse-service/internal/logging"
	"github.com/enterprise-digital-platform/warehouse-service/internal/metrics"
	"github.com/enterprise-digital-platform/warehouse-service/internal/requestid"
	"github.com/enterprise-digital-platform/warehouse-service/internal/store"
	"github.com/enterprise-digital-platform/warehouse-service/migrations"
)

func main() {
	logging.Init("warehouse-service")
	cfg := config.Load()
	ctx := context.Background()

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("warehouse-service: db connect failed: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("warehouse-service: migration failed: %v", err)
	}

	events := eventbus.NewPublisher(cfg.KafkaBrokers)
	defer events.Close()

	handler := httpapi.NewHandler(pool, events)

	mux := http.NewServeMux()
	handler.Register(mux)

	var topHandler http.Handler = metrics.Middleware(mux)
	topHandler = requestid.Middleware(topHandler)

	log.Printf("warehouse-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, topHandler); err != nil {
		log.Fatal(err)
	}
}
