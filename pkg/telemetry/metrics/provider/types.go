// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
)

// MetricsProvider defines functions to publish metrics like requests and latency to the metrics endpoint.
type MetricsProvider interface {
	// IncrementRequestCount should increment the number of requests for request metric with the given labels/attributes.
	IncrementRequestCount(ctx context.Context, val int, labels ...attribute.KeyValue)
	// RecordLatency should record the latency of a request for latency metric with the given labels/attributes.
	RecordLatency(ctx context.Context, val int, labels ...attribute.KeyValue)
}
