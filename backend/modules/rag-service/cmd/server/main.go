package main

import (
	"context"
	"log"
	"net/http"
	"strconv"

	"github.com/enterprise-digital-platform/rag-service/internal/config"
	"github.com/enterprise-digital-platform/rag-service/internal/httpapi"
	"github.com/enterprise-digital-platform/rag-service/internal/llm"
	"github.com/enterprise-digital-platform/rag-service/internal/logging"
	"github.com/enterprise-digital-platform/rag-service/internal/metrics"
	"github.com/enterprise-digital-platform/rag-service/internal/requestid"
	"github.com/enterprise-digital-platform/rag-service/internal/store"
	"github.com/enterprise-digital-platform/rag-service/internal/tracing"
	"github.com/enterprise-digital-platform/rag-service/migrations"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	logging.Init("rag-service")
	cfg := config.Load()
	ctx := context.Background()

	shutdownTracing := tracing.Init(ctx, "rag-service", cfg.OTLPEndpoint)
	defer shutdownTracing(context.Background())

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("rag-service: db connect failed: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("rag-service: migration failed: %v", err)
	}

	// Tidak adanya kunci API BUKAN alasan untuk mati. Pencarian dokumentasi
	// tetap berguna tanpa model; yang hilang hanya jawaban naratifnya. Yang
	// dicatat di sini adalah satu baris log yang menjelaskan keadaannya,
	// supaya "kenapa chatbotnya cuma mengeluarkan kutipan" punya jawaban di
	// tempat pertama yang dilihat orang.
	var answerer llm.Answerer
	if claude := llm.NewClaude(cfg.AnthropicAPIKey, cfg.Model, cfg.Effort); claude != nil {
		answerer = claude
		log.Printf("rag-service: penyedia jawaban aktif (model %s, effort %s)", cfg.Model, cfg.Effort)
	} else {
		log.Printf("rag-service: ANTHROPIC_API_KEY kosong -- /ask akan mengembalikan kutipan dokumentasi tanpa jawaban model")
	}

	topK, err := strconv.Atoi(cfg.TopK)
	if err != nil || topK <= 0 {
		log.Printf("rag-service: RAG_TOP_K tidak valid (%q), memakai 5", cfg.TopK)
		topK = 5
	}

	handler := httpapi.NewHandler(pool, answerer, cfg.CorpusDir, topK)

	mux := http.NewServeMux()
	handler.Register(mux)

	var topHandler http.Handler = metrics.Middleware(mux)
	topHandler = requestid.Middleware(topHandler)
	topHandler = otelhttp.NewHandler(topHandler, "rag-service")

	log.Printf("rag-service listening on :%s (korpus: %s)", cfg.Port, cfg.CorpusDir)
	if err := http.ListenAndServe(":"+cfg.Port, topHandler); err != nil {
		log.Fatal(err)
	}
}
