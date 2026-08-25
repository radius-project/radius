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
	"fmt"
	"sort"
	"strings"
)

const (
	// environmentResourceType is the resource type for Radius environments
	environmentResourceType = "radius.core/environments"

	// legacyEnvironmentResourceType is the legacy resource type for Radius environments
	legacyEnvironmentResourceType = "applications.core/environments"

	// deprecatedNamespacePrefix is the namespace prefix of the legacy Radius resource types.
	deprecatedNamespacePrefix = "applications."

	// deprecatedAPIVersion is the API version of the legacy Radius resource types.
	deprecatedAPIVersion = "2023-10-01-preview"

	// replacementAPIVersion is the API version of the extensible Radius.* resource types that
	// replace the legacy Applications.* resource types.
	replacementAPIVersion = "2025-08-01-preview"
)

// deprecatedTypeReplacements maps a lowercased legacy Applications.* resource type to the
// extensible Radius.* resource type that replaces it. Types that are absent from this map have no
// direct one-to-one replacement and are reported with generic recipe pack guidance instead.
//
// Keep this in sync with the built-in resource types in deploy/manifest/defaults.yaml.
var deprecatedTypeReplacements = map[string]string{
	"applications.core/applications":         "Radius.Core/applications",
	"applications.core/environments":         "Radius.Core/environments",
	"applications.core/containers":           "Radius.Compute/containers",
	"applications.core/gateways":             "Radius.Compute/routes",
	"applications.core/volumes":              "Radius.Compute/persistentVolumes",
	"applications.core/secretstores":         "Radius.Security/secrets",
	"applications.datastores/mongodatabases": "Radius.Data/mongoDatabases",
	"applications.datastores/rediscaches":    "Radius.Data/redisCaches",
	"applications.datastores/sqldatabases":   "Radius.Data/sqlServerDatabases",
	"applications.messaging/rabbitmqqueues":  "Radius.Messaging/rabbitMQ",
}

// DeprecatedResource describes a resource in a template that uses a legacy Applications.* resource
// type with the deprecated API version.
type DeprecatedResource struct {
	// FullType is the resource type as authored, including the API version,
	// e.g. "Applications.Core/containers@2023-10-01-preview".
	FullType string

	// Replacement is the extensible Radius.* resource type that replaces this type, including the
	// API version, e.g. "Radius.Compute/containers@2025-08-01-preview". It is empty when no direct
	// replacement exists.
	Replacement string
}

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

	// EnvironmentResources contains the list of environment resources found in the template.
	EnvironmentResources []map[string]any

	// DeprecatedResources contains the resources that use a legacy Applications.* resource type
	// with the deprecated API version, along with their replacements.
	DeprecatedResources []DeprecatedResource
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
		if before, after, ok0 := strings.Cut(resourceType, "@"); ok0 {
			entry.Type = before
			entry.APIVersion = after
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
	containsEnvironment, environmentResources := inspectEnvironmentResources(template)

	return TemplateInspectionResult{
		ContainsEnvironmentResource: containsEnvironment,
		EnvironmentResources:        environmentResources,
		DeprecatedResources:         inspectDeprecatedResources(template),
	}
}

// inspectEnvironmentResources walks the top-level resources of a compiled template and reports
// whether an environment resource is present, along with the Radius.Core environment resources.
//
// This deliberately does not descend into nested module templates: the results drive deployment
// behavior (whether the template creates its own environment, and default recipe pack injection),
// so the scope must stay exactly as it has always been.
func inspectEnvironmentResources(template map[string]any) (bool, []map[string]any) {
	if template == nil {
		return false, nil
	}

	resourcesValue, ok := template["resources"]
	if !ok {
		return false, nil
	}

	resources, ok := resourcesValue.(map[string]any)
	if !ok {
		return false, nil
	}

	containsEnvironment := false
	var environmentResources []map[string]any

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
			containsEnvironment = true
		}

		// add Radius.Core environment resources to the result list
		if strings.HasPrefix(resourceTypeLower, environmentResourceType) {
			environmentResources = append(environmentResources, resource)
		}
	}

	return containsEnvironment, environmentResources
}

// inspectDeprecatedResources returns the distinct deprecated resource types in a compiled template,
// in a stable order.
func inspectDeprecatedResources(template map[string]any) []DeprecatedResource {
	var deprecated []DeprecatedResource
	collectDeprecatedResources(template, map[string]struct{}{}, &deprecated)

	// Sort for deterministic output, since Go map iteration order is randomized.
	sort.Slice(deprecated, func(i, j int) bool {
		return deprecated[i].FullType < deprecated[j].FullType
	})

	return deprecated
}

