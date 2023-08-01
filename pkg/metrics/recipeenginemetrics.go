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
	// RecipeExecutionCount is the metric name for the recipe execution count.
	RecipeExecutionCount = "recipe.execution"

	// RecipeExecutionDuration is the metric name for the recipe execution duration.
	RecipeExecutionDuration = "recipe.execution.duration"

	// RecipeDeletionCount is the metric name for the recipe deletion count.
	RecipeDeletionCount = "recipe.deletion"

	// RecipeDeletionDuration is the metric name for the recipe deletion duration.
	RecipeDeletionDuration = "recipe.deletion.duration"

	// RecipeDownloadCount is the metric name for the recipe download count.
	RecipeDownloadCount = "recipe.download"

	// RecipeDownloadDuration is the metric name for the recipe download duration.
	RecipeDownloadDuration = "recipe.download.duration"
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

func (m *recipeEngineMetrics) Init() error {
	meter := otel.GetMeterProvider().Meter("recipe-engine-metrics")

	var err error
	m.counters[RecipeExecutionCount], err = meter.Int64Counter(RecipeExecutionCount)
	if err != nil {
		return err
	}

	m.valueRecorders[RecipeExecutionDuration], err = meter.Float64Histogram(RecipeExecutionDuration)
	if err != nil {
		return err
	}

	m.counters[RecipeDeletionCount], err = meter.Int64Counter(RecipeDeletionCount)
	if err != nil {
		return err
	}

	m.valueRecorders[RecipeDeletionDuration], err = meter.Float64Histogram(RecipeDeletionDuration)
	if err != nil {
		return err
	}

	m.counters[RecipeDownloadCount], err = meter.Int64Counter(RecipeDownloadCount)
	if err != nil {
		return err
	}

	m.valueRecorders[RecipeDownloadDuration], err = meter.Float64Histogram(RecipeDownloadDuration)
	if err != nil {
		return err
	}

	return nil
}

func (m *recipeEngineMetrics) RecordRecipeExecution(ctx context.Context, attrs ...attribute.KeyValue) {
	if m.counters[RecipeExecutionCount] != nil {
		m.counters[RecipeExecutionCount].Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

func (m *recipeEngineMetrics) RecordRecipeExecutionDuration(ctx context.Context, startTime time.Time, attrs ...attribute.KeyValue) {
	if m.valueRecorders[RecipeExecutionDuration] != nil {
		elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
		m.valueRecorders[AsnycOperationDuration].Record(ctx, elapsedTime, metric.WithAttributes(attrs...))
	}
}

func (m *recipeEngineMetrics) RecordRecipeDeletion(ctx context.Context, attrs ...attribute.KeyValue) {
	if m.counters[RecipeDeletionCount] != nil {
		m.counters[RecipeDeletionCount].Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

func (m *recipeEngineMetrics) RecordRecipeDeletionDuration(ctx context.Context, startTime time.Time, attrs ...attribute.KeyValue) {
	if m.valueRecorders[RecipeDeletionDuration] != nil {
		elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
		m.valueRecorders[RecipeDeletionDuration].Record(ctx, elapsedTime, metric.WithAttributes(attrs...))
	}
}

func (m *recipeEngineMetrics) RecordRecipeDownload(ctx context.Context, attrs ...attribute.KeyValue) {
	if m.counters[RecipeDownloadCount] != nil {
		m.counters[RecipeDownloadCount].Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

func (m *recipeEngineMetrics) RecordRecipeDownloadDuration(ctx context.Context, startTime time.Time, attrs ...attribute.KeyValue) {
	if m.valueRecorders[RecipeDownloadDuration] != nil {
		elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
		m.valueRecorders[RecipeDownloadDuration].Record(ctx, elapsedTime, metric.WithAttributes(attrs...))
	}
}
