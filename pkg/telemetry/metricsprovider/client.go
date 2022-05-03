// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package metricsprovider

import (
	"context"

	"go.opentelemetry.io/otel/unit"
	"go.opentelemetry.io/otel/attribute"
)

type MetricsClient interface {
	// Provides a synchronous metric instrument which supports additive metrics. ex: number of requests
	Add(ctx context.Context, val int, metricName string, labels ...attribute.KeyValue)
	// Provides an Async metric instrument which records values related to an event. ex: latency of an operation
	Observe(ctx context.Context, val float64, metricName string, metricUnit unit.Unit, labels ...attribute.KeyValue)
}