// newDeprecatedResource returns a DeprecatedResource for the given resource type if it uses a legacy
// Applications.* namespace together with the deprecated API version. The second return value reports
// whether the resource type is deprecated.
func newDeprecatedResource(resourceType string) (DeprecatedResource, bool) {
	baseType, apiVersion, hasAPIVersion := strings.Cut(resourceType, "@")
	if !hasAPIVersion {
		return DeprecatedResource{}, false
	}

	baseTypeLower := strings.ToLower(baseType)
	if !strings.HasPrefix(baseTypeLower, deprecatedNamespacePrefix) ||
		!strings.EqualFold(apiVersion, deprecatedAPIVersion) {
		return DeprecatedResource{}, false
	}

	deprecated := DeprecatedResource{FullType: resourceType}
	if replacement, ok := deprecatedTypeReplacements[baseTypeLower]; ok {
		deprecated.Replacement = replacement + "@" + replacementAPIVersion
	}

	return deprecated, true
}

// collectDeprecatedResources walks a compiled template and appends every distinct deprecated
// resource type it finds to out, recursing into nested templates so that resources declared inside
// Bicep modules are reported too. Templates commonly declare several resources of the same type, so
// seen is shared across the whole walk to report each distinct type only once.
//
// A Bicep module compiles to a resource of type Microsoft.Resources/deployments whose resources are
// nested at properties.template.resources.
func collectDeprecatedResources(template map[string]any, seen map[string]struct{}, out *[]DeprecatedResource) {
	if template == nil {
		return
	}

	resourcesValue, ok := template["resources"]
	if !ok {
		return
	}

	// Symbolic-name templates use a map keyed by resource name; classic ARM templates and nested
	// module templates may use an array instead, so handle both shapes.
	var resourceValues []any
	switch typed := resourcesValue.(type) {
	case map[string]any:
		// Visit map entries in sorted key order. Go randomizes map iteration, and de-duplication
		// keeps the first spelling of a type it encounters, so an unordered walk would render a
		// different casing of the same type from run to run when a template declares it more than
		// once with different casing (resource types are case-insensitive).
		names := make([]string, 0, len(typed))
		for name := range typed {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			resourceValues = append(resourceValues, typed[name])
		}
	case []any:
		resourceValues = typed
	default:
		return
	}

	for _, resourceValue := range resourceValues {
		resource, ok := resourceValue.(map[string]any)
		if !ok {
			continue
		}

		if resourceType, ok := resource["type"].(string); ok {
			if deprecated, isDeprecated := newDeprecatedResource(resourceType); isDeprecated {
				key := strings.ToLower(deprecated.FullType)
				if _, alreadySeen := seen[key]; !alreadySeen {
					seen[key] = struct{}{}
					*out = append(*out, deprecated)
				}
			}
		}

		// Recurse into the inline template of a nested deployment (a Bicep module).
		properties, ok := resource["properties"].(map[string]any)
		if !ok {
			continue
		}

		if nested, ok := properties["template"].(map[string]any); ok {
			collectDeprecatedResources(nested, seen, out)
		}
	}
}

// ContainsEnvironmentResource inspects the compiled Radius Bicep template's resources to determine if an
// environment resource will be created as part of the deployment.
//
// The expected structure of resource in the template is:
// {"resources": {"resourceName": {"type": "Applications.Core/environments@2023-10-01-preview", ...}}}
func ContainsEnvironmentResource(template map[string]any) bool {
	containsEnvironment, _ := inspectEnvironmentResources(template)
	return containsEnvironment
}

// GetEnvironmentResources inspects the compiled Radius Bicep template's resources and returns
// all environment resources found as maps.
func GetEnvironmentResources(template map[string]any) []map[string]any {
	_, environmentResources := inspectEnvironmentResources(template)
	return environmentResources
}

// GetDeprecatedResources inspects the compiled Radius Bicep template's resources and returns the
// resources that use a legacy Applications.* resource type with the deprecated API version.
func GetDeprecatedResources(template map[string]any) []DeprecatedResource {
	return inspectDeprecatedResources(template)
}

// FormatDeprecationWarning builds the user-facing warning message for the given deprecated
// resources. It returns an empty string when there is nothing to warn about, so callers can skip
// printing entirely.
func FormatDeprecationWarning(deprecated []DeprecatedResource) string {
	if len(deprecated) == 0 {
		return ""
	}

	var builder strings.Builder
	fmt.Fprintf(&builder,
		"WARNING: The following resource types use the deprecated Applications.* namespace with API version %s and will be removed in a future release:\n",
		deprecatedAPIVersion)

	for _, resource := range deprecated {
		if resource.Replacement != "" {
			fmt.Fprintf(&builder, "  - %s: use %s instead.\n", resource.FullType, resource.Replacement)
		} else {
			fmt.Fprintf(&builder,
				"  - %s: no direct replacement. Define an equivalent resource type and register a recipe pack that provides it to your environment.\n",
				resource.FullType)
		}
	}

	return builder.String()
}
