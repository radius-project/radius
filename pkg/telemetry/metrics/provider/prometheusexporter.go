// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

// PrometheusExporter is the struct that holds the metrics reklated data
type PrometheusExporter struct {
	// MeterProvider is used in the creation and coordination of Meters
	MeterProvider otelmetric.MeterProvider

	// Handler is the HTTP handler with basic metrics
	Handler http.Handler
}

var prometheusExporter *PrometheusExporter
var once sync.Once

func GetPrometheusExporter() (*PrometheusExporter, error) {
	if prometheusExporter != nil {
		fmt.Println("PrometheusExporter already exists")
		return prometheusExporter, nil
	}

	var err error
	once.Do(func() {
		fmt.Println("Creating PrometheusExporter")
		prometheusExporter, err = NewPrometheusExporter()
	})

	return prometheusExporter, err
}

// NewPrometheusExporter builds and returns prometheus exporter used for metrics collection
func NewPrometheusExporter() (*PrometheusExporter, error) {
	registry := prometheus.NewRegistry()
	exporter, err := otelprom.New(otelprom.WithRegisterer(registry))
	if err != nil {
		return nil, err
	}

	res, err := resource.New(context.Background())

	if err != nil {
		return nil, err
	}

	provider := metric.NewMeterProvider(
		metric.WithReader(exporter),
		metric.WithResource(res),
	)

	global.SetMeterProvider(provider)
	fmt.Println("global meter provider set")

	return &PrometheusExporter{
		MeterProvider: global.MeterProvider(),
		Handler:       promhttp.Handler(),
	}, nil
}
