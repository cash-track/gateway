package traces

import (
	"context"
	"fmt"
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	TracerName    = "gateway"
	traceCtxKey   = "traceCtx"
	traceIdCtxKey = "traceIdCtx"
)

func GetTracer() trace.Tracer {
	return otel.Tracer(TracerName)
}

func NewTracer(ctx context.Context) (*sdktrace.TracerProvider, func(), error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("error initializing OpenTelemetry tracer: %w", err)
	}

	res, err := resource.New(
		ctx,
		resource.WithOS(),
		resource.WithFromEnv(),
		resource.WithContainer(),
		resource.WithHost(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("error initializing OpenTelemetry resource: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tracerProvider)

	return tracerProvider, func() {
		if err := tracerProvider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down OpenTelemetry tracer: %v\n", err)
		}
	}, nil
}
