// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package metricsprovider

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
)

type MetricsClient interface {
	RecordLatency(ctx context.Context, val int, metricName string, labels ...attribute.KeyValue)
	RecordRequests(ctx context.Context, val int, metricName string, labels ...attribute.KeyValue)

	// Provides an Async metric instrument which records values related to an event.
	Observe(ctx context.Context, val int, metricName string, labels ...attribute.KeyValue)
}
