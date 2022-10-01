// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package metrics

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/metric/instrument"
	"go.opentelemetry.io/otel/metric/instrument/syncint64"
	"go.opentelemetry.io/otel/metric/unit"
)

const (
	timeMetricsName   = "corerp_request_duration"
	requestMetricName = "corerp_request_counts"
	coreRPMeterName   = "rad-coreRP"
)

type HTTPMetrics struct {
	RequestCounter  syncint64.Counter
	LatencyRecorder syncint64.Histogram
}

// NewHTTPMetrics creates new otel instruments to record metrics.
func NewHTTPMetrics() (*HTTPMetrics, error) {
	var err error

	hm := &HTTPMetrics{}
	corerpmeter := global.Meter(coreRPMeterName)
	hm.RequestCounter, err = corerpmeter.SyncInt64().Counter(requestMetricName, instrument.WithUnit(unit.Dimensionless))
	if err != nil {
		return nil, err
	}
	hm.LatencyRecorder, err = corerpmeter.SyncInt64().Histogram(timeMetricsName, instrument.WithUnit(unit.Milliseconds))
	if err != nil {
		return nil, err
	}

	return hm, nil

}

// IncrementRequestCount increments the count metric for the given labels.
func (p *HTTPMetrics) IncrementRequestCount(ctx context.Context, val int, labels ...attribute.KeyValue) {
	p.RequestCounter.Add(ctx, int64(val), labels...)
}

// RecordLatency registers the value provided as the latency metric for the given labels.
func (p *HTTPMetrics) RecordLatency(ctx context.Context, val int64, labels ...attribute.KeyValue) {
	p.LatencyRecorder.Record(ctx, int64(val), labels...)
}
