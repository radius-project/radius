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

package graph

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
)

// reviewRelevantKeys are the property keys used for diff hash computation.
// These are the properties that reviewers care about — changes to these
// indicate a meaningful modification to the resource.
var reviewRelevantKeys = []string{
	"connections",
	"container",
	"ports",
	"routes",
	"resources",
	"recipe",
	"resourceProvisioning",
}

// ComputeDiffHash extracts review-relevant properties, canonicalizes them
// to sorted JSON, and returns a hex-encoded SHA-256 hash string.
// The hash is deterministic: identical inputs always produce the same hash.
func ComputeDiffHash(properties map[string]interface{}) string {
	canonical := extractCanonicalProperties(properties)

	data, err := marshalCanonical(canonical)
	if err != nil {
		// If we can't marshal, return empty — resource will show as modified.
		return ""
	}

	hash := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", hash)
}

// extractCanonicalProperties extracts only the review-relevant properties
// and returns them as a sorted map.
func extractCanonicalProperties(properties map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for _, key := range reviewRelevantKeys {
		if val, ok := properties[key]; ok {
			result[key] = val
		}
	}
	return result
}

// marshalCanonical produces deterministic JSON output by sorting all map keys
// at every level of nesting.
func marshalCanonical(v interface{}) ([]byte, error) {
	normalized := normalizeForJSON(v)
	return json.Marshal(normalized)
}

// normalizeForJSON recursively sorts map keys and normalizes values for
// deterministic JSON serialization.
func normalizeForJSON(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		ordered := make([]keyValue, 0, len(keys))
		for _, k := range keys {
			ordered = append(ordered, keyValue{Key: k, Value: normalizeForJSON(val[k])})
		}
		return orderedMap(ordered)

	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = normalizeForJSON(item)
		}
		return result

	default:
		return val
	}
}

// keyValue is a key-value pair for ordered map serialization.
type keyValue struct {
	Key   string
	Value interface{}
}

// orderedMap is a slice of key-value pairs that marshals to JSON as an object
// with keys in the order they appear in the slice.
type orderedMap []keyValue

func (o orderedMap) MarshalJSON() ([]byte, error) {
	buf := []byte{'{'}
	for i, kv := range o {
		if i > 0 {
			buf = append(buf, ',')
		}

		keyJSON, err := json.Marshal(kv.Key)
		if err != nil {
			return nil, err
		}
		buf = append(buf, keyJSON...)
		buf = append(buf, ':')

		valJSON, err := json.Marshal(kv.Value)
		if err != nil {
			return nil, err
		}
		buf = append(buf, valJSON...)
	}
	buf = append(buf, '}')
	return buf, nil
}
