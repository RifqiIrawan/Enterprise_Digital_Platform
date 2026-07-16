package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
	"github.com/enterprise-digital-platform/dw-service/internal/config"
	"github.com/enterprise-digital-platform/dw-service/internal/httpapi"
	"github.com/enterprise-digital-platform/dw-service/internal/sourcedb"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	sources, err := sourcedb.Connect(ctx, cfg.FinanceDatabaseURL, cfg.SalesDatabaseURL, cfg.WarehouseDatabaseURL)
	if err != nil {
		log.Fatalf("dw-service: connect source databases failed: %v", err)
	}
	defer sources.Close()

	// ClickHouse adalah destinasi utama (bukan side-channel opsional seperti
	// MQTT di iot-service) -- tapi kegagalan konek di awal tetap tidak
	// log.Fatal supaya /health tetap hidup dan bisa diagnosa; ticker/endpoint
	// /sync akan terus gagal dengan error yang jelas sampai ClickHouse naik.
	dest, err := clickhouse.Connect(ctx, cfg.ClickHouseAddr, cfg.ClickHouseUser, cfg.ClickHousePassword, cfg.ClickHouseDatabase)
	if err != nil {
		log.Printf("dw-service: clickhouse connect failed, /sync will fail until it's reachable: %v", err)
	} else if err := dest.EnsureSchema(ctx); err != nil {
		log.Printf("dw-service: clickhouse schema setup failed: %v", err)
	}

	handler := httpapi.NewHandler(sources, dest)

	mux := http.NewServeMux()
	handler.Register(mux)

	if cfg.SyncEnabled && dest != nil {
		go runTicker(ctx, sources, dest, time.Duration(cfg.SyncIntervalSeconds)*time.Second)
	}

	log.Printf("dw-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		log.Fatal(err)
	}
}

func runTicker(ctx context.Context, sources *sourcedb.Pools, dest *clickhouse.Client, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		results := httpapi.RunSync(ctx, sources, dest)
		for _, r := range results {
			if r.Error != "" {
				log.Printf("dw-service: sync %s failed: %s", r.Fact, r.Error)
			} else if r.Rows > 0 {
				log.Printf("dw-service: synced %d rows into %s", r.Rows, r.Fact)
			}
		}
	}
}
