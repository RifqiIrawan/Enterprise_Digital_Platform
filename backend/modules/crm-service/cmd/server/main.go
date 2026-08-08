package main

import (
	"context"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/crm-service/internal/config"
	"github.com/enterprise-digital-platform/crm-service/internal/eventbus"
	"github.com/enterprise-digital-platform/crm-service/internal/httpapi"
	"github.com/enterprise-digital-platform/crm-service/internal/logging"
	"github.com/enterprise-digital-platform/crm-service/internal/metrics"
	"github.com/enterprise-digital-platform/crm-service/internal/requestid"
	"github.com/enterprise-digital-platform/crm-service/internal/store"
	"github.com/enterprise-digital-platform/crm-service/internal/tracing"
	"github.com/enterprise-digital-platform/crm-service/migrations"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	logging.Init("crm-service")
	cfg := config.Load()
	ctx := context.Background()

	shutdownTracing := tracing.Init(ctx, "crm-service", cfg.OTLPEndpoint)
	defer shutdownTracing(context.Background())

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("crm-service: db connect failed: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("crm-service: migration failed: %v", err)
	}

	events := eventbus.NewPublisher(cfg.KafkaBrokers)
	defer events.Close()

	handler := httpapi.NewHandler(pool, events)

	mux := http.NewServeMux()
	handler.Register(mux)

	var topHandler http.Handler = metrics.Middleware(mux)
	topHandler = requestid.Middleware(topHandler)
	topHandler = otelhttp.NewHandler(topHandler, "crm-service")

	log.Printf("crm-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, topHandler); err != nil {
		log.Fatal(err)
	}
}
