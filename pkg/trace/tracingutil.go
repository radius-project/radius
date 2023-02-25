// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package trace

import (
	"context"

	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	UCPTracerName    string = "ucp-tracer"
	RPFrontendTracer string = "rp-frontend-tracer"
	RPBackendTracer  string = "rp-backend-tracer"

	TraceparentHeader string = "traceparent"
)

// AddProducerSpan adds span to enqueuing async operations.
func AddProducerSpan(ctx context.Context, spanName string, tracerName string) (context.Context, trace.Span) {
	attr := []attribute.KeyValue{
		attribute.String(string(semconv.MessagingSystemKey), "radius-internal"),
		attribute.String(string(semconv.MessagingOperationKey), "publish"),
	}
	return StartCustomSpan(ctx, spanName, tracerName, attr, trace.WithSpanKind(trace.SpanKindProducer))
}

// AddConsumerTelemtryData adds span data to dequeing async operations.
func AddConsumerSpan(ctx context.Context, spanName string, tracerName string) (context.Context, trace.Span) {
	attr := []attribute.KeyValue{
		attribute.String(string(semconv.MessagingSystemKey), "radius-internal"),
		attribute.String(string(semconv.MessagingOperationKey), "receive"),
	}
	return StartCustomSpan(ctx, spanName, tracerName, attr, trace.WithSpanKind(trace.SpanKindConsumer))
}

// StartCustomSpan starts a custom span based on opts
func StartCustomSpan(ctx context.Context, spanName string, tracerName string, attrs []attribute.KeyValue, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tr := otel.GetTracerProvider().Tracer(tracerName)
	ctx, span := tr.Start(ctx, spanName, opts...)
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	return ctx, span
}

func RecordAsyncResult(result ctrl.Result, span trace.Span) {
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
