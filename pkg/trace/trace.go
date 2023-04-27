// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package trace

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

// InitTracer initializes a new and configured TracerProvider.
func InitTracer(opts Options) (func(context.Context) error, error) {
	tp := sdktrace.NewTracerProvider(
		trace.WithSampler(trace.TraceIDRatioBased(0.5)),
		// sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(opts.ServiceName),
		)),
	)
	if opts.Zipkin != nil {
		exporter, err := zipkin.New(opts.Zipkin.URL)
		if err != nil {
			return nil, err
		}
		batcher := sdktrace.NewBatchSpanProcessor(exporter)
		tp.RegisterSpanProcessor(batcher)

	}
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}))
	return tp.Shutdown, nil
}
