package main

import (
	"context"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/qc-service/internal/config"
	"github.com/enterprise-digital-platform/qc-service/internal/eventbus"
	"github.com/enterprise-digital-platform/qc-service/internal/httpapi"
	"github.com/enterprise-digital-platform/qc-service/internal/logging"
	"github.com/enterprise-digital-platform/qc-service/internal/metrics"
	"github.com/enterprise-digital-platform/qc-service/internal/requestid"
	"github.com/enterprise-digital-platform/qc-service/internal/store"
	"github.com/enterprise-digital-platform/qc-service/internal/tracing"
	"github.com/enterprise-digital-platform/qc-service/migrations"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	logging.Init("qc-service")
	cfg := config.Load()
	ctx := context.Background()

	shutdownTracing := tracing.Init(ctx, "qc-service", cfg.OTLPEndpoint)
	defer shutdownTracing(context.Background())

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("qc-service: db connect failed: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("qc-service: migration failed: %v", err)
	}

	events := eventbus.NewPublisher(cfg.KafkaBrokers)
	defer events.Close()

	handler := httpapi.NewHandler(pool, events)

	mux := http.NewServeMux()
	handler.Register(mux)

	var topHandler http.Handler = metrics.Middleware(mux)
	topHandler = requestid.Middleware(topHandler)
	topHandler = otelhttp.NewHandler(topHandler, "qc-service")

	log.Printf("qc-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, topHandler); err != nil {
		log.Fatal(err)
	}
}
