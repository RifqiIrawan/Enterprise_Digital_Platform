package config

import "os"

type Config struct {
	Port         string
	DatabaseURL  string
	KafkaBrokers string
	OTLPEndpoint string
	// CorpusDir menunjuk folder dokumentasi yang di-index. Dibuat env, bukan
	// konstanta, karena di container foldernya harus di-mount (lihat
	// docker-compose) -- dan karena repo ini punya DUA folder dokumentasi,
	// yang satu tinggalan lama. Menjadikannya konfigurasi membuat pilihan itu
	// terlihat alih-alih terkubur di dalam kode.
	CorpusDir string
	// AnthropicAPIKey boleh kosong: tanpa kunci, /ask tetap mencari dan
	// mengembalikan kutipan, hanya tidak menyusun jawaban naratif.
	AnthropicAPIKey string
	Model           string
	// Effort mengikuti output_config.effort Claude ("low".."max"). Default
	// "low": tugas di sini menyusun jawaban dari kutipan yang sudah disodorkan,
	// bukan memecahkan masalah -- dan ini jalur chat yang dipakai berulang.
	Effort string
	// TopK: berapa potongan dokumentasi yang dikirim sebagai bahan. Lima cukup
	// untuk pertanyaan yang jawabannya tersebar di beberapa bagian, dan masih
	// murah; menaikkannya menambah biaya input tiap pertanyaan.
	TopK string
}

func Load() *Config {
	return &Config{
		Port:            getEnv("PORT", "8101"),
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://platform:platform@localhost:5432/rag_service?sslmode=disable"),
		KafkaBrokers:    getEnv("KAFKA_BROKERS", "localhost:9092"),
		OTLPEndpoint:    getEnv("OTLP_ENDPOINT", "localhost:4318"),
		CorpusDir:       getEnv("RAG_CORPUS_DIR", "../../../dokumentasi"),
		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
		Model:           getEnv("RAG_MODEL", "claude-opus-5"),
		Effort:          getEnv("RAG_EFFORT", "low"),
		TopK:            getEnv("RAG_TOP_K", "5"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
