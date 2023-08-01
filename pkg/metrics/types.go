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
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

const (
	// ResourceTypeAttrKey is the attribute name for the resource type.
	ResourceTypeAttrKey = "resource_type"

	// OperationTypeAttrKey is the attribute name for the operation type.
	OperationTypeAttrKey = "operation_type"

	// OperationStateAttrKey is the attribute name for the operation state.
	OperationStateAttrKey = "operation_state"

	// DriverAttrKey is the attribute name for the recipe driver.
	DriverAttrKey = "driver"

	// TemplatePathAttrKey is the attribute name for the template path.
	TemplatePathAttrKey = "template_path"

	// RecipeExecutionResultAttrKey is the attribute name for the recipe execution result.
	RecipeExecutionResultAttrKey = "recipe_execution_result"

	// RecipeDownloadResultAttrKey is the attribute name for the recipe download result.
	RecipeDownloadResultAttrKey = "recipe_download_result"
)

// GenerateDriverAttribute generates a driver attribute to be used in a metric.
func GenerateDriverAttribute(driver string) attribute.KeyValue {
	return attribute.String(DriverAttrKey, strings.ToLower(driver))
}

// GenerateResourceTypeAttribute generates a resource type attribute to be used in a metric.
func GenerateResourceTypeAttribute(resourceType string) attribute.KeyValue {
	return attribute.String(ResourceTypeAttrKey, strings.ToLower(resourceType))
}

// GenerateTemplatePathAttribute generates a template path attribute to be used in a metric.
func GenerateTemplatePathAttribute(templatePath string) attribute.KeyValue {
	return attribute.String(TemplatePathAttrKey, strings.ToLower(templatePath))
}

// GenerateRecipeExecutionResultAttribute generates a recipe execution result attribute to be used in a metric.
func GenerateRecipeExecutionResultAttribute(result string) attribute.KeyValue {
	return attribute.String(RecipeExecutionResultAttrKey, strings.ToLower(result))
}

// GenerateRecipeDownloadResultAttribute generates a recipe download result attribute to be used in a metric.
func GenerateRecipeDownloadResultAttribute(result string) attribute.KeyValue {
	return attribute.String(RecipeDownloadResultAttrKey, strings.ToLower(result))
}
