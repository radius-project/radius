// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package metrics

import (
	"go.opentelemetry.io/otel/exporters/metric/prometheus"
)

type MetricsProvider interface{
	GetExporter()(*prometheus.Exporter)
}