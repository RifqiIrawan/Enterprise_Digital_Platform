package main

import (
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/ai-bi-service/internal/config"
	"github.com/enterprise-digital-platform/ai-bi-service/internal/httpapi"
	"github.com/enterprise-digital-platform/ai-bi-service/internal/logging"
	"github.com/enterprise-digital-platform/ai-bi-service/internal/metrics"
	"github.com/enterprise-digital-platform/ai-bi-service/internal/requestid"
)

func main() {
	logging.Init("ai-bi-service")
	cfg := config.Load()

	handler := httpapi.NewHandler(cfg)

	mux := http.NewServeMux()
	handler.Register(mux)

	var topHandler http.Handler = metrics.Middleware(mux)
	topHandler = requestid.Middleware(topHandler)

	log.Printf("ai-bi-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, topHandler); err != nil {
		log.Fatal(err)
	}
}
