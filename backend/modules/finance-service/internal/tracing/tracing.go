package tracing

import (
	"context"
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Init wires a TracerProvider exporting every span (100% sampling, dev-only)
// via OTLP/HTTP to Jaeger. Returns a shutdown func to defer in main() so
// buffered spans flush before the process exits. Exporter init failure is
// non-fatal (same best-effort posture as this service's Kafka/MQTT
// connections) -- tracing is diagnostic, never a hard dependency.
func Init(ctx context.Context, service, otlpEndpoint string) func(context.Context) error {
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(otlpEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		log.Printf("tracing: otlp exporter init failed, spans will not be exported: %v", err)
		return func(context.Context) error { return nil }
	}

	res, _ := resource.Merge(resource.Default(),
		resource.NewSchemaless(semconv.ServiceNameKey.String(service)))

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return tp.Shutdown
}
