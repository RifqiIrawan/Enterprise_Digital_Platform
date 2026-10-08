package streaming

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// eventsTotal menghitung hasil akhir tiap event: processed (sukses, mungkin
	// setelah retry), dead_lettered (gagal permanen, dikirim ke <topic>.dlq).
	eventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dw_streaming_events_total",
		Help: "Kafka events handled by dw-service streaming, by final outcome",
	}, []string{"topic", "outcome"})

	retriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dw_streaming_handler_retries_total",
		Help: "Handler attempts that failed and were retried",
	}, []string{"topic"})

	handlerDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "dw_streaming_handler_duration_seconds",
		Help:    "Time spent in the topic handler (Postgres lookup + ClickHouse insert), per attempt",
		Buckets: prometheus.DefBuckets,
	}, []string{"topic"})

	// eventLatency = waktu sejak event di-publish ke Kafka sampai selesai
	// diproses. Naik tajam kalau consumer tertinggal (lag).
	eventLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "dw_streaming_event_latency_seconds",
		Help:    "Seconds from Kafka message timestamp until the event was fully processed",
		Buckets: []float64{0.05, 0.1, 0.5, 1, 5, 15, 60, 300, 1800},
	}, []string{"topic"})
)
