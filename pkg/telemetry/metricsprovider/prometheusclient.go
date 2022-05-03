// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package metricsprovider

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/metric/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/global"
	export "go.opentelemetry.io/otel/sdk/export/metric"
	"go.opentelemetry.io/otel/sdk/metric/aggregator/histogram"
	controller "go.opentelemetry.io/otel/sdk/metric/controller/basic"
	processor "go.opentelemetry.io/otel/sdk/metric/processor/basic"
	selector "go.opentelemetry.io/otel/sdk/metric/selector/simple"
	"go.opentelemetry.io/otel/unit"
)

var _ MetricsClient = (*PrometheusMetricsClient)(nil)

type PrometheusMetricsClient struct {
	Client *prometheus.Exporter
}

func NewPrometheusMetricsClient() (*PrometheusMetricsClient, error) {
	promConfig := prometheus.Config{}
	c := controller.New(
		processor.New(
			selector.NewWithHistogramDistribution(
				histogram.WithExplicitBoundaries(promConfig.DefaultHistogramBoundaries),
			),
			export.CumulativeExportKindSelector(),
		),
	)
	exporter, err := prometheus.NewExporter(promConfig, c)
	if err != nil {
		return nil, err
	}

	global.SetMeterProvider(exporter.MeterProvider())
	return &PrometheusMetricsClient{Client: exporter}, nil
}

func (p *PrometheusMetricsClient) Add(ctx context.Context, val int, metricName string, labels ...attribute.KeyValue) {
	getMeterMust().NewInt64Counter(metricName).Add(ctx, int64(val), labels...)
}

func (p *PrometheusMetricsClient) Observe(ctx context.Context, val float64, metricName string, metricUnit unit.Unit, labels ...attribute.KeyValue) {
	callback := func(v float64) metric.Float64ObserverFunc {
		return metric.Float64ObserverFunc(func(_ context.Context, result metric.Float64ObserverResult) { result.Observe(v, labels...) })
	}(float64(val))
	getMeterMust().NewFloat64ValueObserver(metricName, callback, metric.WithUnit(metricUnit)).Observation(val)
}

func getMeterMust() metric.MeterMust {
	meter := global.GetMeterProvider().Meter("radius-rp")
	return metric.Must(meter)
}
