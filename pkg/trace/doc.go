// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

/*
Package trace includes the distributed trace utilities to help initialize the trace provider, create traces and spans.

# Basic

Radius uses opentelemetry SDK to enable distributed tracing. We wrap Radius handlers with otelhttp handlers for generating http traces.
In addition, trace package introduces the below helpers:

* InitTracer initializes a new and configured Tracer.
* StartCustomSpan(ctx, spanName, tracerName, attr, spanKind) starts a new span with the given names and attributes.
* StartProducerSpan(ctx, spanName, tracerName) starts a new Producer span with the given names.
* StartConsumerSpan(ctx, spanName, tracerName) starts a new Consumer span with the given names.


# Examples

Initializing a new Tracer:
shutdown,err := trace.InitTracer(trace.Options{ServiceName: serviceName})
if err != nil {
	...
}

Adding new internal span:

func functionName(ctx context.Context) {
	attr := []attribute.KeyValue{
			// Add otel attributes of interest here.
	}
	ctx, span := StartCustomSpan(ctx, spanName, tracerName, attr, trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	...
}

Adding new producer span:

func functionName(ctx context.Context) {
	ctx, span := StartProducerSpan(ctx, spanName, tracerName)
	defer span.End()
	...
}

Adding new consumer span:

func functionName(ctx context.Context) {
	ctx, span := StartConsumerSpan(ctx, spanName, tracerName)
	defer span.End()
	...
}

# References

1. https://opentelemetry.io/docs/reference/specification/trace
2. https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/net/http/otelhttp
3. https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/semantic_conventions/http.md
4. https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/semantic_conventions/messaging.md

*/

package trace
