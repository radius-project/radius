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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlattenPropertiesAliases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTop   map[string]any // expected top-level keys to be present (and equal)
		wantNoTop []string       // top-level keys that must NOT be present
		expectErr bool
		// If unchanged is true, output bytes must equal input bytes (no Unmarshal/Marshal round-trip).
		unchanged bool
	}{
		{
			name: "single resource hoists properties children",
			input: `{
  "id": "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/containers/ctnr",
  "name": "ctnr",
  "type": "Applications.Core/containers",
  "properties": {
    "application": "myapp",
    "container": { "image": "nginx" }
  }
}`,
			wantTop: map[string]any{
				"application": "myapp",
				"container":   map[string]any{"image": "nginx"},
			},
		},
		{
			name: "properties itself is preserved",
			input: `{
  "name": "ctnr",
  "properties": { "foo": 1 }
}`,
			wantTop: map[string]any{
				"foo":        float64(1),
				"properties": map[string]any{"foo": float64(1)},
			},
		},
		{
			name: "reserved envelope keys are never overwritten",
			input: `{
  "id": "envelope-id",
  "name": "envelope-name",
  "properties": {
    "id": "inner-id",
    "name": "inner-name",
    "tags": { "k": "v" },
    "foo": "bar"
  }
}`,
			wantTop: map[string]any{
				"id":   "envelope-id",
				"name": "envelope-name",
				"foo":  "bar",
			},
			wantNoTop: []string{"tags"},
		},
		{
			name: "collision with existing top-level key is skipped",
			input: `{
  "name": "envelope-name",
  "customField": "existing",
  "properties": { "customField": "would-overwrite", "other": "ok" }
}`,
			wantTop: map[string]any{
				"customField": "existing",
				"other":       "ok",
			},
		},
		{
			name:      "no properties key is pass-through",
			input:     `{"id":"x","name":"y","status":"Succeeded"}`,
			wantTop:   map[string]any{"id": "x", "name": "y", "status": "Succeeded"},
			unchanged: true,
		},
		{
			name:      "properties is not an object is pass-through",
			input:     `{"name":"y","properties":"a-string"}`,
			wantTop:   map[string]any{"name": "y", "properties": "a-string"},
			unchanged: true,
		},
		{
			name:      "properties is null is pass-through",
			input:     `{"name":"y","properties":null}`,
			wantTop:   map[string]any{"name": "y", "properties": nil},
			unchanged: true,
		},
		{
			name: "paginated list flattens each element",
			input: `{
  "value": [
    {"name":"a","properties":{"foo":1}},
    {"name":"b","properties":{"bar":2}}
  ],
  "nextLink": "https://example/next"
}`,
		},
		{
			name:      "empty paginated list is pass-through",
			input:     `{"value":[],"nextLink":""}`,
			unchanged: true,
		},
		{
			name:      "async operation status pass-through",
			input:     `{"id":"op-id","name":"op-name","status":"Succeeded","startTime":"2023-01-01T00:00:00Z"}`,
			wantTop:   map[string]any{"id": "op-id", "status": "Succeeded"},
			unchanged: true,
		},
		{
			name:      "empty body is pass-through",
			input:     ``,
			unchanged: true,
		},
		{
			name:      "malformed json returns error",
			input:     `{not-json`,
			expectErr: true,
		},
		{
			name:      "non-object top-level (string) is pass-through",
			input:     `"a-string"`,
			unchanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := flattenPropertiesAliases([]byte(tt.input))
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tt.unchanged {
				require.Equal(t, tt.input, string(got), "expected pass-through bytes")
			}

			if tt.name == "paginated list flattens each element" {
				var out map[string]any
				require.NoError(t, json.Unmarshal(got, &out))
				values, ok := out["value"].([]any)
				require.True(t, ok)
				require.Len(t, values, 2)

				a := values[0].(map[string]any)
				require.Equal(t, float64(1), a["foo"])
				b := values[1].(map[string]any)
				require.Equal(t, float64(2), b["bar"])
				return
			}

			if len(tt.wantTop) > 0 || len(tt.wantNoTop) > 0 {
				var out map[string]any
				require.NoError(t, json.Unmarshal(got, &out))
				for k, v := range tt.wantTop {
					require.Equal(t, v, out[k], "key %q", k)
				}
				for _, k := range tt.wantNoTop {
					_, present := out[k]
					require.False(t, present, "key %q must not be present at top level", k)
				}
			}
		})
	}
}

func TestFlattenPropertiesAliases_AliasIsSameReference(t *testing.T) {
	// The alias must point at the same object as properties.<key>, so mutating
	// the alias is observable through properties.<key>. This guarantees we are
	// not deep-copying.
	body := []byte(`{"name":"x","properties":{"container":{"image":"nginx"}}}`)
	got, err := flattenPropertiesAliases(body)
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal(got, &out))

	top := out["container"].(map[string]any)
	inner := out["properties"].(map[string]any)["container"].(map[string]any)
	require.Equal(t, top["image"], inner["image"])
}
