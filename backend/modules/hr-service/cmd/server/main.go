package main

import (
	"context"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/hr-service/internal/config"
	"github.com/enterprise-digital-platform/hr-service/internal/eventbus"
	"github.com/enterprise-digital-platform/hr-service/internal/financeclient"
	"github.com/enterprise-digital-platform/hr-service/internal/httpapi"
	"github.com/enterprise-digital-platform/hr-service/internal/metrics"
	"github.com/enterprise-digital-platform/hr-service/internal/store"
	"github.com/enterprise-digital-platform/hr-service/migrations"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("hr-service: db connect failed: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("hr-service: migration failed: %v", err)
	}

	events := eventbus.NewPublisher(cfg.KafkaBrokers)
	defer events.Close()

	finance := financeclient.New(cfg.FinanceServiceURL)

	handler := httpapi.NewHandler(pool, events, finance)

	mux := http.NewServeMux()
	handler.Register(mux)

	log.Printf("hr-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, metrics.Middleware(mux)); err != nil {
		log.Fatal(err)
	}
}
