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
	"go.opentelemetry.io/otel/attribute"
)

const (
	// resourceTypeAttrKey is the attribute name for the resource type.
	resourceTypeAttrKey = attribute.Key("resource_type")

	// operationTypeAttrKey is the attribute name for the operation type.
	operationTypeAttrKey = attribute.Key("operation_type")

	// operationStateAttrKey is the attribute name for the operation state.
	operationStateAttrKey = attribute.Key("operation_state")

	// operationErrorCodeAttrKey is the attribute name for the operation error code.
	operationErrorCodeAttrKey = attribute.Key("operation_error_code")

	// recipeNameAttrKey is the attribute name for the recipe name.
	recipeNameAttrKey = attribute.Key("recipe_name")

	// recipeDriverAttrKey is the attribute name for the recipe driver.
	recipeDriverAttrKey = attribute.Key("recipe_driver")

	// recipeTemplatePathAttrKey is the attribute name for the recipe template path.
	recipeTemplatePathAttrKey = attribute.Key("recipe_template_path")

	// TerraformVersionAttrKey is the attribute key for the Terraform version.
	TerraformVersionAttrKey = attribute.Key("terraform_version")

	// SuccessfulOperationState is the value for a successful operation state.
	SuccessfulOperationState = "success"

	// FailedOperationState is the value for a failed operation state.
	FailedOperationState = "failed"
)
