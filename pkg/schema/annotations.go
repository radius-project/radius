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

package schema

import (
	"context"
	"strings"

	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// GetSensitiveFieldPaths fetches the schema for a resource and returns paths to fields marked with x-radius-sensitive.
// Paths are in dot notation, e.g., "credentials.password" or "config.apiKey".
//
// Parameters:
//   - ctx: The request context
//   - ucpClient: UCP client factory for fetching the schema
//   - resourceID: The full resource ID (e.g., "/planes/radius/local/resourceGroups/test/providers/Foo.Bar/myResources/test")
//   - resourceType: The resource type (e.g., "Foo.Bar/myResources")
//   - apiVersion: The API version to fetch the schema for
//
// Returns:
//   - []string: Paths to sensitive fields, or empty slice if none found
//   - error: Any error encountered while fetching the schema
func GetSensitiveFieldPaths(ctx context.Context, ucpClient *v20231001preview.ClientFactory, resourceID string, resourceType string, apiVersion string) ([]string, error) {
	schema, err := GetSchema(ctx, ucpClient, resourceID, resourceType, apiVersion)
	if err != nil {
		return nil, err
	}
	if schema == nil {
		return nil, nil
	}

	return ExtractSensitiveFieldPaths(schema, ""), nil
}

// GetSchema fetches the OpenAPI schema for a resource type and api version.
// Returns nil if the schema is not found or the client is nil.
func GetSchema(ctx context.Context, ucpClient *v20231001preview.ClientFactory, resourceID string, resourceType string, apiVersion string) (map[string]any, error) {
	if ucpClient == nil {
		return nil, nil
	}

	// Parse the resource ID to get plane information
	ID, err := resources.Parse(resourceID)
	if err != nil {
		return nil, err
	}

	plane := ID.PlaneNamespace()
	planeName := strings.Split(plane, "/")[1]
	resourceProvider := strings.Split(resourceType, "/")[0]
	resourceTypeName := strings.Split(resourceType, "/")[1]

	// Fetch the API version resource which contains the schema
	apiVersionResource, err := ucpClient.NewAPIVersionsClient().Get(ctx, planeName, resourceProvider, resourceTypeName, apiVersion, nil)
	if err != nil {
		return nil, err
	}

	return apiVersionResource.APIVersionResource.Properties.Schema, nil
}

// ExtractSensitiveFieldPaths recursively walks the schema and returns paths to fields marked with x-radius-sensitive.
// The prefix parameter builds up the path as we traverse nested objects.
// Supports object properties, array items, and additionalProperties (maps).
// If a field is marked sensitive, its nested properties are not checked since the entire field is considered sensitive.
func ExtractSensitiveFieldPaths(schema map[string]any, prefix string) []string {
	var paths []string

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return paths
	}

	for fieldName, fieldSchema := range properties {
		fieldSchemaMap, ok := fieldSchema.(map[string]any)
		if !ok {
			continue
		}

		// Build the full path for this field
		var fullPath string
		if prefix == "" {
			fullPath = fieldName
		} else {
			fullPath = prefix + "." + fieldName
		}

		// Check if this field has the x-radius-sensitive annotation
		// If sensitive, add the path and skip nested properties since the entire field is sensitive
		if isSensitive, ok := fieldSchemaMap[annotationRadiusSensitive].(bool); ok && isSensitive {
			paths = append(paths, fullPath)
			continue
		}

		// Recursively check nested objects
		if nestedProps, ok := fieldSchemaMap["properties"].(map[string]any); ok {
			nestedSchema := map[string]any{"properties": nestedProps}
			nestedPaths := ExtractSensitiveFieldPaths(nestedSchema, fullPath)
			paths = append(paths, nestedPaths...)
		}

		// Handle array types - check items schema
		// Path uses [*] to indicate all array elements, e.g., "secrets[*].value"
		if items, ok := fieldSchemaMap["items"].(map[string]any); ok {
			arrayItemPath := fullPath + "[*]"

			// Check if items themselves are marked sensitive
			// If sensitive, add the path and skip nested properties
			if isSensitive, ok := items[annotationRadiusSensitive].(bool); ok && isSensitive {
				paths = append(paths, arrayItemPath)
			} else {
				// Recursively check nested properties within array items
				if itemProps, ok := items["properties"].(map[string]any); ok {
					itemSchema := map[string]any{"properties": itemProps}
					nestedPaths := ExtractSensitiveFieldPaths(itemSchema, arrayItemPath)
					paths = append(paths, nestedPaths...)
				}
			}
		}

		// Handle additionalProperties (map/dictionary types)
		// Path uses [*] to indicate all map values, e.g., "secrets[*]" or "config[*].password"
		if additionalProps, ok := fieldSchemaMap["additionalProperties"].(map[string]any); ok {
			mapValuePath := fullPath + "[*]"

			// Check if additionalProperties values are marked sensitive
			// If sensitive, add the path and skip nested properties
			if isSensitive, ok := additionalProps[annotationRadiusSensitive].(bool); ok && isSensitive {
				paths = append(paths, mapValuePath)
			} else {
				// Recursively check nested properties within additionalProperties
				if addProps, ok := additionalProps["properties"].(map[string]any); ok {
					addPropsSchema := map[string]any{"properties": addProps}
					nestedPaths := ExtractSensitiveFieldPaths(addPropsSchema, mapValuePath)
					paths = append(paths, nestedPaths...)
				}
			}
		}
	}

	return paths
}
