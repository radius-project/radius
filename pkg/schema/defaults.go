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

// ApplyDefaults adds defaults to missing properties and returns how many it added.
//
// It keeps explicit values and skips required and read-only properties. It doesn't create missing
// objects or arrays unless their schema declares a default.
func ApplyDefaults(properties map[string]any, schemaData map[string]any) int {
	if properties == nil || schemaData == nil {
		return 0
	}

	declared, _ := schemaData["properties"].(map[string]any)
	required := requiredProperties(schemaData)

	applied := 0
	for name, raw := range declared {
		fieldSchema, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		if readOnly, ok := fieldSchema["readOnly"].(bool); ok && readOnly {
			continue
		}

		// Apply nested defaults to supplied objects and arrays, even when required.
		if existing, present := properties[name]; present {
			applied += applyDefaultsToValue(existing, fieldSchema)
			continue
		}

		if required[name] {
			continue
		}

		if defaultValue, ok := fieldSchema["default"]; ok {
			materialized := copyDefaultValue(defaultValue)
			properties[name] = materialized
			applied++
			applied += applyDefaultsToValue(materialized, fieldSchema)
		}
	}

	additionalProperties, ok := schemaData["additionalProperties"].(map[string]any)
	if !ok {
		return applied
	}

	for name, existing := range properties {
		if _, declared := declared[name]; declared {
			continue
		}

		applied += applyDefaultsToValue(existing, additionalProperties)
	}

	return applied
}

func applyDefaultsToValue(value any, schemaData map[string]any) int {
	switch typed := value.(type) {
	case map[string]any:
		return ApplyDefaults(typed, schemaData)
	case []any:
		itemSchema, ok := schemaData["items"].(map[string]any)
		if !ok {
			return 0
		}

		applied := 0
		for _, item := range typed {
			applied += applyDefaultsToValue(item, itemSchema)
		}
		return applied
	default:
		return 0
	}
}

// requiredProperties accepts required lists decoded as []any or []string.
func requiredProperties(schemaData map[string]any) map[string]bool {
	required := map[string]bool{}

	switch list := schemaData["required"].(type) {
	case []any:
		for _, item := range list {
			if name, ok := item.(string); ok {
				required[name] = true
			}
		}
	case []string:
		for _, name := range list {
			required[name] = true
		}
	}

	return required
}

// copyDefaultValue copies maps and slices so resources don't share schema data.
func copyDefaultValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copied := make(map[string]any, len(typed))
		for key, item := range typed {
			copied[key] = copyDefaultValue(item)
		}
		return copied
	case []any:
		copied := make([]any, len(typed))
		for i, item := range typed {
			copied[i] = copyDefaultValue(item)
		}
		return copied
	default:
		return value
	}
}
