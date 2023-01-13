// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"

	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"go.opentelemetry.io/otel/trace"
)

// MetricsRecorder is the middleware which collects metrics for incoming server requests.

func WithRouteTag(route string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := trace.SpanFromContext(r.Context())
		span.SetAttributes(semconv.HTTPRouteKey.String(route))
		h.ServeHTTP(w, r)
	})
}
