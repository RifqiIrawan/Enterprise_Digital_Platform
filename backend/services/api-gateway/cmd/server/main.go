package main

import (
	"context"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/api-gateway/internal/config"
	"github.com/enterprise-digital-platform/api-gateway/internal/gateway"
	"github.com/enterprise-digital-platform/api-gateway/internal/logging"
	"github.com/enterprise-digital-platform/api-gateway/internal/tracing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	logging.Init("api-gateway")
	cfg := config.Load()

	shutdownTracing := tracing.Init(context.Background(), "api-gateway", cfg.OTLPEndpoint)
	defer shutdownTracing(context.Background())

	handler := otelhttp.NewHandler(gateway.New(cfg), "api-gateway")

	log.Printf("api-gateway listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatal(err)
	}
}
