// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"go.opentelemetry.io/otel/exporters/metric/prometheus"
)

// MetricsProvider is an interface defining to publish metrics.
type MetricsProvider interface{
	//GetExporter should return an exporter which is used to collect metrics from the metrics endpoint of the server.
	GetExporter()(*prometheus.Exporter)
}