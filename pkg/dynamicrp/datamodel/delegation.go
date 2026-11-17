/*
Copyright 2026 The Radius Authors.

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

package datamodel

import (
	"strings"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// SelectDelegationTarget normalizes only schema-annotated identities for the existing selector.
// Desired values remain unchanged, including secrets, arbitrary strings and opaque revisions.
func SelectDelegationTarget(properties, sourceSchema map[string]any, targetType string) (resources.ID, error) {
	if target, err := recipes.SelectDelegationTarget(properties, sourceSchema, targetType); err == nil {
		return target, nil
	}
	normalized, _ := normalizeReferenceIdentity(properties, sourceSchema).(map[string]any)
	return recipes.SelectDelegationTarget(normalized, sourceSchema, targetType)
}

func normalizeReferenceIdentity(value any, field map[string]any) any {
	if _, annotated := field[schema.ResourceReferenceAnnotation]; annotated {
		if id, ok := value.(string); ok {
			return strings.ToLower(id)
		}
	}
	switch value := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		fields, _ := field["properties"].(map[string]any)
		for key, child := range value {
			childSchema, exists := fields[key]
			if !exists {
				childSchema = field["additionalProperties"]
			}
			nested, _ := childSchema.(map[string]any)
			result[key] = normalizeReferenceIdentity(child, nested)
		}
		return result
	case []any:
		result := make([]any, len(value))
		nested, _ := field["items"].(map[string]any)
		for i, child := range value {
			result[i] = normalizeReferenceIdentity(child, nested)
		}
		return result
	default:
		return value
	}
}
