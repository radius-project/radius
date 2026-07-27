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

// ApplyDefaults fills unset properties from the "default" declared in the schema and returns the
// number applied.
//
// Explicit values are never overwritten. Required properties are never defaulted, so omitting one
// still fails validation. Read-only properties are recipe outputs, and an absent object is not
// created just to hold defaults.
func ApplyDefaults(properties map[string]any, schemaData map[string]any) int {
	if properties == nil || schemaData == nil {
		return 0
	}

	declared, ok := schemaData["properties"].(map[string]any)
	if !ok {
		return 0
	}

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

		// A supplied object is descended into even when required, since its own optional
		// properties may declare defaults.
		if existing, present := properties[name]; present {
			if nested, ok := existing.(map[string]any); ok {
				applied += ApplyDefaults(nested, fieldSchema)
			}
			continue
		}

		if required[name] {
			continue
		}

		if defaultValue, ok := fieldSchema["default"]; ok {
			properties[name] = copyDefaultValue(defaultValue)
			applied++
		}
	}

	return applied
}

// requiredProperties reads the schema's "required" list, accepting either the []any from JSON
// decoding or a []string.
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

// copyDefaultValue deep copies a default so mutating a materialized property cannot reach back into
// a cached schema.
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
