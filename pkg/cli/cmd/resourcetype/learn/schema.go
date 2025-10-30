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
	Namespace string                  `yaml:"namespace"`
	Types     map[string]ResourceType `yaml:"types"`
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
	Type       string                        `yaml:"type"`
	Properties map[string]PropertyDefinition `yaml:"properties"`
	Required   []string                      `yaml:"required,omitempty"`
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
		// Skip server-side auto-generated properties that shouldn't be user inputs
		if shouldSkipVariable(variable.Name) {
			continue
		}

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

// InferNamespaceFromModule attempts to infer a meaningful namespace from the module
func InferNamespaceFromModule(module *TerraformModule, gitURL string) string {
	moduleName := GenerateModuleName(gitURL)

	// Extract provider/cloud from module name patterns
	provider := extractProvider(moduleName)
	category := extractCategory(moduleName)

	// Build namespace in format Provider.Category
	if provider != "" && category != "" {
		return titleCase(provider) + "." + titleCase(category)
	} else if provider != "" {
		return titleCase(provider) + ".Resources"
	} else if category != "" {
		return "Custom." + titleCase(category)
	}

	// Fallback to analyzing variables for hints
	if namespace := inferFromVariableNames(module.Variables); namespace != "" {
		return namespace
	}

	// Final fallback
	return "Custom.Resources"
}

// extractProvider identifies cloud provider from module name
func extractProvider(moduleName string) string {
	lower := strings.ToLower(moduleName)

	patterns := map[string]string{
		"aws":    "AWS",
		"amazon": "AWS",
		"azure":  "Azure",
		"gcp":    "GCP",
		"google": "GCP",
		"docker": "Docker",
		"helm":   "Helm",
	}

	for pattern, provider := range patterns {
		if strings.Contains(lower, pattern) {
			return provider
		}
	}

	return ""
}

// extractCategory identifies resource category from module name
func extractCategory(moduleName string) string {
	lower := strings.ToLower(moduleName)

	// Order matters: check more specific patterns first
	patterns := []struct {
		pattern  string
		category string
	}{
		// Network patterns (check before "security" to handle "network-security" correctly)
		{"vpc", "Network"},
		{"network", "Network"},
		{"subnet", "Network"},
		{"ingress", "Network"},
		{"loadbalancer", "Network"},

		// Data/Database resources
		{"redis", "Data"},
		{"database", "Data"},
		{"db", "Data"},
		{"rds", "Data"},
		{"postgres", "Data"},
		{"mysql", "Data"},
		{"mongodb", "Data"},
		{"cassandra", "Data"},
		{"elasticsearch", "Data"},

		// Storage resources
		{"storage", "Storage"},
		{"s3", "Storage"},
		{"blob", "Storage"},

		// Compute resources
		{"compute", "Compute"},
		{"vm", "Compute"},
		{"instance", "Compute"},
		{"container", "Compute"},

		// Orchestration (only for actual orchestration tools, not apps on k8s)
		{"aks", "Orchestration"},
		{"eks", "Orchestration"},
		{"gke", "Orchestration"},
		{"cluster", "Orchestration"},

		// Security resources (check after network to avoid "network-security" conflicts)
		{"security", "Security"},
		{"iam", "Security"},
		{"rbac", "Security"},

		// Observability resources
		{"monitoring", "Observability"},
		{"logging", "Observability"},
		{"metric", "Observability"},
	}

	for _, p := range patterns {
		if strings.Contains(lower, p.pattern) {
			return p.category
		}
	}

	return ""
}

// inferFromVariableNames analyzes variable names for namespace hints
func inferFromVariableNames(variables []TerraformVariable) string {
	providerHints := make(map[string]int)
	categoryHints := make(map[string]int)

	for _, variable := range variables {
		varName := strings.ToLower(variable.Name)

		// Check for provider hints in variable names
		if strings.Contains(varName, "aws") || strings.Contains(varName, "region") {
			providerHints["AWS"]++
		}
		if strings.Contains(varName, "azure") || strings.Contains(varName, "resource_group") {
			providerHints["Azure"]++
		}
		if strings.Contains(varName, "gcp") || strings.Contains(varName, "project") {
			providerHints["GCP"]++
		}

		// Check for category hints
		if strings.Contains(varName, "vpc") || strings.Contains(varName, "subnet") || strings.Contains(varName, "cidr") {
			categoryHints["Network"]++
		}
		if strings.Contains(varName, "db") || strings.Contains(varName, "database") {
			categoryHints["Data"]++
		}
		if strings.Contains(varName, "storage") || strings.Contains(varName, "bucket") {
			categoryHints["Storage"]++
		}
	}

	// Find the most common provider and category
	var topProvider, topCategory string
	var maxProviderCount, maxCategoryCount int

	for provider, count := range providerHints {
		if count > maxProviderCount {
			maxProviderCount = count
			topProvider = provider
		}
	}

	for category, count := range categoryHints {
		if count > maxCategoryCount {
			maxCategoryCount = count
			topCategory = category
		}
	}

	if topProvider != "" && topCategory != "" {
		return topProvider + "." + topCategory
	} else if topProvider != "" {
		return topProvider + ".Resources"
	} else if topCategory != "" {
		return "Custom." + topCategory
	}

	return ""
}

// titleCase converts a string to title case, handling common acronyms
func titleCase(s string) string {
	if s == "" {
		return s
	}

	// Handle common acronyms that should remain uppercase
	upper := strings.ToUpper(s)
	switch upper {
	case "AWS", "GCP", "API", "HTTP", "HTTPS", "DNS", "VPC", "IAM":
		return upper
	}

	return strings.ToUpper(string(s[0])) + strings.ToLower(s[1:])
}

// shouldSkipVariable determines if a Terraform variable should be excluded from the schema
// because it represents server-side auto-generated properties
func shouldSkipVariable(variableName string) bool {
	// Convert to lowercase for case-insensitive comparison
	name := strings.ToLower(variableName)

	// Skip recipe context which is auto-generated by Radius
	// See: https://github.com/radius-project/radius/blob/main/pkg/recipes/recipecontext/types.go#L31
	if name == "context" {
		return true
	}

	return false
}
