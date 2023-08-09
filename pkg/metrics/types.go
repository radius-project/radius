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

	"github.com/project-radius/radius/pkg/recipes"
	"go.opentelemetry.io/otel/attribute"
)

const (
	// ResourceTypeAttrKey is the attribute name for the resource type.
	ResourceTypeAttrKey = "resource_type"

	// OperationTypeAttrKey is the attribute name for the operation type.
	OperationTypeAttrKey = "operation_type"

	// OperationStateAttrKey is the attribute name for the operation state.
	OperationStateAttrKey = "operation_state"

	// OperationErrorCodeAttrKey is the attribute name for the operation error code.
	OperationErrorCodeAttrKey = "operation_error_code"

	// RecipeNameAttrKey is the attribute name for the recipe name.
	RecipeNameAttrKey = "recipe_name"

	// RecipeDriverAttrKey is the attribute name for the recipe driver.
	RecipeDriverAttrKey = "recipe_driver"

	// RecipeTemplatePathAttrKey is the attribute name for the recipe template path.
	RecipeTemplatePathAttrKey = "recipe_template_path"

	// SuccessfulOperationState is the value for a successful operation state.
	SuccessfulOperationState = "success"
)

// GenerateStringAttribute generates a string attribute.
func GenerateStringAttribute(key, value string) attribute.KeyValue {
	return attribute.String(key, strings.ToLower(value))
}

// GenerateRecipeOperationCommonAttributes generates common attributes for recipe operations.
func GenerateRecipeOperationCommonAttributes(operationType, recipeName string, definition *recipes.EnvironmentDefinition, opRes string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0)

	if operationType != "" {
		attrs = append(attrs, attribute.String(OperationTypeAttrKey, strings.ToLower(operationType)))
	}

	if recipeName != "" {
		attrs = append(attrs, attribute.String(RecipeNameAttrKey, strings.ToLower(operationType)))
	}

	if definition != nil && definition.Driver != "" {
		attrs = append(attrs, attribute.String(RecipeDriverAttrKey, strings.ToLower(definition.Driver)))
	}

	if definition != nil && definition.TemplatePath != "" {
		attrs = append(attrs, attribute.String(RecipeTemplatePathAttrKey, strings.ToLower(definition.TemplatePath)))

	}

	if opRes != "" {
		attrs = append(attrs, attribute.String(OperationStateAttrKey, opRes))
	}

	return attrs
}
