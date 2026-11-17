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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractResourceReferences(t *testing.T) {
	t.Parallel()
	const target = "/planes/radius/local/resourceGroups/rg/providers/Radius.Compute/containers/a"
	reference := map[string]any{"type": "string", ResourceReferenceAnnotation: "Radius.Compute/containers"}
	sourceSchema := map[string]any{"properties": map[string]any{
		"rules": map[string]any{"items": map[string]any{"properties": map[string]any{
			"destinations": map[string]any{"additionalProperties": reference},
		}}},
		"computed": map[string]any{"readOnly": true, "properties": map[string]any{"id": reference}},
	}}
	for _, tt := range []struct {
		name       string
		properties map[string]any
		count      int
		fails      bool
	}{
		{"array and map navigation", map[string]any{"rules": []any{map[string]any{"destinations": map[string]any{"one": target, "two": target}}}}, 1, false},
		{"case-insensitive deduplication", map[string]any{"rules": []any{map[string]any{"destinations": map[string]any{"one": target, "two": strings.Replace(target, "containers/a", "CONTAINERS/A", 1)}}}}, 1, false},
		{"optional relationship absent", map[string]any{"arbitraryId": target}, 0, false},
		{"computed parent excluded", map[string]any{"computed": map[string]any{"id": target}}, 0, false},
		{"wrong resource type", map[string]any{"rules": []any{map[string]any{"destinations": map[string]any{"one": target + "/children/a"}}}}, 0, true},
		{"platform resource excluded", map[string]any{"rules": []any{map[string]any{"destinations": map[string]any{"one": "/subscriptions/sub/resourceGroups/rg/providers/Radius.Compute/containers/a"}}}}, 0, true},
		{"invalid ID", map[string]any{"rules": []any{map[string]any{"destinations": map[string]any{"private-key": "private-value"}}}}, 0, true},
		{"null reference", map[string]any{"rules": []any{map[string]any{"destinations": map[string]any{"one": nil}}}}, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := ExtractResourceReferences(tt.properties, sourceSchema)
			if tt.fails {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "private-key")
				require.NotContains(t, err.Error(), "private-value")
				return
			}
			require.NoError(t, err)
			require.Len(t, result, tt.count)
			if tt.count > 0 {
				require.True(t, strings.EqualFold(target, result[0].ID))
			}
		})
	}
}

func TestResourceReferenceMetadataValidation(t *testing.T) {
	t.Parallel()
	for _, field := range []map[string]any{
		{"type": "object", ResourceReferenceAnnotation: "Radius.Compute/containers"},
		{"type": "string", ResourceReferenceAnnotation: true},
		{"type": "string", ResourceReferenceAnnotation: ""},
		{"type": "string", ResourceReferenceAnnotation: "containers"},
	} {
		_, err := ExtractResourceReferences(map[string]any{}, map[string]any{"properties": map[string]any{"link": field}})
		require.ErrorContains(t, err, "resource reference annotation requires")
		converted, err := ConvertToOpenAPISchema(map[string]any{"type": "object", "properties": map[string]any{"link": field}})
		require.NoError(t, err)
		require.ErrorContains(t, NewValidator().ValidateSchema(t.Context(), converted), "resource reference annotation requires")
	}
}
