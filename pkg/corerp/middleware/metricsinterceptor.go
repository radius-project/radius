// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/unit"
)

const (
	timeMetricsName   = "request_duration"
	requestMetricName = "request_counts"
)

//MetricsInterceptor intercepts every http request to core rp and emits number of requests and time for a request metrics.
func MetricsInterceptor(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		requestStartTime := time.Now()
		wi := &responseWriterInterceptor{
			statusCode:     http.StatusOK,
			ResponseWriter: w,
		}

		h.ServeHTTP(wi, r)

		elapsedTime := time.Since(requestStartTime).Microseconds()
		labels := []attribute.KeyValue{attribute.String("Path", r.RequestURI), attribute.String("Method", r.Method), attribute.String("responseCode", strconv.Itoa(wi.statusCode)), attribute.String("APIName", mux.CurrentRoute(r).GetName())}
		metric.Must(global.GetMeterProvider().Meter("rad-core-rp")).NewInt64Counter(requestMetricName, metric.WithUnit(unit.Dimensionless)).Add(r.Context(), int64(1), labels...)
		metric.Must(global.GetMeterProvider().Meter("rad-core-rp")).NewInt64ValueRecorder(timeMetricsName, metric.WithUnit(unit.Milliseconds)).Record(r.Context(), int64(elapsedTime), labels...)
	}
	return http.HandlerFunc(fn)
}

// responseWriterInterceptor is a simple wrapper to intercept the statusCode needed for metrics attributes
// default response writer doesn't provide the status code of the response
type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode int
}

//Customized response writer to fetch the response status in the middleware
func (w *responseWriterInterceptor) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterInterceptor) Write(p []byte) (int, error) {
	return w.ResponseWriter.Write(p)
}
