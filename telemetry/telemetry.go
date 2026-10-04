// Package telemetry sets up OpenTelemetry tracing for the API and the runner.
// Spans go to Tempo over OTLP/HTTP when LEETFORCE_OTLP_ENDPOINT is set (for example
// http://127.0.0.1:4318, reached through scripts/obs-tunnel.sh); when it is empty
// tracing is off and every span is a no-op, so nothing changes for a plain run.
package telemetry

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// EndpointEnv names the OTLP/HTTP base URL of the collector (Tempo).
const EndpointEnv = "LEETFORCE_OTLP_ENDPOINT"

// Init installs the global tracer provider and W3C propagator for service
// ("api" or "runner", the same names the Loki "service" label uses). The returned
// function flushes and stops the exporter; it is safe to call when tracing is off.
func Init(ctx context.Context, service string) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	endpoint := os.Getenv(EndpointEnv)
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint+"/v1/traces"))
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}
	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(service)))
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(2*time.Second)),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}
