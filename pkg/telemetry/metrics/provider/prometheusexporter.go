// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/view"
)

// NewPrometheusMetricsExporter returns prometheus exporter used for metrics collection
func NewPrometheusMetricsExporter() (*metric.MeterProvider, http.Handler, error) {
	exporter := otelprom.New()
	registry := prometheus.NewRegistry()
	if err := registry.Register(exporter.Collector); err != nil {
		return nil, nil, err
	}

	defaultView, err := view.New(view.MatchInstrumentName("*"))
	if err != nil {
		return nil, nil, err
	}

	provider := metric.NewMeterProvider(metric.WithReader(exporter, defaultView))

	return provider, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}), nil
}
