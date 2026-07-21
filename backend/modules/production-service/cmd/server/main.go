package main

import (
	"context"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/production-service/internal/config"
	"github.com/enterprise-digital-platform/production-service/internal/eventbus"
	"github.com/enterprise-digital-platform/production-service/internal/httpapi"
	"github.com/enterprise-digital-platform/production-service/internal/logging"
	"github.com/enterprise-digital-platform/production-service/internal/metrics"
	"github.com/enterprise-digital-platform/production-service/internal/requestid"
	"github.com/enterprise-digital-platform/production-service/internal/store"
	"github.com/enterprise-digital-platform/production-service/internal/tracing"
	"github.com/enterprise-digital-platform/production-service/internal/warehouseclient"
	"github.com/enterprise-digital-platform/production-service/migrations"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	logging.Init("production-service")
	cfg := config.Load()
	ctx := context.Background()

	shutdownTracing := tracing.Init(ctx, "production-service", cfg.OTLPEndpoint)
	defer shutdownTracing(context.Background())

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("production-service: db connect failed: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("production-service: migration failed: %v", err)
	}

	events := eventbus.NewPublisher(cfg.KafkaBrokers)
	defer events.Close()

	warehouse := warehouseclient.New(cfg.WarehouseServiceURL)

	handler := httpapi.NewHandler(pool, events, warehouse)

	mux := http.NewServeMux()
	handler.Register(mux)

	var topHandler http.Handler = metrics.Middleware(mux)
	topHandler = requestid.Middleware(topHandler)
	topHandler = otelhttp.NewHandler(topHandler, "production-service")

	log.Printf("production-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, topHandler); err != nil {
		log.Fatal(err)
	}
}
