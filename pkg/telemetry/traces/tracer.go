// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package traces

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

/*func InitCliTracer(serviceName string) (func(context.Context) error, error) {
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		)),
	)
	otel.SetTracerProvider(tp)
	// otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp.Shutdown, nil
} */

func InitTracer(opts TracerOptions) (func(context.Context) error, error) {
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(opts.ServiceName),
			semconv.ServiceNameKey.String(opts.URL),
		)),
	)

	var batcher sdktrace.SpanProcessor
	if opts.URL != "" {
		exporter, err := zipkin.New(
			opts.URL,
		)
		if err != nil || exporter == nil {
			return nil, err
		}
		batcher = sdktrace.NewBatchSpanProcessor(exporter)
		tp.RegisterSpanProcessor(batcher)
	}

	otel.SetTracerProvider(tp)
	// otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp.Shutdown, nil
}
