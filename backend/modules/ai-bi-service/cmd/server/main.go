package main

import (
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/ai-bi-service/internal/config"
	"github.com/enterprise-digital-platform/ai-bi-service/internal/httpapi"
	"github.com/enterprise-digital-platform/ai-bi-service/internal/metrics"
)

func main() {
	cfg := config.Load()

	handler := httpapi.NewHandler(cfg)

	mux := http.NewServeMux()
	handler.Register(mux)

	log.Printf("ai-bi-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, metrics.Middleware(mux)); err != nil {
		log.Fatal(err)
	}
}
