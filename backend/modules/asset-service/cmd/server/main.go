package main

import (
	"context"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/asset-service/internal/config"
	"github.com/enterprise-digital-platform/asset-service/internal/eventbus"
	"github.com/enterprise-digital-platform/asset-service/internal/httpapi"
	"github.com/enterprise-digital-platform/asset-service/internal/logging"
	"github.com/enterprise-digital-platform/asset-service/internal/metrics"
	"github.com/enterprise-digital-platform/asset-service/internal/requestid"
	"github.com/enterprise-digital-platform/asset-service/internal/store"
	"github.com/enterprise-digital-platform/asset-service/migrations"
)

func main() {
	logging.Init("asset-service")
	cfg := config.Load()
	ctx := context.Background()

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("asset-service: db connect failed: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("asset-service: migration failed: %v", err)
	}

	events := eventbus.NewPublisher(cfg.KafkaBrokers)
	defer events.Close()

	handler := httpapi.NewHandler(pool, events)

	mux := http.NewServeMux()
	handler.Register(mux)

	var topHandler http.Handler = metrics.Middleware(mux)
	topHandler = requestid.Middleware(topHandler)

	log.Printf("asset-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, topHandler); err != nil {
		log.Fatal(err)
	}
}
