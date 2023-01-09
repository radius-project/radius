// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/project-radius/radius/pkg/telemetry/metrics"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"go.opentelemetry.io/otel/attribute"
)

// MetricsRecorder is the middleware which collects metrics for incoming server requests.
func MetricsRecorder(p *metrics.HTTPMetrics) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			requestStartTime := time.Now()
			wi := &responseWriterInterceptor{
				statusCode:     http.StatusOK,
				ResponseWriter: w,
			}

			h.ServeHTTP(wi, r)

			// ignore errors as we don't want to fail a request because of parsing failures for resource type on a request
			rid, _ := resources.Parse(r.URL.Path)
			elapsedTime := time.Since(requestStartTime).Microseconds()
			labels := []attribute.KeyValue{attribute.String("path", r.URL.Path), attribute.String("method", r.Method), attribute.String("statusCode", strconv.Itoa(wi.statusCode)),
				attribute.String("resourceType", rid.Type())}
			p.IncrementRequestCount(r.Context(), 1, labels...)
			p.RecordLatency(r.Context(), elapsedTime, labels...)
		}
		return http.HandlerFunc(fn)
	}
}

// responseWriterInterceptor is a simple wrapper to intercept the statusCode needed for metrics attributes
// default response writer doesn't provide the status code of the response
type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode int
}

// Customized response writer to fetch the response status in the middleware
func (w *responseWriterInterceptor) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterInterceptor) Write(p []byte) (int, error) {
	return w.ResponseWriter.Write(p)
}
