package main

import (
	"context"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/fleet-service/internal/config"
	"github.com/enterprise-digital-platform/fleet-service/internal/ecommerceclient"
	"github.com/enterprise-digital-platform/fleet-service/internal/eventbus"
	"github.com/enterprise-digital-platform/fleet-service/internal/httpapi"
	"github.com/enterprise-digital-platform/fleet-service/internal/logging"
	"github.com/enterprise-digital-platform/fleet-service/internal/metrics"
	"github.com/enterprise-digital-platform/fleet-service/internal/requestid"
	"github.com/enterprise-digital-platform/fleet-service/internal/store"
	"github.com/enterprise-digital-platform/fleet-service/internal/tracing"
	"github.com/enterprise-digital-platform/fleet-service/migrations"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	logging.Init("fleet-service")
	cfg := config.Load()
	ctx := context.Background()

	shutdownTracing := tracing.Init(ctx, "fleet-service", cfg.OTLPEndpoint)
	defer shutdownTracing(context.Background())

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("fleet-service: db connect failed: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("fleet-service: migration failed: %v", err)
	}

	events := eventbus.NewPublisher(cfg.KafkaBrokers)
	defer events.Close()

	ecommerce := ecommerceclient.New(cfg.EcommerceServiceURL)
	handler := httpapi.NewHandler(pool, events, ecommerce)

	mux := http.NewServeMux()
	handler.Register(mux)

	var topHandler http.Handler = metrics.Middleware(mux)
	topHandler = requestid.Middleware(topHandler)
	topHandler = otelhttp.NewHandler(topHandler, "fleet-service")

	log.Printf("fleet-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, topHandler); err != nil {
		log.Fatal(err)
	}
}
