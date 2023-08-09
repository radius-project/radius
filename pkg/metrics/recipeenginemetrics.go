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
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	// RecipeEngineOperationCount is the metric name for the recipe engine operation counter.
	// Operations can be Execution, and Deletion.
	RecipeEngineOperationCounter = "recipe_engine_operation_counter"

	// RecipeEngineOperationDurationMilliseconds is the metric name for the recipe engine operation duration.
	// Operations can be Execution, and Deletion.
	RecipeEngineOperationDurationMilliseconds = "recipe_engine_operation_duration_milliseconds"

	// RecipeOperation_Execute is the metric name for the recipe execution operation.
	RecipeOperation_Execute = "execute"

	// RecipeOperation_Delete is the metric name for the recipe deletion operation.
	RecipeOperation_Delete = "delete"

	// RecipeOperationResult_Success is the metric name for the successful recipe operation result.
	RecipeOperationResult_Success = "success"

	// RecipeOperationResult_Failed is the metric name for the failed recipe operation result.
	RecipeOperationResult_Failed = "failed"
)

type recipeEngineMetrics struct {
	counters       map[string]metric.Int64Counter
	valueRecorders map[string]metric.Float64Histogram
}

func newRecipeEngineMetrics() *recipeEngineMetrics {
	return &recipeEngineMetrics{
		counters:       make(map[string]metric.Int64Counter),
		valueRecorders: make(map[string]metric.Float64Histogram),
	}
}

// Init initializes the Recipe Engine metrics.
func (m *recipeEngineMetrics) Init() error {
	meter := otel.GetMeterProvider().Meter("recipe-engine-metrics")

	var err error
	m.counters[RecipeEngineOperationCounter], err = meter.Int64Counter(RecipeEngineOperationCounter)
	if err != nil {
		return err
	}

	m.valueRecorders[RecipeEngineOperationDurationMilliseconds], err = meter.Float64Histogram(RecipeEngineOperationDurationMilliseconds)
	if err != nil {
		return err
	}

	return nil
}

// RecordRecipeOperation records the Recipe Engine operation with the given attributes.
func (m *recipeEngineMetrics) RecordRecipeOperation(ctx context.Context, attrs []attribute.KeyValue) {
	if m.counters[RecipeEngineOperationCounter] != nil {
		m.counters[RecipeEngineOperationCounter].Add(ctx, 1,
			metric.WithAttributes(attrs...))
	}
}

// RecordRecipeOperationDuration records the Recipe Engine operation duration with the given attributes.
func (m *recipeEngineMetrics) RecordRecipeOperationDuration(ctx context.Context, startTime time.Time, attrs []attribute.KeyValue) {
	if m.valueRecorders[RecipeEngineOperationDurationMilliseconds] != nil {
		elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
		m.valueRecorders[RecipeEngineOperationDurationMilliseconds].Record(ctx, elapsedTime,
			metric.WithAttributes(attrs...))
	}
}
