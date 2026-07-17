package main

import (
	"log"
	"net/http"

	"github.com/enterprise-digital-platform/api-gateway/internal/config"
	"github.com/enterprise-digital-platform/api-gateway/internal/gateway"
	"github.com/enterprise-digital-platform/api-gateway/internal/logging"
)

func main() {
	logging.Init("api-gateway")
	cfg := config.Load()

	handler := gateway.New(cfg)

	log.Printf("api-gateway listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatal(err)
	}
}
