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

package learn

import (
	"strings"
)

// ResourceTypeSchema represents the structure for a resource type definition
type ResourceTypeSchema struct {
	Namespace string                    `yaml:"namespace"`
	Types     map[string]ResourceType   `yaml:"types"`
}

// ResourceType represents a single resource type with its API versions
type ResourceType struct {
	APIVersions map[string]APIVersionSchema `yaml:"apiVersions"`
}

// APIVersionSchema represents the schema for a specific API version
type APIVersionSchema struct {
	Schema SchemaDefinition `yaml:"schema"`
}

// SchemaDefinition represents the OpenAPI-style schema definition
type SchemaDefinition struct {
	Type       string                      `yaml:"type"`
	Properties map[string]PropertyDefinition `yaml:"properties"`
	Required   []string                    `yaml:"required,omitempty"`
}

// PropertyDefinition represents a single property in the schema
type PropertyDefinition struct {
	Type        string      `yaml:"type"`
	Description string      `yaml:"description,omitempty"`
	Default     interface{} `yaml:"default,omitempty"`
}

// GenerateResourceTypeSchema converts a Terraform module to a resource type schema
func GenerateResourceTypeSchema(module *TerraformModule, namespace, resourceTypeName string) (*ResourceTypeSchema, error) {
	if namespace == "" {
		namespace = "Custom.Resources"
	}

	if resourceTypeName == "" {
		// Generate resource type name from module name
		resourceTypeName = generateResourceTypeName(module.Name)
	}

	properties := make(map[string]PropertyDefinition)
	var required []string

	// Add standard Radius properties
	properties["application"] = PropertyDefinition{
		Type:        "string",
		Description: "The resource ID of the application.",
	}
	properties["environment"] = PropertyDefinition{
		Type:        "string",
		Description: "The resource ID of the environment.",
	}
	required = append(required, "application", "environment")

	// Convert Terraform variables to schema properties
	for _, variable := range module.Variables {
		prop := PropertyDefinition{
			Type:        ConvertTerraformTypeToJSONSchema(variable.Type),
			Description: variable.Description,
		}

		if variable.Default != nil {
			prop.Default = variable.Default
		}

		properties[variable.Name] = prop

		if variable.Required {
			required = append(required, variable.Name)
		}
	}

	schema := SchemaDefinition{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}

	apiVersions := map[string]APIVersionSchema{
		"2025-01-01-preview": {
			Schema: schema,
		},
	}

	resourceType := ResourceType{
		APIVersions: apiVersions,
	}

	types := map[string]ResourceType{
		resourceTypeName: resourceType,
	}

	return &ResourceTypeSchema{
		Namespace: namespace,
		Types:     types,
	}, nil
}

// generateResourceTypeName creates a valid resource type name from module name
func generateResourceTypeName(moduleName string) string {
	// Remove common prefixes/suffixes
	name := moduleName
	name = strings.TrimPrefix(name, "terraform-")
	name = strings.TrimPrefix(name, "tf-")
	name = strings.TrimSuffix(name, "-module")
	name = strings.TrimSuffix(name, "-terraform")

	// Convert to camelCase by splitting on delimiters and capitalizing words
	parts := strings.FieldsFunc(name, func(c rune) bool {
		return c == '-' || c == '_' || c == '.'
	})

	if len(parts) == 0 {
		return "customResource"
	}

	// Build camelCase string
	var result strings.Builder
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 {
			// First word starts with lowercase
			result.WriteString(strings.ToLower(part))
		} else {
			// Subsequent words start with uppercase
			if len(part) > 0 {
				result.WriteString(strings.ToUpper(string(part[0])))
				if len(part) > 1 {
					result.WriteString(strings.ToLower(part[1:]))
				}
			}
		}
	}

	finalName := result.String()
	if finalName == "" {
		return "customResource"
	}

	return finalName
}

// GenerateModuleName extracts a meaningful name from git URL
func GenerateModuleName(gitURL string) string {
	// Extract repository name from git URL
	parts := strings.Split(gitURL, "/")
	if len(parts) > 0 {
		repoName := parts[len(parts)-1]
		if strings.HasSuffix(repoName, ".git") {
			repoName = strings.TrimSuffix(repoName, ".git")
		}
		return repoName
	}
	return "terraform-module"
}