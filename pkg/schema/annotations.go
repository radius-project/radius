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
	"strconv"
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
		// If sensitive, treat the whole field as sensitive and skip nested properties.
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

// FieldPathSegment represents a single segment in a field path.
// A field path can contain field names, wildcards, and array indices.
type FieldPathSegment struct {
	Type  SegmentType
	Value string // field name or index value (empty for wildcards)
}

// SegmentType represents the type of a field path segment.
type SegmentType int

const (
	// SegmentTypeField represents a named field (e.g., "password" in "credentials.password")
	SegmentTypeField SegmentType = iota
	// SegmentTypeWildcard represents a wildcard segment (e.g., [*] in "secrets[*].value")
	SegmentTypeWildcard
	// SegmentTypeIndex represents an array index (e.g., [0] in "items[0].name")
	SegmentTypeIndex
)

// IsWildcard returns true if the segment is a wildcard.
func (s FieldPathSegment) IsWildcard() bool {
	return s.Type == SegmentTypeWildcard
}

// IsField returns true if the segment is a named field.
func (s FieldPathSegment) IsField() bool {
	return s.Type == SegmentTypeField
}

// IsIndex returns true if the segment is an array index.
func (s FieldPathSegment) IsIndex() bool {
	return s.Type == SegmentTypeIndex
}

// ParseFieldPath parses a field path string into segments.
// Supports dot notation, wildcards [*], and array indices [N].
//
// Examples:
//   - "credentials.password" -> [field:credentials, field:password]
//   - "secrets[*].value" -> [field:secrets, wildcard, field:value]
//   - "config[*]" -> [field:config, wildcard]
//   - "items[0].name" -> [field:items, index:0, field:name]
//
// Returns nil if the path is invalid (e.g., unterminated bracket).
func ParseFieldPath(path string) []FieldPathSegment {
	if path == "" {
		return nil
	}

	var segments []FieldPathSegment
	var current strings.Builder

	i := 0
	for i < len(path) {
		ch := path[i]

		switch ch {
		case '.':
			if current.Len() > 0 {
				segments = append(segments, FieldPathSegment{Type: SegmentTypeField, Value: current.String()})
				current.Reset()
			}
			i++

		case '[':
			if current.Len() > 0 {
				segments = append(segments, FieldPathSegment{Type: SegmentTypeField, Value: current.String()})
				current.Reset()
			}

			// Find the closing bracket
			end := strings.Index(path[i:], "]")
			if end == -1 {
				// Invalid path - unterminated bracket
				return nil
			}

			bracketContent := path[i+1 : i+end]
			if bracketContent == "*" {
				segments = append(segments, FieldPathSegment{Type: SegmentTypeWildcard})
			} else {
				segments = append(segments, FieldPathSegment{Type: SegmentTypeIndex, Value: bracketContent})
			}
			i += end + 1

		default:
			current.WriteByte(ch)
			i++
		}
	}

	// Don't forget the last segment
	if current.Len() > 0 {
		segments = append(segments, FieldPathSegment{Type: SegmentTypeField, Value: current.String()})
	}

	return segments
}

// RedactFields sets the values at the given field paths to nil in the data map.
// Paths support dot notation, wildcards [*], and array indices [N].
// Missing fields and invalid paths are silently skipped.
func RedactFields(data map[string]any, paths []string) {
	if data == nil {
		return
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		segments := ParseFieldPath(path)
		if len(segments) == 0 {
			continue
		}
		redactAtSegments(data, segments)
	}
}

// redactAtSegments traverses the data following the path segments and sets the final value to nil.
func redactAtSegments(current any, segments []FieldPathSegment) {
	if len(segments) == 0 {
		return
	}

	segment := segments[0]
	remaining := segments[1:]

	// Handle wildcard [*] - iterate over array or map
	if segment.IsWildcard() {
		switch v := current.(type) {
		case []any:
			for i := range v {
				if len(remaining) == 0 {
					v[i] = nil
				} else {
					redactAtSegments(v[i], remaining)
				}
			}
		case map[string]any:
			for key := range v {
				if len(remaining) == 0 {
					v[key] = nil
				} else {
					redactAtSegments(v[key], remaining)
				}
			}
		}
		return
	}

	// Handle array index [N]
	if segment.IsIndex() {
		arr, ok := current.([]any)
		if !ok {
			return
		}

		idx, err := strconv.Atoi(segment.Value)
		if err != nil || idx < 0 || idx >= len(arr) {
			return
		}

		if len(remaining) == 0 {
			arr[idx] = nil
		} else {
			redactAtSegments(arr[idx], remaining)
		}
		return
	}

	// Handle field name
	dataMap, ok := current.(map[string]any)
	if !ok {
		return
	}

	value, exists := dataMap[segment.Value]
	if !exists {
		return
	}

	if len(remaining) == 0 {
		dataMap[segment.Value] = nil
		return
	}

	redactAtSegments(value, remaining)
}
