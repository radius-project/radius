// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric/global"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// PrometheusExporter is the struct that holds the metrics reklated data
type PrometheusExporter struct {
	// MeterProvider is used in the creation and coordination of Meters
	MeterProvider *sdkmetric.MeterProvider

	// Handler is the HTTP handler with basic metrics
	Handler http.Handler
}

// NewPrometheusExporter builds and returns prometheus exporter used for metrics collection
func NewPrometheusExporter() (*PrometheusExporter, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	global.SetMeterProvider(mp)

	return &PrometheusExporter{
		MeterProvider: mp,
		Handler:       promhttp.Handler(),
	}, nil
}
