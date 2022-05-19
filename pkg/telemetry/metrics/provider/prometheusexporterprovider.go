// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import "go.opentelemetry.io/otel/exporters/metric/prometheus"

// NewPrometheusMetricsExporter returns prometheus exporter used for metrics collection
func NewPrometheusMetricsExporter(config prometheus.Config) (*prometheus.Exporter, error) {
	exporter, err := prometheus.InstallNewPipeline(config)
	if err != nil {
		return nil, err
	}
	return exporter, nil
}
