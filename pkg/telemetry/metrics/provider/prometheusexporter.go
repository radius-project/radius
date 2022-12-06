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

// PrometheusExporter is the struct that holds the metrics reklated data
type PrometheusExporter struct {
	// MeterProvider is used in the creation and coordination of Meters
	MeterProvider *metric.MeterProvider

	// Handler is the HTTP handler with basic metrics
	Handler http.Handler
}

// NewPrometheusExporter builds and returns prometheus exporter used for metrics collection
func NewPrometheusExporter() (*PrometheusExporter, error) {
	exporter := otelprom.New()
	registry := prometheus.NewRegistry()
	if err := registry.Register(exporter.Collector); err != nil {
		return nil, err
	}

	defaultView, err := view.New(view.MatchInstrumentName("*"))
	if err != nil {
		return nil, err
	}

	provider := metric.NewMeterProvider(metric.WithReader(exporter, defaultView))

	return &PrometheusExporter{
		MeterProvider: provider,
		Handler:       promhttp.Handler(),
	}, nil
}
