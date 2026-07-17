package main

import (
	"context"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/purchasing-service/internal/config"
	"github.com/enterprise-digital-platform/purchasing-service/internal/eventbus"
	"github.com/enterprise-digital-platform/purchasing-service/internal/financeclient"
	"github.com/enterprise-digital-platform/purchasing-service/internal/httpapi"
	"github.com/enterprise-digital-platform/purchasing-service/internal/logging"
	"github.com/enterprise-digital-platform/purchasing-service/internal/metrics"
	"github.com/enterprise-digital-platform/purchasing-service/internal/requestid"
	"github.com/enterprise-digital-platform/purchasing-service/internal/store"
	"github.com/enterprise-digital-platform/purchasing-service/internal/warehouseclient"
	"github.com/enterprise-digital-platform/purchasing-service/migrations"
)

func main() {
	logging.Init("purchasing-service")
	cfg := config.Load()
	ctx := context.Background()

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("purchasing-service: db connect failed: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("purchasing-service: migration failed: %v", err)
	}

	events := eventbus.NewPublisher(cfg.KafkaBrokers)
	defer events.Close()

	finance := financeclient.New(cfg.FinanceServiceURL)
	warehouse := warehouseclient.New(cfg.WarehouseServiceURL)

	handler := httpapi.NewHandler(pool, events, finance, warehouse)

	mux := http.NewServeMux()
	handler.Register(mux)

	var topHandler http.Handler = metrics.Middleware(mux)
	topHandler = requestid.Middleware(topHandler)

	log.Printf("purchasing-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, topHandler); err != nil {
		log.Fatal(err)
	}
}
