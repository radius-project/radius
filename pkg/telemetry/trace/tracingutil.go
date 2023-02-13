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
	TracestateHeader  string = "tracestate"

	MessagingSystem    string = "messaging.system"
	MessagingOperation string = "messaging.operation"
)

// AddProducerSpan adds span to enqueuing async operations.
func AddProducerSpan(ctx context.Context, spanName string, tracerName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tp := otel.GetTracerProvider()
	tr := tp.Tracer(tracerName)
	ctx, span := tr.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindProducer))
	producerAttributes := map[string]string{}
	producerAttributes[MessagingSystem] = "radius"
	producerAttributes[MessagingOperation] = "publish"

	var attrs []attribute.KeyValue
	for k, v := range producerAttributes {
		attrs = append(attrs, attribute.String(k, v))
	}

	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}

	return ctx, span
}

// AddConsumerTelemtryData adds span data to dequeing async operations.
func AddConsumerSpan(ctx context.Context, spanName string, tracerName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tp := otel.GetTracerProvider()
	tr := tp.Tracer(tracerName)
	ctx, span := tr.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindConsumer))
	consumerAttributes := map[string]string{}
	consumerAttributes[MessagingSystem] = "radius"
	consumerAttributes[MessagingOperation] = "receive"

	var attrs []attribute.KeyValue
	for k, v := range consumerAttributes {
		attrs = append(attrs, attribute.String(k, v))
	}

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
