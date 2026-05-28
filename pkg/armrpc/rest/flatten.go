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

package rest

import (
	"context"
	"encoding/json"

	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// marshalResourceBody marshals body as indented JSON and then hoists each
// child of "properties" onto the top level as a read-only alias via
// flattenPropertiesAliases. If the flatten step fails (which should never
// happen for valid marshaled JSON) the original marshaled bytes are
// returned and the error is logged — flatten is best-effort sugar and must
// never fail the request.
func marshalResourceBody(ctx context.Context, body any) ([]byte, error) {
	bytes, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return nil, err
	}

	flattened, ferr := flattenPropertiesAliases(bytes)
	if ferr != nil {
		ucplog.FromContextOrDiscard(ctx).Info("flatten properties failed; returning unflattened body", "error", ferr)
		return bytes, nil
	}
	return flattened, nil
}

// reservedEnvelopeKeys is the union of standard ARM tracked and proxy
// resource envelope keys. These keys are never overwritten by aliases
// hoisted from "properties" and are never themselves splatted.
var reservedEnvelopeKeys = map[string]struct{}{
	"id":               {},
	"name":             {},
	"type":             {},
	"location":         {},
	"tags":             {},
	"properties":       {},
	"systemData":       {},
	"kind":             {},
	"etag":             {},
	"sku":              {},
	"identity":         {},
	"plan":             {},
	"managedBy":        {},
	"extendedLocation": {},
	"zones":            {},
}

// flattenPropertiesAliases takes a marshaled ARM resource body (single
// resource or PaginatedList) and, for every object that has a "properties"
// object, copies each key of that object onto the top level as a read-only
// alias. The original "properties" object is preserved unchanged.
//
// Reserved envelope keys are never overwritten or splatted. If a top-level
// key already exists with the same name as a child of "properties", that
// child is skipped (no overwrite).
//
// Returns the original bytes unchanged if the body is not a JSON object/array
// or contains no "properties" object to hoist. The hoist is shallow — only
// one level of aliasing — and nested values are copied by reference.
func flattenPropertiesAliases(body []byte) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}

	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}

	if !flattenValue(v) {
		return body, nil
	}

	return json.MarshalIndent(v, "", "  ")
}

// flattenValue walks v in place and hoists "properties" children onto the
// parent object wherever applicable. Returns true if any modification was
// made.
func flattenValue(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}

	// PaginatedList shape: { "value": [...], "nextLink": "...", "count": n }
	// Recurse into each element of value rather than treating the list itself
	// as a resource.
	if value, isList := m["value"].([]any); isList && isPaginatedListShape(m) {
		modified := false
		for _, item := range value {
			if flattenValue(item) {
				modified = true
			}
		}
		return modified
	}

	props, hasProps := m["properties"].(map[string]any)
	if !hasProps {
		return false
	}

	modified := false
	for k, val := range props {
		if _, reserved := reservedEnvelopeKeys[k]; reserved {
			continue
		}
		if _, exists := m[k]; exists {
			continue
		}
		m[k] = val
		modified = true
	}
	return modified
}

// isPaginatedListShape reports whether m looks like a PaginatedList envelope:
// it has a "value" array and no keys outside the known list-envelope keys.
func isPaginatedListShape(m map[string]any) bool {
	for k := range m {
		switch k {
		case "value", "nextLink", "count":
		default:
			return false
		}
	}
	return true
}
