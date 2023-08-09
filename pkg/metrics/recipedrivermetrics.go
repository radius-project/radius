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
	// RecipeDownloadDurationMilliseconds is the metric name for the recipe download duration in milliseconds.
	RecipeDownloadDurationMilliseconds = "recipe_download_duration_milliseconds"

	// RecipeOperation_Download is the metric name for the recipe download operation.
	RecipeOperation_Download = "download"
)

type recipeDriverMetrics struct {
	counters       map[string]metric.Int64Counter
	valueRecorders map[string]metric.Float64Histogram
}

func newRecipeDriverMetrics() *recipeDriverMetrics {
	return &recipeDriverMetrics{
		counters:       make(map[string]metric.Int64Counter),
		valueRecorders: make(map[string]metric.Float64Histogram),
	}
}

// Init initializes the Recipe Driver metrics.
func (m *recipeDriverMetrics) Init() error {
	meter := otel.GetMeterProvider().Meter("recipe-driver-metrics")

	var err error

	m.valueRecorders[RecipeDownloadDurationMilliseconds], err = meter.Float64Histogram(RecipeDownloadDurationMilliseconds)
	if err != nil {
		return err
	}

	return nil
}

// RecordRecipeDownloadDuration records the recipe download duration in milliseconds with the given attributes.
func (m *recipeDriverMetrics) RecordRecipeDownloadDuration(ctx context.Context, startTime time.Time, attrs []attribute.KeyValue) {
	if m.valueRecorders[RecipeDownloadDurationMilliseconds] != nil {
		elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
		m.valueRecorders[RecipeDownloadDurationMilliseconds].Record(ctx, elapsedTime, metric.WithAttributes(attrs...))
	}
}
