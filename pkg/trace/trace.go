/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package trace

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

// Service implements the hosting.Service interface for the tracer.
type Service struct {
	Options Options
}

// Name gets the service name.
func (s *Service) Name() string {
	return "tracer"
}

// Run runs the tracer service.
func (s *Service) Run(ctx context.Context) error {
	shutdown, err := InitTracer(s.Options)
	if err != nil {
		return err
	}

	<-ctx.Done()
	return shutdown(ctx)
}

// InitTracer sets up a tracer provider with a sampler and resource attributes, and optionally registers a Zipkin exporter
// and batcher. It returns a shutdown function and an error if one occurs.
func InitTracer(opts Options) (func(context.Context) error, error) {
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
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
