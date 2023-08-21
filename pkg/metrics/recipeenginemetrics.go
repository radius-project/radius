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

	"github.com/radius-project/radius/pkg/recipes"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	// recipeEngineOperationDuration is the metric name for the recipe engine operation duration.
	recipeEngineOperationDuration = "recipe.operation.duration"

	// recipeDownloadDuration is the metric name for the recipe download duration.
	recipeDownloadDuration = "recipe.download.duration"

	// terraformInstallationDuration is the metric name for the Terraform installation duration.
	terraformInstallationDuration = "recipe.tf.installation.duration"

	// terraformInitializationDuration is the metric name for the Terraform initialization duration.
	terraformInitializationDuration = "recipe.tf.init.duration"

	// RecipeEngineOperationExecute represents the Execute operation of the Recipe Engine.
	RecipeEngineOperationExecute = "execute"

	// RecipeEngineOperationDelete represents the Delete operation of the Recipe Engine.
	RecipeEngineOperationDelete = "delete"

	// RecipeEngineOperationDownloadRecipe represents the Download Recipe operation of the Recipe Engine.
	RecipeEngineOperationDownloadRecipe = "download.recipe"
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
	m.valueRecorders[recipeEngineOperationDuration], err = meter.Float64Histogram(recipeEngineOperationDuration)
	if err != nil {
		return err
	}

	m.valueRecorders[recipeDownloadDuration], err = meter.Float64Histogram(recipeDownloadDuration)
	if err != nil {
		return err
	}

	m.valueRecorders[terraformInstallationDuration], err = meter.Float64Histogram(terraformInstallationDuration)
	if err != nil {
		return err
	}

	m.valueRecorders[terraformInitializationDuration], err = meter.Float64Histogram(terraformInitializationDuration)
	if err != nil {
		return err
	}

	return nil
}

// RecordRecipeOperationDuration records the Recipe Engine operation duration with the given attributes.
func (m *recipeEngineMetrics) RecordRecipeOperationDuration(ctx context.Context, startTime time.Time, attrs []attribute.KeyValue) {
	if m.valueRecorders[recipeEngineOperationDuration] != nil {
		elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
		m.valueRecorders[recipeEngineOperationDuration].Record(ctx, elapsedTime,
			metric.WithAttributes(attrs...))
	}
}

// RecordRecipeDownloadDuration records the recipe download duration with the given attributes.
func (m *recipeEngineMetrics) RecordRecipeDownloadDuration(ctx context.Context, startTime time.Time, attrs []attribute.KeyValue) {
	if m.valueRecorders[recipeDownloadDuration] != nil {
		elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
		m.valueRecorders[recipeDownloadDuration].Record(ctx, elapsedTime, metric.WithAttributes(attrs...))
	}
}

// RecordTerraformInstallationDuration records the duration of the Terraform installation with the given attributes.
func (m *recipeEngineMetrics) RecordTerraformInstallationDuration(ctx context.Context, startTime time.Time, attrs []attribute.KeyValue) {
	if m.valueRecorders[terraformInstallationDuration] != nil {
		elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
		m.valueRecorders[terraformInstallationDuration].Record(ctx, elapsedTime, metric.WithAttributes(attrs...))
	}
}

// RecordTerraformInitializationDuration records the Terraform initialization duration with the given attributes.
func (m *recipeEngineMetrics) RecordTerraformInitializationDuration(ctx context.Context, startTime time.Time, attrs []attribute.KeyValue) {
	if m.valueRecorders[terraformInitializationDuration] != nil {
		elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
		m.valueRecorders[terraformInitializationDuration].Record(ctx, elapsedTime, metric.WithAttributes(attrs...))
	}
}

// NewRecipeAttributes generates common attributes for recipe operations.
func NewRecipeAttributes(operationType, recipeName string, definition *recipes.EnvironmentDefinition, state string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0)

	if operationType != "" {
		attrs = append(attrs, operationTypeAttrKey.String(strings.ToLower(operationType)))
	}

	if recipeName != "" {
		attrs = append(attrs, recipeNameAttrKey.String(strings.ToLower(recipeName)))
	}

	if definition != nil && definition.Driver != "" {
		attrs = append(attrs, recipeDriverAttrKey.String(strings.ToLower(definition.Driver)))
	}

	if definition != nil && definition.TemplatePath != "" {
		attrs = append(attrs, recipeTemplatePathAttrKey.String(strings.ToLower(definition.TemplatePath)))

	}

	if state != "" {
		attrs = append(attrs, operationStateAttrKey.String(strings.ToLower(state)))
	}

	return attrs
}
