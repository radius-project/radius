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

func TestApplyDefaults(t *testing.T) {
	tests := []struct {
		name       string
		properties map[string]any
		schema     map[string]any
		expected   map[string]any
		applied    int
	}{
		{
			name:       "unset property takes the declared default",
			properties: map[string]any{"environment": "env"},
			schema: map[string]any{"properties": map[string]any{
				"environment": map[string]any{"type": "string"},
				"size":        map[string]any{"type": "string", "default": "S"},
			}},
			expected: map[string]any{"environment": "env", "size": "S"},
			applied:  1,
		},
		{
			name:       "explicit value is never overwritten",
			properties: map[string]any{"size": "L"},
			schema: map[string]any{"properties": map[string]any{
				"size": map[string]any{"type": "string", "default": "S"},
			}},
			expected: map[string]any{"size": "L"},
			applied:  0,
		},
		{
			name:       "explicit empty string is kept, not defaulted",
			properties: map[string]any{"database": ""},
			schema: map[string]any{"properties": map[string]any{
				"database": map[string]any{"type": "string", "default": "appdb"},
			}},
			expected: map[string]any{"database": ""},
			applied:  0,
		},
		{
			name:       "property without a default is left absent",
			properties: map[string]any{},
			schema: map[string]any{"properties": map[string]any{
				"application": map[string]any{"type": "string"},
			}},
			expected: map[string]any{},
			applied:  0,
		},
		{
			name:       "read only property is skipped",
			properties: map[string]any{},
			schema: map[string]any{"properties": map[string]any{
				"host": map[string]any{"type": "string", "default": "localhost", "readOnly": true},
			}},
			expected: map[string]any{},
			applied:  0,
		},
		{
			name:       "non string defaults are applied",
			properties: map[string]any{},
			schema: map[string]any{"properties": map[string]any{
				"port":     map[string]any{"type": "integer", "default": 5432},
				"replicas": map[string]any{"type": "integer", "default": 1},
				"enabled":  map[string]any{"type": "boolean", "default": true},
			}},
			expected: map[string]any{"port": 5432, "replicas": 1, "enabled": true},
			applied:  3,
		},
		{
			name:       "nested object present is recursed into",
			properties: map[string]any{"config": map[string]any{"tls": true}},
			schema: map[string]any{"properties": map[string]any{
				"config": map[string]any{"type": "object", "properties": map[string]any{
					"tls":  map[string]any{"type": "boolean"},
					"mode": map[string]any{"type": "string", "default": "require"},
				}},
			}},
			expected: map[string]any{"config": map[string]any{"tls": true, "mode": "require"}},
			applied:  1,
		},
		{
			name: "objects inside an array are recursed into",
			properties: map[string]any{"workers": []any{
				map[string]any{"name": "first"},
				map[string]any{"name": "second", "replicas": 2},
			}},
			schema: map[string]any{"properties": map[string]any{
				"workers": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":     map[string]any{"type": "string"},
							"replicas": map[string]any{"type": "integer", "default": 1},
						},
					},
				},
			}},
			expected: map[string]any{"workers": []any{
				map[string]any{"name": "first", "replicas": 1},
				map[string]any{"name": "second", "replicas": 2},
			}},
			applied: 1,
		},
		{
			name:       "new object default is recursed into",
			properties: map[string]any{},
			schema: map[string]any{"properties": map[string]any{
				"config": map[string]any{
					"type":    "object",
					"default": map[string]any{"name": "given"},
					"properties": map[string]any{
						"name":    map[string]any{"type": "string"},
						"retries": map[string]any{"type": "integer", "default": 3},
					},
				},
			}},
			expected: map[string]any{"config": map[string]any{"name": "given", "retries": 3}},
			applied:  2,
		},
		{
			name:       "new array default is recursed into",
			properties: map[string]any{},
			schema: map[string]any{"properties": map[string]any{
				"workers": map[string]any{
					"type":    "array",
					"default": []any{map[string]any{"name": "first"}},
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":     map[string]any{"type": "string"},
							"replicas": map[string]any{"type": "integer", "default": 1},
						},
					},
				},
			}},
			expected: map[string]any{"workers": []any{
				map[string]any{"name": "first", "replicas": 1},
			}},
			applied: 2,
		},
		{
			name: "additional property values are recursed into",
			properties: map[string]any{"regions": map[string]any{
				"east": map[string]any{"name": "primary"},
				"west": map[string]any{"name": "secondary", "replicas": 3},
			}},
			schema: map[string]any{"properties": map[string]any{
				"regions": map[string]any{
					"type": "object",
					"additionalProperties": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":     map[string]any{"type": "string"},
							"replicas": map[string]any{"type": "integer", "default": 1},
						},
					},
				},
			}},
			expected: map[string]any{"regions": map[string]any{
				"east": map[string]any{"name": "primary", "replicas": 1},
				"west": map[string]any{"name": "secondary", "replicas": 3},
			}},
			applied: 1,
		},
		{
			name: "additional properties schema is not applied to declared properties",
			properties: map[string]any{"settings": map[string]any{
				"fixed":  map[string]any{},
				"custom": map[string]any{},
			}},
			schema: map[string]any{"properties": map[string]any{
				"settings": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"fixed": map[string]any{"type": "object"},
					},
					"additionalProperties": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"enabled": map[string]any{"type": "boolean", "default": true},
						},
					},
				},
			}},
			expected: map[string]any{"settings": map[string]any{
				"fixed":  map[string]any{},
				"custom": map[string]any{"enabled": true},
			}},
			applied: 1,
		},
		{
			name:       "absent object is not created to hold defaults",
			properties: map[string]any{},
			schema: map[string]any{"properties": map[string]any{
				"config": map[string]any{"type": "object", "properties": map[string]any{
					"mode": map[string]any{"type": "string", "default": "require"},
				}},
			}},
			expected: map[string]any{},
			applied:  0,
		},
		{
			name:       "schema without properties is a no-op",
			properties: map[string]any{"size": "S"},
			schema:     map[string]any{"type": "object"},
			expected:   map[string]any{"size": "S"},
			applied:    0,
		},
		{
			name:       "required property is not defaulted",
			properties: map[string]any{"environment": "env"},
			schema: map[string]any{
				"required": []any{"environment", "database"},
				"properties": map[string]any{
					"environment": map[string]any{"type": "string"},
					"database":    map[string]any{"type": "string", "default": "appdb"},
				},
			},
			expected: map[string]any{"environment": "env"},
			applied:  0,
		},
		{
			name:       "required list given as strings is honoured",
			properties: map[string]any{},
			schema: map[string]any{
				"required":   []string{"database"},
				"properties": map[string]any{"database": map[string]any{"type": "string", "default": "appdb"}},
			},
			expected: map[string]any{},
			applied:  0,
		},
		{
			name:       "optional property is still defaulted alongside a required one",
			properties: map[string]any{"environment": "env"},
			schema: map[string]any{
				"required": []any{"environment"},
				"properties": map[string]any{
					"environment": map[string]any{"type": "string"},
					"size":        map[string]any{"type": "string", "default": "S"},
				},
			},
			expected: map[string]any{"environment": "env", "size": "S"},
			applied:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applied := ApplyDefaults(tt.properties, tt.schema)
			require.Equal(t, tt.applied, applied)
			require.Equal(t, tt.expected, tt.properties)
		})
	}
}

