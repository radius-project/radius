// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/metric/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/unit"
)

const (
	timeMetricsName   = "request_duration"
	requestMetricName = "request_counts"
	coreRPMeterName   = "rad-coreRP"
)

var _ MetricsProvider = (*PrometheusMetricsProvider)(nil)

type PrometheusMetricsProvider struct {
	RequestCounter  metric.Int64Counter
	LatencyRecorder metric.Int64ValueRecorder
}

func NewPrometheusMetricsProvider() *PrometheusMetricsProvider {
	return &PrometheusMetricsProvider{
		RequestCounter:  metric.Must(global.GetMeterProvider().Meter(coreRPMeterName)).NewInt64Counter(requestMetricName, metric.WithUnit(unit.Dimensionless)),
		LatencyRecorder: metric.Must(global.GetMeterProvider().Meter(coreRPMeterName)).NewInt64ValueRecorder(timeMetricsName, metric.WithUnit(unit.Milliseconds)),
	}

}

func (p *PrometheusMetricsProvider) IncrementRequestCount(ctx context.Context, val int, labels ...attribute.KeyValue) {
	p.RequestCounter.Add(ctx, int64(val), labels...)
}

func (p *PrometheusMetricsProvider) RecordLatency(ctx context.Context, val int, labels ...attribute.KeyValue) {
	p.LatencyRecorder.Record(ctx, int64(val), labels...)
}

func (p *PrometheusMetricsProvider) MetricsMiddleware(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		requestStartTime := time.Now()
		wi := &responseWriterInterceptor{
			statusCode:     http.StatusOK,
			ResponseWriter: w,
		}

		h.ServeHTTP(wi, r)

		// ignore errors as we don't want to fail a request because of parsing failures for resource type on a request
		resourceType, _ := azresources.Parse(r.URL.Path)
		elapsedTime := time.Since(requestStartTime).Microseconds()
		labels := []attribute.KeyValue{attribute.String("Path", r.URL.Path), attribute.String("Method", r.Method), attribute.String("statusCode", strconv.Itoa(wi.statusCode)),
			attribute.String("responseCode", http.StatusText(wi.statusCode)), attribute.String("resourceType", resourceType.ID)}
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

// NewPrometheusMetricsExporter returns prometheus exporter used for metrics collection
func NewPrometheusMetricsExporter() (*prometheus.Exporter, error) {
	promConfig := prometheus.Config{}
	exporter, err := prometheus.InstallNewPipeline(promConfig)
	if err != nil {
		return nil, err
	}
	return exporter, nil
}
