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

package schema

import (
	"fmt"
	"slices"
	"strings"

	"github.com/radius-project/radius/pkg/ucp/resources"
)

// ResourceReferenceAnnotation identifies the resource type referenced by a string property.
const ResourceReferenceAnnotation = "x-radius-resource-reference"

// ResourceReference identifies a related Radius resource without assigning provisioning ownership.
type ResourceReference struct {
	ID   string
	Type string
}

// ExtractResourceReferences discovers writable, annotated references in objects, arrays, and maps.
// Computed properties and unannotated strings do not establish relationships.
func ExtractResourceReferences(properties, resourceSchema map[string]any) ([]ResourceReference, error) {
	if err := ValidateResourceReferences(resourceSchema); err != nil {
		return nil, err
	}
	found := map[string]ResourceReference{}
	if err := collectResourceReferences(properties, resourceSchema, found); err != nil {
		return nil, err
	}
	result := make([]ResourceReference, 0, len(found))
	for _, reference := range found {
		result = append(result, reference)
	}
	slices.SortFunc(result, func(a, b ResourceReference) int {
		return strings.Compare(strings.ToLower(a.ID), strings.ToLower(b.ID))
	})
	return result, nil
}

// ValidateResourceReferences validates relationship metadata, including absent optional properties.
func ValidateResourceReferences(field map[string]any) error {
	if field == nil {
		return nil
	}
	if raw, exists := field[ResourceReferenceAnnotation]; exists {
		if err := validateResourceReferenceAnnotation(raw, field["type"] == "string"); err != nil {
			return err
		}
	}
	properties, _ := field["properties"].(map[string]any)
	for _, raw := range properties {
		child, _ := raw.(map[string]any)
		if err := ValidateResourceReferences(child); err != nil {
			return err
		}
	}
	for _, name := range []string{"items", "additionalProperties"} {
		child, _ := field[name].(map[string]any)
		if err := ValidateResourceReferences(child); err != nil {
			return err
		}
	}
	return nil
}

func validateResourceReferenceAnnotation(raw any, isString bool) error {
	resourceType, ok := raw.(string)
	if !ok || !isString {
		return fmt.Errorf("resource reference annotation requires a string field and fully qualified resource type")
	}
	id, err := resources.ParseResource("/planes/radius/local/resourceGroups/validation/providers/" + resourceType + "/validation")
	if err != nil || id.Type() != resourceType {
		return fmt.Errorf("resource reference annotation requires a fully qualified resource type")
	}
	return nil
}

func collectResourceReferences(value any, field map[string]any, found map[string]ResourceReference) error {
	if readOnly, _ := field["readOnly"].(bool); readOnly {
		return nil
	}
	if resourceType, annotated := field[ResourceReferenceAnnotation].(string); annotated {
		rawID, ok := value.(string)
		if !ok {
			return fmt.Errorf("a schema-declared resource reference must be a Radius resource ID string")
		}
		id, err := resources.ParseResource(rawID)
		if err != nil || id.FindScope("radius") == "" || !strings.EqualFold(id.Type(), resourceType) {
			// Neither arbitrary map keys nor sensitive input values belong in validation errors.
			return fmt.Errorf("a schema-declared resource reference must identify a Radius resource of its declared type")
		}
		key := strings.ToLower(id.String())
		if _, exists := found[key]; !exists {
			found[key] = ResourceReference{ID: id.String(), Type: id.Type()}
		}
		return nil
	}
	switch value := value.(type) {
	case map[string]any:
		fields, _ := field["properties"].(map[string]any)
		for key, child := range value {
			childSchema, declared := fields[key]
			if !declared {
				childSchema = field["additionalProperties"]
			}
			nested, _ := childSchema.(map[string]any)
			if err := collectResourceReferences(child, nested, found); err != nil {
				return err
			}
		}
	case []any:
		items, _ := field["items"].(map[string]any)
		for _, child := range value {
			if err := collectResourceReferences(child, items, found); err != nil {
				return err
			}
		}
	}
	return nil
}
