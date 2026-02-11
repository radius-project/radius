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

import "strings"

type sensitivePathSegment struct {
	name     string
	wildcard bool
}

func sanitizeSensitiveEncryptedValues(properties map[string]any, schema map[string]any) {
	if properties == nil || schema == nil {
		return
	}

	paths := ExtractSensitiveFieldPaths(schema, "")
	for _, path := range paths {
		sanitizeEncryptedAtPath(properties, schema, path)
	}
}

func sanitizeEncryptedAtPath(properties map[string]any, schema map[string]any, path string) {
	segments := parseSensitivePath(path)
	if len(segments) == 0 {
		return
	}

	fieldSchema := getSchemaForSensitivePath(schema, segments)
	placeholder := placeholderValue(fieldSchema)
	applySanitizeAtSegments(properties, segments, placeholder)
}

func applySanitizeAtSegments(current any, segments []sensitivePathSegment, placeholder any) {
	if len(segments) == 0 {
		return
	}

	segment := segments[0]
	remaining := segments[1:]

	if segment.wildcard {
		switch v := current.(type) {
		case []any:
			for i := range v {
				if len(remaining) == 0 {
					v[i] = sanitizeValueIfEncrypted(v[i], placeholder)
				} else {
					applySanitizeAtSegments(v[i], remaining, placeholder)
				}
			}
		case map[string]any:
			for key := range v {
				if len(remaining) == 0 {
					v[key] = sanitizeValueIfEncrypted(v[key], placeholder)
				} else {
					applySanitizeAtSegments(v[key], remaining, placeholder)
				}
			}
		}
		return
	}

	dataMap, ok := current.(map[string]any)
	if !ok {
		return
	}

	value, exists := dataMap[segment.name]
	if !exists {
		return
	}

	if len(remaining) == 0 {
		dataMap[segment.name] = sanitizeValueIfEncrypted(value, placeholder)
		return
	}

	applySanitizeAtSegments(value, remaining, placeholder)
}

func sanitizeValueIfEncrypted(value any, placeholder any) any {
	encMap, ok := value.(map[string]any)
	if !ok {
		return value
	}

	_, hasEncrypted := encMap["encrypted"].(string)
	_, hasNonce := encMap["nonce"].(string)
	if !hasEncrypted || !hasNonce {
		return value
	}

	return placeholder
}

func placeholderValue(schema map[string]any) any {
	if schema == nil {
		return ""
	}

	if t, ok := schema["type"].(string); ok {
		switch t {
		case "object":
			return map[string]any{}
		case "array":
			return []any{}
		default:
			return ""
		}
	}

	return ""
}

func getSchemaForSensitivePath(schema map[string]any, segments []sensitivePathSegment) map[string]any {
	current := schema

	for _, segment := range segments {
		if segment.wildcard {
			if items, ok := current["items"].(map[string]any); ok {
				current = items
				continue
			}
			if addProps, ok := current["additionalProperties"].(map[string]any); ok {
				current = addProps
				continue
			}
			return nil
		}

		properties, ok := current["properties"].(map[string]any)
		if !ok {
			return nil
		}

		next, ok := properties[segment.name].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}

	return current
}

func parseSensitivePath(path string) []sensitivePathSegment {
	if path == "" {
		return nil
	}

	var segments []sensitivePathSegment
	var current strings.Builder

	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '.':
			if current.Len() > 0 {
				segments = append(segments, sensitivePathSegment{name: current.String()})
				current.Reset()
			}
		case '[':
			if current.Len() > 0 {
				segments = append(segments, sensitivePathSegment{name: current.String()})
				current.Reset()
			}
			end := strings.Index(path[i:], "]")
			if end == -1 {
				return nil
			}
			content := path[i+1 : i+end]
			if content == "*" {
				segments = append(segments, sensitivePathSegment{wildcard: true})
			}
			i += end
		default:
			current.WriteByte(path[i])
		}
	}

	if current.Len() > 0 {
		segments = append(segments, sensitivePathSegment{name: current.String()})
	}

	return segments
}
