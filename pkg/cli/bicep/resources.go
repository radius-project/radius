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

// TemplateInspectionResult contains the results of inspecting a Bicep template's resources.
type TemplateInspectionResult struct {
	// ContainsEnvironmentResource indicates whether the template contains an environment resource.
	ContainsEnvironmentResource bool

	// EnvironmentResources contains the list of environment resources found in the template.
	EnvironmentResources []map[string]any

	// DeprecatedResources contains the list of resource types using the deprecated Applications.* namespace
	// with the 2023-10-01-preview API version.
	DeprecatedResources []string
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

		// add Radius.Core environment resources to the result list
		if strings.HasPrefix(resourceTypeLower, environmentResourceType) {
			result.EnvironmentResources = append(result.EnvironmentResources, resource)
		}

		// Check for deprecated Applications.* namespace with 2023-10-01-preview API version
		if strings.HasPrefix(resourceTypeLower, deprecatedNamespacePrefix) &&
			strings.HasSuffix(resourceTypeLower, "@"+deprecatedAPIVersion) {
			result.DeprecatedResources = append(result.DeprecatedResources, resourceType)
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

// GetEnvironmentResources inspects the compiled Radius Bicep template's resources and returns
// all environment resources found as mutable maps. The returned maps are references to the
// original template entries, so mutations are reflected in the template.
func GetEnvironmentResources(template map[string]any) []map[string]any {
	return InspectTemplateResources(template).EnvironmentResources
}

// GetDeprecatedResources inspects the compiled Radius Bicep template's resources to find any resources
// using the deprecated Applications.* namespace with the 2023-10-01-preview API version.
//
// Returns a slice of resource type strings that are using the deprecated namespace/API version combination.
func GetDeprecatedResources(template map[string]any) []string {
	return InspectTemplateResources(template).DeprecatedResources
}
