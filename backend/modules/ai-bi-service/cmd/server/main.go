package main

import (
	"context"
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/ai-bi-service/internal/config"
	"github.com/enterprise-digital-platform/ai-bi-service/internal/httpapi"
	"github.com/enterprise-digital-platform/ai-bi-service/internal/logging"
	"github.com/enterprise-digital-platform/ai-bi-service/internal/metrics"
	"github.com/enterprise-digital-platform/ai-bi-service/internal/requestid"
	"github.com/enterprise-digital-platform/ai-bi-service/internal/tracing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	logging.Init("ai-bi-service")
	cfg := config.Load()

	shutdownTracing := tracing.Init(context.Background(), "ai-bi-service", cfg.OTLPEndpoint)
	defer shutdownTracing(context.Background())

	handler := httpapi.NewHandler(cfg)

	mux := http.NewServeMux()
	handler.Register(mux)

	var topHandler http.Handler = metrics.Middleware(mux)
	topHandler = requestid.Middleware(topHandler)
	topHandler = otelhttp.NewHandler(topHandler, "ai-bi-service")

	log.Printf("ai-bi-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, topHandler); err != nil {
		log.Fatal(err)
	}
}
