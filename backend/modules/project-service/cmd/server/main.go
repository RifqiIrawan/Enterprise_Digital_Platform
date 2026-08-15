package main

import (
	"context"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/project-service/internal/config"
	"github.com/enterprise-digital-platform/project-service/internal/eventbus"
	"github.com/enterprise-digital-platform/project-service/internal/financeclient"
	"github.com/enterprise-digital-platform/project-service/internal/hrclient"
	"github.com/enterprise-digital-platform/project-service/internal/httpapi"
	"github.com/enterprise-digital-platform/project-service/internal/logging"
	"github.com/enterprise-digital-platform/project-service/internal/metrics"
	"github.com/enterprise-digital-platform/project-service/internal/requestid"
	"github.com/enterprise-digital-platform/project-service/internal/store"
	"github.com/enterprise-digital-platform/project-service/internal/tracing"
	"github.com/enterprise-digital-platform/project-service/migrations"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	logging.Init("project-service")
	cfg := config.Load()
	ctx := context.Background()

	shutdownTracing := tracing.Init(ctx, "project-service", cfg.OTLPEndpoint)
	defer shutdownTracing(context.Background())

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("project-service: db connect failed: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("project-service: migration failed: %v", err)
	}

	events := eventbus.NewPublisher(cfg.KafkaBrokers)
	defer events.Close()

	hr := hrclient.New(cfg.HRServiceURL)
	finance := financeclient.New(cfg.FinanceServiceURL)
	handler := httpapi.NewHandler(pool, events, hr, finance)

	mux := http.NewServeMux()
	handler.Register(mux)

	var topHandler http.Handler = metrics.Middleware(mux)
	topHandler = requestid.Middleware(topHandler)
	topHandler = otelhttp.NewHandler(topHandler, "project-service")

	log.Printf("project-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, topHandler); err != nil {
		log.Fatal(err)
	}
}
