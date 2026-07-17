package main

import (
	"context"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/rbac-service/internal/config"
	"github.com/enterprise-digital-platform/rbac-service/internal/eventbus"
	"github.com/enterprise-digital-platform/rbac-service/internal/httpapi"
	"github.com/enterprise-digital-platform/rbac-service/internal/logging"
	"github.com/enterprise-digital-platform/rbac-service/internal/metrics"
	"github.com/enterprise-digital-platform/rbac-service/internal/requestid"
	"github.com/enterprise-digital-platform/rbac-service/internal/store"
	"github.com/enterprise-digital-platform/rbac-service/migrations"
)

func main() {
	logging.Init("rbac-service")
	cfg := config.Load()
	ctx := context.Background()

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("rbac-service: db connect failed: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("rbac-service: migration failed: %v", err)
	}

	events := eventbus.NewPublisher(cfg.KafkaBrokers)
	defer events.Close()

	handler := httpapi.NewHandler(pool, events)

	mux := http.NewServeMux()
	handler.Register(mux)

	var topHandler http.Handler = metrics.Middleware(mux)
	topHandler = requestid.Middleware(topHandler)

	log.Printf("rbac-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, topHandler); err != nil {
		log.Fatal(err)
	}
}
