/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package metrics

import (
	"context"
	"strings"
	"time"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/ucp/resources"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	// AsyncOperationCount is the metric name for async operation count.
	AsyncOperationCount = "asyncoperation.operation"

	// QueuedAsyncOperationCount is the metric name for queued async operation count.
	QueuedAsyncOperationCount = "asyncoperation.queued.operation"

	// ExtendedAsyncOperationCount is the metric name for extended async operation count.
	ExtendedAsyncOperationCount = "asyncoperation.extended.operation"

	// AsyncOperationDuration is the metric name for async operation duration.
	AsnycOperationDuration = "asyncoperation.duration"
)

type asyncOperationMetrics struct {
	counters       map[string]metric.Int64Counter
	valueRecorders map[string]metric.Float64Histogram
}

func newAsyncOperationMetrics() *asyncOperationMetrics {
	return &asyncOperationMetrics{
		counters:       make(map[string]metric.Int64Counter),
		valueRecorders: make(map[string]metric.Float64Histogram),
	}
}

// Init initializes the counters and value recorders for asyncOperationMetrics and returns an error if any of the
// initialization fails.
func (a *asyncOperationMetrics) Init() error {
	meter := otel.GetMeterProvider().Meter("async-operation-metrics")

	var err error
	a.counters[QueuedAsyncOperationCount], err = meter.Int64Counter(QueuedAsyncOperationCount)
	if err != nil {
		return err
	}

	a.counters[AsyncOperationCount], err = meter.Int64Counter(AsyncOperationCount)
	if err != nil {
		return err
	}

	a.counters[ExtendedAsyncOperationCount], err = meter.Int64Counter(ExtendedAsyncOperationCount)
	if err != nil {
		return err
	}

	a.valueRecorders[AsnycOperationDuration], err = meter.Float64Histogram(AsnycOperationDuration)
	if err != nil {
		return err
	}

	return nil
}

// RecordQueuedAsyncOperation records the number of queued async operations with resource type and operation type attributes.
// It should be called when an async operation is queued successfully.
func (a *asyncOperationMetrics) RecordQueuedAsyncOperation(ctx context.Context) {
	if a.counters[QueuedAsyncOperationCount] != nil {
		serviceCtx := v1.ARMRequestContextFromContext(ctx)
		a.counters[QueuedAsyncOperationCount].Add(ctx, 1,
			metric.WithAttributes(
				resourceTypeAttrKey.String(normalizeAttrValue(serviceCtx.ResourceID.Type())),
				operationTypeAttrKey.String(normalizeAttrValue(serviceCtx.OperationType.Method.HTTPMethod())),
			),
		)
	}
}

// RecordAsyncOperation records the number of async operations. If an error occurs, it is returned. It should be called
// when an async operation is completed.
func (a *asyncOperationMetrics) RecordAsyncOperation(ctx context.Context, req *ctrl.Request, res *ctrl.Result) {
	if a.counters[AsyncOperationCount] != nil {
		a.counters[AsyncOperationCount].Add(ctx, 1, metric.WithAttributes(newAsyncOperationCommonAttributes(req, res)...))
	}
}

// RecordExtendedAsyncOperation increments the ExtendedAsyncOperationCount metric for the given request. It should
// be called with an async operation is extended.
func (a *asyncOperationMetrics) RecordExtendedAsyncOperation(ctx context.Context, req *ctrl.Request) {
	if a.counters[ExtendedAsyncOperationCount] != nil {
		a.counters[ExtendedAsyncOperationCount].Add(ctx, 1, metric.WithAttributes(newAsyncOperationCommonAttributes(req, nil)...))
	}
}

// RecordAsyncOperationDuration records the duration of an asynchronous operation in milliseconds.
func (a *asyncOperationMetrics) RecordAsyncOperationDuration(ctx context.Context, req *ctrl.Request, startTime time.Time) {
	if a.valueRecorders[AsnycOperationDuration] != nil {
		elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
		a.valueRecorders[AsnycOperationDuration].Record(ctx, elapsedTime, metric.WithAttributes(newAsyncOperationCommonAttributes(req, nil)...))
	}
}

func newAsyncOperationCommonAttributes(req *ctrl.Request, res *ctrl.Result) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0)

	resourceID, err := resources.ParseResource(req.ResourceID)
	if err == nil {
		attrs = append(attrs, resourceTypeAttrKey.String(normalizeAttrValue(resourceID.Type())))
	}

	opType, ok := v1.ParseOperationType(req.OperationType)
	if ok {
		attrs = append(attrs, operationTypeAttrKey.String(normalizeAttrValue(opType.Method.HTTPMethod())))
	}

	if res != nil && res.ProvisioningState() != "" {
		attrs = append(attrs, OperationStateAttrKey.String(normalizeAttrValue(string(res.ProvisioningState()))))
	}

	if res != nil && res.Error != nil && res.Error.Code != "" {
		attrs = append(attrs, operationErrorCodeAttrKey.String(normalizeAttrValue(string(res.Error.Code))))
	}

	return attrs
}

func normalizeAttrValue(value string) string {
	return strings.ToLower(value)
}
