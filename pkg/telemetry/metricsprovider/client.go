// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package metricsprovider

import (
	"context"
)

type MetricsClient interface {
	// Provides a synchronous metric instrument which supports additive metrics. ex: number of requests
	Add(ctx context.Context, val int, metricName string)
	// Provides a synchronous metric instrument which records values related to an event. ex: latency of an operation
	Measure(ctx context.Context, val float64, metricName string)
	// Async version of Meaure instrument.
	Observe(ctx context.Context, val float64, metricName string)
}
