// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package trace

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

type Options struct {
	TracerOptions
}

func InitTracer(opts TracerOptions) (func(context.Context) error, error) {
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(opts.ServiceName),
		)),
	)

	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

func InitServerTracer(url string, serviceName string) (func(context.Context) error, error) {
	//Assume zipkin for now
	exporter, err := zipkin.New(
		url,
	)
	if err != nil || exporter == nil {
		return nil, err
	}

	exporter1, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil || exporter == nil {
		return nil, err
	}
	batcher := sdktrace.NewBatchSpanProcessor(exporter)
	batcher1 := sdktrace.NewBatchSpanProcessor(exporter1)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(batcher),
		sdktrace.WithSpanProcessor(batcher1),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp.Shutdown, nil
}
