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

// # Function Explanation
//
// StartProducerSpan creates a new span with SpanKindProducer for enqueuing async operations. It creates the span
// with the given spanName and tracerName, and adds the given attributes.
func StartProducerSpan(ctx context.Context, spanName string, tracerName string) (context.Context, trace.Span) {
	attr := []attribute.KeyValue{
		{Key: semconv.MessagingSystemKey, Value: attribute.StringValue("radius-internal")},
		{Key: semconv.MessagingOperationKey, Value: attribute.StringValue("publish")},
	}
	return StartCustomSpan(ctx, spanName, tracerName, attr, trace.WithSpanKind(trace.SpanKindProducer))
}

// # Function Explanation
//
// StartConsumerSpan creates a new span with SpanKindConsumer for dequeing async operations. It creates the span
// with the given spanName and tracerName, and adds the given attributes.
func StartConsumerSpan(ctx context.Context, spanName string, tracerName string) (context.Context, trace.Span) {
	attr := []attribute.KeyValue{
		{Key: semconv.MessagingSystemKey, Value: attribute.StringValue("radius-internal")},
		{Key: semconv.MessagingOperationKey, Value: attribute.StringValue("receive")},
	}
	return StartCustomSpan(ctx, spanName, tracerName, attr, trace.WithSpanKind(trace.SpanKindConsumer))
}

// # Function Explanation
//
// StartCustomSpan creates a new span with the given name, tracer name and attributes and returns a context and the span.
func StartCustomSpan(ctx context.Context, spanName string, tracerName string, attrs []attribute.KeyValue, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tr := otel.GetTracerProvider().Tracer(tracerName)
	ctx, span := tr.Start(ctx, spanName, opts...)
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	return ctx, span
}

// # Function Explanation
//
// SetAsyncResultStatus sets the status of the span based on the result and adds an exception event if the result contains
// an error.
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

// # Function Explanation
//
// ExtractTraceparent extracts the traceparent header from the context.
// Retrieve the current span context from context and serialize it to its w3c string representation using propagator.
// ref: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/semantic_conventions/messaging.md
func ExtractTraceparent(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier[traceparentHeaderKey]
}

// # Function Explanation
//
// WithTraceparent creates a new context with the given traceparent string.
func WithTraceparent(ctx context.Context, traceparent string) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier{traceparentHeaderKey: traceparent})
}
