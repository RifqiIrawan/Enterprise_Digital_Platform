package main

import (
	"context"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/audit-service/internal/config"
	"github.com/enterprise-digital-platform/audit-service/internal/consumer"
	"github.com/enterprise-digital-platform/audit-service/internal/httpapi"
	"github.com/enterprise-digital-platform/audit-service/internal/logging"
	"github.com/enterprise-digital-platform/audit-service/internal/metrics"
	"github.com/enterprise-digital-platform/audit-service/internal/requestid"
	"github.com/enterprise-digital-platform/audit-service/internal/store"
	"github.com/enterprise-digital-platform/audit-service/internal/tracing"
	"github.com/enterprise-digital-platform/audit-service/migrations"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	logging.Init("audit-service")
	cfg := config.Load()
	ctx := context.Background()

	shutdownTracing := tracing.Init(ctx, "audit-service", cfg.OTLPEndpoint)
	defer shutdownTracing(context.Background())

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

	var topHandler http.Handler = metrics.Middleware(mux)
	topHandler = requestid.Middleware(topHandler)
	topHandler = otelhttp.NewHandler(topHandler, "audit-service")

	log.Printf("audit-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, topHandler); err != nil {
		log.Fatal(err)
	}
}
