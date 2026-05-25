package observability

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// InitTracer configures the global OpenTelemetry TracerProvider and propagator.
//
// When OTEL_EXPORTER_OTLP_ENDPOINT is set, traces are exported via OTLP/HTTP.
// Otherwise a noop exporter is used — zero performance overhead, nothing emitted.
//
// sampleRate controls the fraction of traces sampled (0.0–1.0). 1.0 = always sample
// (good for development), 0.1 = 10% (typical for high-traffic production).
//
// The returned shutdown function flushes pending spans and must be called before
// the process exits (typically via defer in main).
func InitTracer(ctx context.Context, serviceName string, sampleRate float64) (func(context.Context) error, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
		resource.WithProcess(),
		resource.WithOS(),
	)
	if err != nil {
		return nil, fmt.Errorf("build otel resource: %w", err)
	}

	var exp sdktrace.SpanExporter
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		exp, err = otlptracehttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("create otlp exporter: %w", err)
		}
	} else {
		exp = noopExporter{}
	}

	var sampler sdktrace.Sampler
	if sampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else {
		sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRate))
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, // W3C Trace Context
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// noopExporter discards all spans — used when no OTLP endpoint is configured.
type noopExporter struct{}

func (noopExporter) ExportSpans(_ context.Context, _ []sdktrace.ReadOnlySpan) error { return nil }
func (noopExporter) Shutdown(_ context.Context) error                                { return nil }
