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

package manifest

import (
	"context"
	"fmt"
	"regexp"

	"github.com/go-playground/validator/v10"
	"github.com/radius-project/radius/pkg/schema"
)

var (
	resourceProviderNamespaceRegex = regexp.MustCompile(`^[A-Z][A-Za-z0-9]+\.[A-Z][A-Za-z0-9]+$`)
	resourceTypeRegex              = regexp.MustCompile(`^[a-z][A-Za-z0-9]+$`)
	apiVersionRegex                = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(-preview)?$`)
	capabilityRegex                = regexp.MustCompile(`^[A-Z][A-Za-z0-9]+$`)

	resourceProviderNamespaceMessage = "{0} must be a valid resource provider namespace. A resource provider namespace must contain two PascalCased segments separated by a '.'. Example: MyCompany.Resources"
	resourceTypeMessage              = "{0} must be a valid resource type. A resource type should be camelCased. Example: myResourceType"
	apiVersionMessage                = "{0} must be a valid API version. An API version must be a date in YYYY-MM-DD format, and may optionally have the suffix '-preview'. Example: 2025-01-01"
	capabilityMessage                = "{0} must be a valid capability. A capability should use PascalCase. Example: MyCapability"
)

func resourceProviderNamespace(fl validator.FieldLevel) bool {
	str := fl.Field().String()
	return resourceProviderNamespaceRegex.Match([]byte(str))
}

func validateResourceType(fl validator.FieldLevel) bool {
	str := fl.Field().String()
	return resourceTypeRegex.Match([]byte(str))
}

func validateAPIVersion(fl validator.FieldLevel) bool {
	str := fl.Field().String()
	return apiVersionRegex.Match([]byte(str))
}

func validateCapability(fl validator.FieldLevel) bool {
	str := fl.Field().String()
	return capabilityRegex.Match([]byte(str))
}

// validateManifestSchemas validates schemas in a ResourceProvider
func validateManifestSchemas(ctx context.Context, provider *ResourceProvider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}

	validator := schema.NewValidator()
	errors := &schema.ValidationErrors{}

	// Iterate through resource types in the provider
	for resourceTypeName, resourceType := range provider.Types {
		// Check each API version
		for apiVersion, versionInfo := range resourceType.APIVersions {
			if versionInfo.Schema != nil {
				schemaPath := fmt.Sprintf("%s/%s@%s", provider.Name, resourceTypeName, apiVersion)

				// Convert schema to OpenAPI schema
				openAPISchema, err := schema.ConvertToOpenAPISchema(versionInfo.Schema)
				if err != nil {
					errors.Add(schema.NewSchemaError(schemaPath, fmt.Sprintf("failed to parse schema: %v", err)))
					continue
				}

				// Validate the schema
				if err := validator.ValidateSchema(ctx, openAPISchema); err != nil {
					if valErr, ok := err.(*schema.ValidationError); ok {
						valErr.Field = schemaPath + "." + valErr.Field
						errors.Add(valErr)
					} else {
						errors.Add(schema.NewSchemaError(schemaPath, err.Error()))
					}
				}
			}
		}
	}

	if errors.HasErrors() {
		return errors
	}

	return nil
}
