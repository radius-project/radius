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

package bicep

import (
	"strings"
)

const (
	// environmentResourceType is the resource type for Radius environments
	environmentResourceType = "radius.core/environments"

	// legacyEnvironmentResourceType is the legacy resource type for Radius environments
	legacyEnvironmentResourceType = "applications.core/environments"
)

// radiusNamespacePatterns lists namespace prefixes for resource type namespaces that belong to Radius.
// Resource types matching these prefixes are routed through the Radius control plane, not Azure ARM.
var radiusNamespacePatterns = []string{
	"Applications.*",
	"Radius.*",
}

// TemplateInspectionResult contains the results of inspecting a Bicep template's resources.
type TemplateInspectionResult struct {
	// ContainsEnvironmentResource indicates whether the template contains an environment resource.
	ContainsEnvironmentResource bool
}

// ResourceTypeEntry represents a parsed resource type from a compiled Bicep/ARM template.
type ResourceTypeEntry struct {
	// FullType is the full type string including API version, e.g. "Radius.Core/applications@2025-08-01-preview".
	FullType string

	// Type is the resource type without the API version, e.g. "Radius.Core/applications".
	Type string

	// APIVersion is the API version, e.g. "2025-08-01-preview".
	APIVersion string
}

// ExtractResourceTypes extracts all resource types and their API versions from a compiled Bicep/ARM template.
func ExtractResourceTypes(template map[string]any) []ResourceTypeEntry {
	if template == nil {
		return nil
	}

	resourcesValue, ok := template["resources"]
	if !ok {
		return nil
	}

	resources, ok := resourcesValue.(map[string]any)
	if !ok {
		return nil
	}

	var entries []ResourceTypeEntry
	for _, resourceValue := range resources {
		resource, ok := resourceValue.(map[string]any)
		if !ok {
			continue
		}

		resourceType, ok := resource["type"].(string)
		if !ok || resourceType == "" {
			continue
		}

		entry := ResourceTypeEntry{FullType: resourceType}
		if idx := strings.Index(resourceType, "@"); idx >= 0 {
			entry.Type = resourceType[:idx]
			entry.APIVersion = resourceType[idx+1:]
		} else {
			entry.Type = resourceType
		}

		entries = append(entries, entry)
	}

	return entries
}

// IsRadiusResourceType returns true if the given resource type belongs to a known Radius namespace.
// It matches against the patterns in radiusNamespacePatterns (e.g. "Applications.*", "Radius.*")
// using case-insensitive prefix matching on the namespace portion before the dot and slash.
func IsRadiusResourceType(resourceType string) bool {
	lower := strings.ToLower(resourceType)
	for _, pattern := range radiusNamespacePatterns {
		// Convert pattern like "Applications.*" to a lowercase prefix like "applications."
		prefix := strings.ToLower(strings.TrimSuffix(pattern, "*"))
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// HasOnlyRadiusResourceTypes returns true if the template contains at least one resource and
// all resources belong to known Radius namespaces (none are Azure ARM or other non-Radius types).
func HasOnlyRadiusResourceTypes(template map[string]any) bool {
	entries := ExtractResourceTypes(template)
	if len(entries) == 0 {
		return false
	}

	for _, entry := range entries {
		if !IsRadiusResourceType(entry.Type) {
			return false
		}
	}

	return true
}

// InspectTemplateResources inspects the compiled Radius Bicep template's resources to find
// environment resources and determine if any environment resource is present.
//
// The expected structure of resource in the template is:
// {"resources": {"resourceName": {"type": "Applications.Core/containers@2023-10-01-preview", ...}}}
func InspectTemplateResources(template map[string]any) TemplateInspectionResult {
	result := TemplateInspectionResult{
		ContainsEnvironmentResource: false,
	}

	if template == nil {
		return result
	}

	resourcesValue, ok := template["resources"]
	if !ok {
		return result
	}

	resources, ok := resourcesValue.(map[string]any)
	if !ok {
		return result
	}

	for _, resourceValue := range resources {
		resource, ok := resourceValue.(map[string]any)
		if !ok {
			continue
		}

		resourceType, ok := resource["type"].(string)
		if !ok {
			continue
		}

		resourceTypeLower := strings.ToLower(resourceType)

		// Check for environment resource
		if strings.HasPrefix(resourceTypeLower, environmentResourceType) ||
			strings.HasPrefix(resourceTypeLower, legacyEnvironmentResourceType) {
			result.ContainsEnvironmentResource = true
		}
	}

	return result
}

// ContainsEnvironmentResource inspects the compiled Radius Bicep template's resources to determine if an
// environment resource will be created as part of the deployment.
//
// The expected structure of resource in the template is:
// {"resources": {"resourceName": {"type": "Applications.Core/environments@2023-10-01-preview", ...}}}
func ContainsEnvironmentResource(template map[string]any) bool {
	return InspectTemplateResources(template).ContainsEnvironmentResource
}
