// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"go.opentelemetry.io/otel/exporters/metric/prometheus"
	"go.opentelemetry.io/otel/metric/global"
	export "go.opentelemetry.io/otel/sdk/export/metric"
	"go.opentelemetry.io/otel/sdk/metric/aggregator/histogram"
	controller "go.opentelemetry.io/otel/sdk/metric/controller/basic"
	processor "go.opentelemetry.io/otel/sdk/metric/processor/basic"
	selector "go.opentelemetry.io/otel/sdk/metric/selector/simple"
)

var _ MetricsProvider = (*PrometheusMetricsProvider)(nil)

type PrometheusMetricsProvider struct {
	exporter *prometheus.Exporter
}

func NewPrometheusMetricsClient() (*PrometheusMetricsProvider, error) {
	exporter, err := initPrometheusExporter()
	if err != nil {
		return nil, err
	}
	global.SetMeterProvider(exporter.MeterProvider())
	return &PrometheusMetricsProvider{exporter: exporter}, nil
}

func initPrometheusExporter() (*prometheus.Exporter, error) {
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
	return exporter, nil
}

func (p *PrometheusMetricsProvider) GetExporter() *prometheus.Exporter {
	return p.exporter
}
