// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/metric"
)

// MetricsRecorder is the middleware which collects metrics for incoming server requests.
func MetricsRecorder() func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		// FIXME: Not sure what the operation should be. The second parameter in the NewHandler call.
		return otelhttp.NewHandler(h, "Radius",
			// FIXME
			otelhttp.WithMeterProvider(metric.NewNoopMeterProvider()))
	}
}

// // responseWriterInterceptor is a simple wrapper to intercept the statusCode needed for metrics attributes
// // default response writer doesn't provide the status code of the response
// type responseWriterInterceptor struct {
// 	http.ResponseWriter
// 	statusCode int
// }

// // Customized response writer to fetch the response status in the middleware
// func (w *responseWriterInterceptor) WriteHeader(statusCode int) {
// 	w.statusCode = statusCode
// 	w.ResponseWriter.WriteHeader(statusCode)
// }

// func (w *responseWriterInterceptor) Write(p []byte) (int, error) {
// 	return w.ResponseWriter.Write(p)
// }
