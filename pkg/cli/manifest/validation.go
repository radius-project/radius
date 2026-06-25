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
	"github.com/radius-project/radius/pkg/schema/baseresource"
)

// baseManifest is the embedded base resource manifest, parsed once at package
// initialization and reused for the lifetime of the process. The manifest layer
// owns this single immutable instance; every registration path merges it into
// per-type schemas through ValidateManifest. It never changes after load.
var baseManifest = baseresource.MustLoad()

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

// applyBaseResourceManifest merges the common base resource properties into every
// schema in the provider, in place, so that schema validation and registration
// both operate on the effective (merged) schema. The same schema maps mutated
// here are what RegisterType ships to UCP, so authors omit the base properties
// and the control plane still receives them.
func applyBaseResourceManifest(provider *ResourceProvider) error {
	if provider == nil {
		return nil
	}

	for resourceTypeName, resourceType := range provider.Types {
		if resourceType == nil {
			continue
		}

		for apiVersion, versionInfo := range resourceType.APIVersions {
			if versionInfo == nil || versionInfo.Schema == nil {
				continue
			}

			schemaMap, ok := versionInfo.Schema.(map[string]any)
			if !ok {
				// Leave non-object schemas untouched; schema validation reports them.
				continue
			}

			// The base property names are reserved by Radius. A per-type schema
			// must not redeclare them under "properties" (it may still list them
			// under "required" to make them mandatory). Check the author's raw
			// schema before Apply injects the base properties.
			if conflicts := baseManifest.ConflictingProperties(schemaMap); len(conflicts) > 0 {
				return fmt.Errorf("%s/%s@%s: schema declares reserved Radius properties %v that are provided automatically and must not be redeclared (you may list them under \"required\" to make them mandatory)",
					provider.Namespace, resourceTypeName, apiVersion, conflicts)
			}

			if err := baseManifest.Apply(schemaMap); err != nil {
				return fmt.Errorf("%s/%s@%s: %w", provider.Namespace, resourceTypeName, apiVersion, err)
			}
		}
	}

	return nil
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
				schemaPath := fmt.Sprintf("%s/%s@%s", provider.Namespace, resourceTypeName, apiVersion)

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
