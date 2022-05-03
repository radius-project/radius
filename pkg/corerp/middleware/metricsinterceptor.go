// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/unit"
)

// LowercaseURLPath is the middelware to lowercase the incoming request url path.
func MetricsInterceptor(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		requestStartTime := time.Now()
		wi := &responseWriterInterceptor{
			statusCode:     http.StatusOK,
			ResponseWriter: w,
		}

		h.ServeHTTP(wi, r)

		timeMetricsName := mux.CurrentRoute(r).GetName() + "_" + r.Method + "_time"
		requestMetricName := mux.CurrentRoute(r).GetName() + "_" + r.Method + "_requests" + "_" + strconv.Itoa(wi.statusCode)
		elapsedTime := time.Since(requestStartTime).Microseconds()
		metric.Must(global.GetMeterProvider().Meter("rad-core-rp")).NewInt64Counter(requestMetricName, metric.WithUnit(unit.Dimensionless)).Add(r.Context(), int64(1))
		metric.Must(global.GetMeterProvider().Meter("rad-core-rp")).NewInt64ValueRecorder(timeMetricsName, metric.WithUnit(unit.Milliseconds)).Record(r.Context(), int64(elapsedTime))
	}
	return http.HandlerFunc(fn)
}

// responseWriterInterceptor is a simple wrapper to intercept set data on a
// ResponseWriter.
type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriterInterceptor) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterInterceptor) Write(p []byte) (int, error) {
	return w.ResponseWriter.Write(p)
}

func (w *responseWriterInterceptor) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("type assertion failed http.ResponseWriter not a http.Hijacker")
	}
	return h.Hijack()
}

func (w *responseWriterInterceptor) Flush() {
	f, ok := w.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}

	f.Flush()
}