func TestApplyDefaults_NilInputs(t *testing.T) {
	require.Equal(t, 0, ApplyDefaults(nil, map[string]any{"properties": map[string]any{}}))
	require.Equal(t, 0, ApplyDefaults(map[string]any{}, nil))
}

func TestApplyDefaults_DefaultsAreCopied(t *testing.T) {
	schemaData := map[string]any{"properties": map[string]any{
		"tags":    map[string]any{"type": "object", "default": map[string]any{"tier": "free"}},
		"origins": map[string]any{"type": "array", "default": []any{"a"}},
	}}

	first := map[string]any{}
	require.Equal(t, 2, ApplyDefaults(first, schemaData))

	first["tags"].(map[string]any)["tier"] = "paid"
	first["origins"].([]any)[0] = "b"

	second := map[string]any{}
	require.Equal(t, 2, ApplyDefaults(second, schemaData))
	require.Equal(t, map[string]any{"tier": "free"}, second["tags"])
	require.Equal(t, []any{"a"}, second["origins"])
}

// These shapes match resource-types-contrib schemas.
func TestApplyDefaults_ContribShapes(t *testing.T) {
	schemaData := map[string]any{"properties": map[string]any{
		"environment":   map[string]any{"type": "string"},
		"application":   map[string]any{"type": "string"},
		"size":          map[string]any{"type": "string", "enum": []any{"S", "M", "L"}, "default": "S"},
		"containerName": map[string]any{"type": "string", "default": "data"},
		"host":          map[string]any{"type": "string", "readOnly": true},
	}}

	properties := map[string]any{"environment": "/planes/radius/local/resourcegroups/default/providers/Radius.Core/environments/test"}
	require.Equal(t, 2, ApplyDefaults(properties, schemaData))
	require.Equal(t, "S", properties["size"])
	require.Equal(t, "data", properties["containerName"])
	require.NotContains(t, properties, "host")
	require.NotContains(t, properties, "application")
}

func TestApplyDefaults_RequiredWinsOverDeclaredDefault(t *testing.T) {
	schemaData := map[string]any{
		"required": []any{"environment", "database", "username", "password"},
		"properties": map[string]any{
			"environment": map[string]any{"type": "string"},
			"database":    map[string]any{"type": "string", "default": "appdb"},
			"application": map[string]any{"type": "string"},
		},
	}

	properties := map[string]any{"environment": "env"}
	require.Equal(t, 0, ApplyDefaults(properties, schemaData))
	require.NotContains(t, properties, "database")
}

func TestApplyDefaults_NestedDefaultsInsideSuppliedRequiredObject(t *testing.T) {
	schemaData := map[string]any{
		"required": []any{"config"},
		"properties": map[string]any{
			"config": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"mode":    map[string]any{"type": "string", "default": "fast"},
					"retries": map[string]any{"type": "integer", "default": 3},
					"name":    map[string]any{"type": "string"},
				},
			},
		},
	}

	properties := map[string]any{"config": map[string]any{"name": "given"}}
	require.Equal(t, 2, ApplyDefaults(properties, schemaData))

	config := properties["config"].(map[string]any)
	require.Equal(t, "fast", config["mode"])
	require.Equal(t, 3, config["retries"])
	require.Equal(t, "given", config["name"])
}

func TestApplyDefaults_AbsentRequiredObjectIsNotCreated(t *testing.T) {
	schemaData := map[string]any{
		"required": []any{"config"},
		"properties": map[string]any{
			"config": map[string]any{
				"type":       "object",
				"default":    map[string]any{"mode": "fast"},
				"properties": map[string]any{"mode": map[string]any{"type": "string", "default": "fast"}},
			},
		},
	}

	properties := map[string]any{}
	require.Equal(t, 0, ApplyDefaults(properties, schemaData))
	require.NotContains(t, properties, "config")
}
