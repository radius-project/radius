// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import "go.opentelemetry.io/otel/exporters/metric/prometheus"

// NewPrometheusMetricsExporter returns prometheus exporter used for metrics collection
func NewPrometheusMetricsExporter() (*prometheus.Exporter, error) {
	promConfig := prometheus.Config{}
	exporter, err := prometheus.InstallNewPipeline(promConfig)
	if err != nil {
		return nil, err
	}
	return exporter, nil
}
