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
	"testing"

	"github.com/stretchr/testify/require"
)

func kafkaSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"environment": map[string]any{"type": "string"},
			"application": map[string]any{"type": "string"},
			"host": map[string]any{
				"type":     "string",
				"readOnly": true,
			},
			"secrets": map[string]any{
				"type": "object",
				"properties": map[string]any{
					// Reserved reference sub-property; excluded from the returned data keys.
					"name": map[string]any{
						"type":     "string",
						"readOnly": true,
					},
					"connectionString": map[string]any{
						"type":     "string",
						"readOnly": true,
					},
					"password": map[string]any{
						"type":     "string",
						"readOnly": true,
					},
				},
			},
		},
	}
}

func Test_GetSecretsBlock(t *testing.T) {
	tests := []struct {
		name     string
		schema   map[string]any
		wantKeys []string
		wantOK   bool
	}{
		{
			name:     "declared secrets block returns sorted data keys, excluding reserved name",
			schema:   kafkaSchema(),
			wantKeys: []string{"connectionString", "password"},
			wantOK:   true,
		},
		{
			name: "block with only the reserved name has no data keys",
			schema: map[string]any{
				"properties": map[string]any{
					"secrets": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{"type": "string", "readOnly": true},
						},
					},
				},
			},
			wantKeys: []string{},
			wantOK:   true,
		},
		{
			name: "writable sub-properties are excluded (reserved for future inputs)",
			schema: map[string]any{
				"properties": map[string]any{
					"secrets": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":             map[string]any{"type": "string", "readOnly": true},
							"connectionString": map[string]any{"type": "string", "readOnly": true},
							"inputValue":       map[string]any{"type": "string"},
						},
					},
				},
			},
			wantKeys: []string{"connectionString"},
			wantOK:   true,
		},
		{
			name: "empty secrets block is present with no keys",
			schema: map[string]any{
				"properties": map[string]any{
					"secrets": map[string]any{
						"type": "object",
					},
				},
			},
			wantKeys: []string{},
			wantOK:   true,
		},
		{
			name: "no secrets block",
			schema: map[string]any{
				"properties": map[string]any{
					"host": map[string]any{"type": "string"},
				},
			},
			wantKeys: nil,
			wantOK:   false,
		},
		{
			name:     "nil schema",
			schema:   nil,
			wantKeys: nil,
			wantOK:   false,
		},
		{
			name:     "schema without properties",
			schema:   map[string]any{"type": "object"},
			wantKeys: nil,
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys, ok := GetSecretsBlock(tt.schema)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantKeys, keys)
		})
	}
}

func Test_HasSecretsBlock(t *testing.T) {
	require.True(t, HasSecretsBlock(kafkaSchema()))
	require.False(t, HasSecretsBlock(map[string]any{"properties": map[string]any{"host": map[string]any{"type": "string"}}}))
	require.False(t, HasSecretsBlock(nil))
}

func Test_ValidateSecretsBlock(t *testing.T) {
	tests := []struct {
		name    string
		schema  map[string]any
		wantErr string
	}{
		{
			name:    "valid secrets block",
			schema:  kafkaSchema(),
			wantErr: "",
		},
		{
			name:    "no secrets block is valid",
			schema:  map[string]any{"properties": map[string]any{"host": map[string]any{"type": "string"}}},
			wantErr: "",
		},
		{
			name: "secrets block must be object",
			schema: map[string]any{
				"properties": map[string]any{
					"secrets": map[string]any{"type": "string", "readOnly": true},
				},
			},
			wantErr: "property 'secrets' must be an object",
		},
		{
			name: "secrets block is not required to be readOnly",
			schema: map[string]any{
				"properties": map[string]any{
					"secrets": map[string]any{"type": "object"},
				},
			},
			wantErr: "",
		},
		{
			name: "writable data sub-property is allowed (future input)",
			schema: map[string]any{
				"properties": map[string]any{
					"secrets": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"connectionString": map[string]any{"type": "string"},
						},
					},
				},
			},
			wantErr: "",
		},
		{
			name: "secret sub-property must be string",
			schema: map[string]any{
				"properties": map[string]any{
					"secrets": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"connectionString": map[string]any{"type": "object", "readOnly": true},
						},
					},
				},
			},
			wantErr: "secret 'secrets.connectionString' must be a string",
		},
		{
			name: "reserved name sub-property must be readOnly",
			schema: map[string]any{
				"properties": map[string]any{
					"secrets": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{"type": "string"},
						},
					},
				},
			},
			wantErr: "secret 'secrets.name' must be marked readOnly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecretsBlock(tt.schema)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.wantErr)
			}
		})
	}
}
