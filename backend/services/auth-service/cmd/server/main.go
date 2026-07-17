package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/enterprise-digital-platform/auth-service/internal/config"
	"github.com/enterprise-digital-platform/auth-service/internal/eventbus"
	"github.com/enterprise-digital-platform/auth-service/internal/httpapi"
	"github.com/enterprise-digital-platform/auth-service/internal/logging"
	"github.com/enterprise-digital-platform/auth-service/internal/metrics"
	"github.com/enterprise-digital-platform/auth-service/internal/requestid"
	"github.com/enterprise-digital-platform/auth-service/internal/store"
	"github.com/enterprise-digital-platform/auth-service/migrations"
)

func main() {
	logging.Init("auth-service")
	cfg := config.Load()
	ctx := context.Background()

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("auth-service: db connect failed: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("auth-service: migration failed: %v", err)
	}

	accessTTL, err := time.ParseDuration(cfg.AccessTokenTTL)
	if err != nil {
		log.Fatalf("auth-service: invalid ACCESS_TOKEN_TTL: %v", err)
	}
	refreshTTL, err := time.ParseDuration(cfg.RefreshTokenTTL)
	if err != nil {
		log.Fatalf("auth-service: invalid REFRESH_TOKEN_TTL: %v", err)
	}

	events := eventbus.NewPublisher(cfg.KafkaBrokers)
	defer events.Close()

	handler := httpapi.NewHandler(pool, events, cfg.JWTSecret, accessTTL, refreshTTL)

	mux := http.NewServeMux()
	handler.Register(mux)

	var topHandler http.Handler = metrics.Middleware(mux)
	topHandler = requestid.Middleware(topHandler)

	log.Printf("auth-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, topHandler); err != nil {
		log.Fatal(err)
	}
}
