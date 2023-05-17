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

	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	// FrontendTracerName represents the name of frontend tracer name.
	FrontendTracerName string = "radius-frontend-tracer"
	// BackendTracerName represents the name of backend tracer name.
	BackendTracerName string = "radius-backend-tracer"

	traceparentHeaderKey string = "traceparent"
)

// StartProducerSpan adds span to enqueuing async operations.
func StartProducerSpan(ctx context.Context, spanName string, tracerName string) (context.Context, trace.Span) {
	attr := []attribute.KeyValue{
		{Key: semconv.MessagingSystemKey, Value: attribute.StringValue("radius-internal")},
		{Key: semconv.MessagingOperationKey, Value: attribute.StringValue("publish")},
	}
	return StartCustomSpan(ctx, spanName, tracerName, attr, trace.WithSpanKind(trace.SpanKindProducer))
}

// StartConsumerSpan adds span data to dequeing async operations.
func StartConsumerSpan(ctx context.Context, spanName string, tracerName string) (context.Context, trace.Span) {
	attr := []attribute.KeyValue{
		{Key: semconv.MessagingSystemKey, Value: attribute.StringValue("radius-internal")},
		{Key: semconv.MessagingOperationKey, Value: attribute.StringValue("receive")},
	}
	return StartCustomSpan(ctx, spanName, tracerName, attr, trace.WithSpanKind(trace.SpanKindConsumer))
}

// StartCustomSpan starts a custom span based on opts.
func StartCustomSpan(ctx context.Context, spanName string, tracerName string, attrs []attribute.KeyValue, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tr := otel.GetTracerProvider().Tracer(tracerName)
	ctx, span := tr.Start(ctx, spanName, opts...)
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	return ctx, span
}

// SetAsyncResultStatus sets Status of Span.
func SetAsyncResultStatus(result ctrl.Result, span trace.Span) {
	if span == nil || !span.IsRecording() {
		return
	}
	if result.Error != nil {
		span.SetStatus(otelcodes.Error, result.Error.Message)
		opts := trace.WithAttributes(
			semconv.ExceptionType(result.Error.Code),
			semconv.ExceptionMessage(result.Error.Message),
		)
		span.AddEvent(semconv.ExceptionEventName, opts)
	} else {
		span.SetStatus(otelcodes.Ok, string(result.ProvisioningState()))
	}
}

// ExtractTraceparent extracts traceparent from context.
// Retrieve the current span context from context and serialize it to its w3c string representation using propagator.
// ref: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/semantic_conventions/messaging.md
func ExtractTraceparent(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier[traceparentHeaderKey]
}

// WithTraceparent returns the context with tracespan.
func WithTraceparent(ctx context.Context, traceparent string) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier{traceparentHeaderKey: traceparent})
}
