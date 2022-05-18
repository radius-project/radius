// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package metrics

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/unit"
)

const (
	timeMetricsName   = "corerp_request_duration"
	requestMetricName = "corerp_request_counts"
	coreRPMeterName   = "rad-coreRP"
)

type HTTPMetrics struct {
	RequestCounter  metric.Int64Counter
	LatencyRecorder metric.Int64ValueRecorder
}

func NewHTTPMetrics() *HTTPMetrics {
	return &HTTPMetrics{
		RequestCounter:  metric.Must(global.GetMeterProvider().Meter(coreRPMeterName)).NewInt64Counter(requestMetricName, metric.WithUnit(unit.Dimensionless)),
		LatencyRecorder: metric.Must(global.GetMeterProvider().Meter(coreRPMeterName)).NewInt64ValueRecorder(timeMetricsName, metric.WithUnit(unit.Milliseconds)),
	}

}

func (p *HTTPMetrics) IncrementRequestCount(ctx context.Context, val int, labels ...attribute.KeyValue) {
	p.RequestCounter.Add(ctx, int64(val), labels...)
}

func (p *HTTPMetrics) RecordLatency(ctx context.Context, val int64, labels ...attribute.KeyValue) {
	p.LatencyRecorder.Record(ctx, int64(val), labels...)
}
