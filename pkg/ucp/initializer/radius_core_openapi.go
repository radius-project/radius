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

package initializer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/swagger"
)

const (
	radiusCoreNamespace      = "Radius.Core"
	radiusCoreAPIVersion     = "2025-08-01-preview"
	radiusCoreOpenAPIFile    = "specification/radius/resource-manager/Radius.Core/preview/2025-08-01-preview/openapi.json"
	openAPIDefinitionRefRoot = "#/definitions/"
)

var radiusCoreTypeOpenAPIDefinitions = map[string]struct {
	resourceDefinition   string
	propertiesDefinition string
}{
	"applications": {
		resourceDefinition:   "ApplicationResource",
		propertiesDefinition: "ApplicationProperties",
	},
	"environments": {
		resourceDefinition:   "EnvironmentResource",
		propertiesDefinition: "EnvironmentProperties",
	},
	"recipePacks": {
		resourceDefinition:   "RecipePackResource",
		propertiesDefinition: "RecipePackProperties",
	},
}

type openAPIDocument struct {
	Definitions map[string]map[string]any `json:"definitions"`
}

func hydrateBuiltInResourceProviderMetadata(rp *manifest.ResourceProvider) error {
	if !strings.EqualFold(rp.Namespace, radiusCoreNamespace) {
		return nil
	}

	doc, err := loadRadiusCoreOpenAPI()
	if err != nil {
		return err
	}

	for typeName, resourceType := range rp.Types {
		definitionNames, ok := radiusCoreTypeOpenAPIDefinitions[typeName]
		if !ok {
			continue
		}
		if resourceType == nil {
			return fmt.Errorf("mapped %s type %s is nil", rp.Namespace, typeName)
		}

		description, err := openAPIDefinitionDescription(doc, definitionNames.resourceDefinition)
		if err != nil {
			return fmt.Errorf("failed to get description for %s/%s: %w", rp.Namespace, typeName, err)
		}
		resourceType.Description = &description

		apiVersion, ok := resourceType.APIVersions[radiusCoreAPIVersion]
		if !ok {
			return fmt.Errorf("mapped %s type %s is missing API version %s", rp.Namespace, typeName, radiusCoreAPIVersion)
		}
		if apiVersion == nil {
			return fmt.Errorf("mapped %s type %s has nil API version %s", rp.Namespace, typeName, radiusCoreAPIVersion)
		}

		schema, err := openAPIDefinitionSchema(doc, definitionNames.propertiesDefinition)
		if err != nil {
			return fmt.Errorf("failed to get schema for %s/%s@%s: %w", rp.Namespace, typeName, radiusCoreAPIVersion, err)
		}
		apiVersion.Schema = schema
	}

	return nil
}

func loadRadiusCoreOpenAPI() (*openAPIDocument, error) {
	contents, err := swagger.SpecFiles.ReadFile(radiusCoreOpenAPIFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded Radius.Core OpenAPI spec: %w", err)
	}

	var doc openAPIDocument
	if err := json.Unmarshal(contents, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse embedded Radius.Core OpenAPI spec: %w", err)
	}
	if len(doc.Definitions) == 0 {
		return nil, fmt.Errorf("embedded Radius.Core OpenAPI spec has no definitions")
	}

	return &doc, nil
}

func openAPIDefinitionDescription(doc *openAPIDocument, name string) (string, error) {
	definition, ok := doc.Definitions[name]
	if !ok {
		return "", fmt.Errorf("definition %q not found", name)
	}

	description, _ := definition["description"].(string)
	if description == "" {
		return "", fmt.Errorf("definition %q has no description", name)
	}

	return description, nil
}

func openAPIDefinitionSchema(doc *openAPIDocument, name string) (map[string]any, error) {
	definition, ok := doc.Definitions[name]
	if !ok {
		return nil, fmt.Errorf("definition %q not found", name)
	}

	resolved, err := resolveOpenAPIValue(definition, doc.Definitions, map[string]bool{})
	if err != nil {
		return nil, err
	}

	schema, ok := resolved.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("definition %q did not resolve to an object schema", name)
	}

	return schema, nil
}

func resolveOpenAPIValue(value any, definitions map[string]map[string]any, resolving map[string]bool) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		if ref, ok := typed["$ref"].(string); ok {
			if !strings.HasPrefix(ref, openAPIDefinitionRefRoot) {
				return cloneMap(typed), nil
			}

			definitionName := strings.TrimPrefix(ref, openAPIDefinitionRefRoot)
			if resolving[definitionName] {
				return nil, fmt.Errorf("circular OpenAPI reference %q", ref)
			}

			definition, ok := definitions[definitionName]
			if !ok {
				return nil, fmt.Errorf("OpenAPI reference %q not found", ref)
			}

			resolving[definitionName] = true
			resolved, err := resolveOpenAPIValue(definition, definitions, resolving)
			delete(resolving, definitionName)
			if err != nil {
				return nil, err
			}

			resolvedMap, ok := resolved.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("OpenAPI reference %q did not resolve to an object schema", ref)
			}

			for key, child := range typed {
				if key == "$ref" {
					continue
				}

				resolvedChild, err := resolveOpenAPIValue(child, definitions, resolving)
				if err != nil {
					return nil, err
				}
				resolvedMap[key] = resolvedChild
			}

			return resolvedMap, nil
		}

		result := map[string]any{}
		for key, child := range typed {
			resolvedChild, err := resolveOpenAPIValue(child, definitions, resolving)
			if err != nil {
				return nil, err
			}
			result[key] = resolvedChild
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for i, child := range typed {
			resolvedChild, err := resolveOpenAPIValue(child, definitions, resolving)
			if err != nil {
				return nil, err
			}
			result[i] = resolvedChild
		}
		return result, nil
	default:
		return typed, nil
	}
}

func cloneMap(value map[string]any) map[string]any {
	result := map[string]any{}
	for key, child := range value {
		result[key] = child
	}
	return result
}
