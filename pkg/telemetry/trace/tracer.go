// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package trace

import (
	"context"
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

func InitServerTracer(url string, serviceName string) (func(context.Context) error, error) {
	//Assume zipkin for now
	exporter, err := zipkin.New(
		url,
	)
	if err != nil || exporter == nil {
		return nil, err
	}
	batcher := sdktrace.NewBatchSpanProcessor(exporter)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(batcher),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		)),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

func InitTracer(opts TracerOptions) (func(context.Context) error, error) {
	var exporter *zipkin.Exporter
	var batcher sdktrace.SpanProcessor

	exporter, err := zipkin.New(
		opts.URL,
	)
	if err != nil || exporter == nil {
		return nil, err
	}
	batcher = sdktrace.NewBatchSpanProcessor(exporter)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(batcher),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(opts.ServiceName),
		)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp.Shutdown, nil
}

func ShutdownTracer(shutdown func(context.Context) error, ctx context.Context) {
	err := shutdown(ctx)
	if err != nil {
		log.Fatal("failed to shutdown TracerProvider: %w", err)
	}
}
