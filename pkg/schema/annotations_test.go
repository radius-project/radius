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
	"context"
	"net/http"
	"testing"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview/fake"
	"github.com/stretchr/testify/require"
)

func TestExtractSensitiveFieldPaths(t *testing.T) {
	tests := []struct {
		name     string
		schema   map[string]any
		expected []string
	}{
		{
			name:     "empty schema",
			schema:   map[string]any{},
			expected: []string{},
		},
		{
			name: "no sensitive fields",
			schema: map[string]any{
				"properties": map[string]any{
					"name": map[string]any{
						"type": "string",
					},
					"port": map[string]any{
						"type": "integer",
					},
				},
			},
			expected: []string{},
		},
		{
			name: "single sensitive field",
			schema: map[string]any{
				"properties": map[string]any{
					"name": map[string]any{
						"type": "string",
					},
					"password": map[string]any{
						"type":                    "string",
						annotationRadiusSensitive: true,
					},
				},
			},
			expected: []string{"password"},
		},
		{
			name: "multiple sensitive fields",
			schema: map[string]any{
				"properties": map[string]any{
					"username": map[string]any{
						"type": "string",
					},
					"password": map[string]any{
						"type":                    "string",
						annotationRadiusSensitive: true,
					},
					"apiKey": map[string]any{
						"type":                    "string",
						annotationRadiusSensitive: true,
					},
				},
			},
			expected: []string{"password", "apiKey"},
		},
		{
			name: "nested sensitive field",
			schema: map[string]any{
				"properties": map[string]any{
					"name": map[string]any{
						"type": "string",
					},
					"credentials": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"username": map[string]any{
								"type": "string",
							},
							"password": map[string]any{
								"type":                    "string",
								annotationRadiusSensitive: true,
							},
						},
					},
				},
			},
			expected: []string{"credentials.password"},
		},
		{
			name: "deeply nested sensitive fields",
			schema: map[string]any{
				"properties": map[string]any{
					"config": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"database": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"connectionString": map[string]any{
										"type":                    "string",
										annotationRadiusSensitive: true,
									},
									"host": map[string]any{
										"type": "string",
									},
								},
							},
						},
					},
				},
			},
			expected: []string{"config.database.connectionString"},
		},
		{
			name: "mixed sensitive fields at different levels",
			schema: map[string]any{
				"properties": map[string]any{
					"apiKey": map[string]any{
						"type":                    "string",
						annotationRadiusSensitive: true,
					},
					"settings": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"token": map[string]any{
								"type":                    "string",
								annotationRadiusSensitive: true,
							},
							"endpoint": map[string]any{
								"type": "string",
							},
						},
					},
				},
			},
			expected: []string{"apiKey", "settings.token"},
		},
		{
			name: "sensitive annotation set to false",
			schema: map[string]any{
				"properties": map[string]any{
					"password": map[string]any{
						"type":                    "string",
						annotationRadiusSensitive: false,
					},
				},
			},
			expected: []string{},
		},
		{
			name: "array with sensitive items",
			schema: map[string]any{
				"properties": map[string]any{
					"secrets": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type":                    "string",
							annotationRadiusSensitive: true,
						},
					},
				},
			},
			expected: []string{"secrets[*]"},
		},
		{
			name: "array with nested sensitive field in items",
			schema: map[string]any{
				"properties": map[string]any{
					"credentials": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"username": map[string]any{
									"type": "string",
								},
								"password": map[string]any{
									"type":                    "string",
									annotationRadiusSensitive: true,
								},
							},
						},
					},
				},
			},
			expected: []string{"credentials[*].password"},
		},
		{
			name: "array with deeply nested sensitive field",
			schema: map[string]any{
				"properties": map[string]any{
					"connections": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"database": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"connectionString": map[string]any{
											"type":                    "string",
											annotationRadiusSensitive: true,
										},
									},
								},
							},
						},
					},
				},
			},
			expected: []string{"connections[*].database.connectionString"},
		},
		{
			name: "additionalProperties with sensitive values",
			schema: map[string]any{
				"properties": map[string]any{
					"envVars": map[string]any{
						"type": "object",
						"additionalProperties": map[string]any{
							"type":                    "string",
							annotationRadiusSensitive: true,
						},
					},
				},
			},
			expected: []string{"envVars[*]"},
		},
		{
			name: "additionalProperties with nested sensitive field",
			schema: map[string]any{
				"properties": map[string]any{
					"secrets": map[string]any{
						"type": "object",
						"additionalProperties": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"value": map[string]any{
									"type":                    "string",
									annotationRadiusSensitive: true,
								},
								"version": map[string]any{
									"type": "string",
								},
							},
						},
					},
				},
			},
			expected: []string{"secrets[*].value"},
		},
		{
			name: "mixed array and additionalProperties with sensitive fields",
			schema: map[string]any{
				"properties": map[string]any{
					"apiKey": map[string]any{
						"type":                    "string",
						annotationRadiusSensitive: true,
					},
					"tokens": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type":                    "string",
							annotationRadiusSensitive: true,
						},
					},
					"secretMap": map[string]any{
						"type": "object",
						"additionalProperties": map[string]any{
							"type":                    "string",
							annotationRadiusSensitive: true,
						},
					},
					"config": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"password": map[string]any{
								"type":                    "string",
								annotationRadiusSensitive: true,
							},
						},
					},
				},
			},
			expected: []string{"apiKey", "tokens[*]", "secretMap[*]", "config.password"},
		},
		{
			name: "array items without sensitive annotation",
			schema: map[string]any{
				"properties": map[string]any{
					"names": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "string",
						},
					},
				},
			},
			expected: []string{},
		},
		{
			name: "additionalProperties without sensitive annotation",
			schema: map[string]any{
				"properties": map[string]any{
					"labels": map[string]any{
						"type": "object",
						"additionalProperties": map[string]any{
							"type": "string",
						},
					},
				},
			},
			expected: []string{},
		},
		{
			name: "sensitive object skips nested properties",
			schema: map[string]any{
				"properties": map[string]any{
					"credentials": map[string]any{
						"type":                    "object",
						annotationRadiusSensitive: true,
						"properties": map[string]any{
							"username": map[string]any{
								"type": "string",
							},
							"password": map[string]any{
								"type":                    "string",
								annotationRadiusSensitive: true,
							},
						},
					},
				},
			},
			expected: []string{"credentials"},
		},
		{
			name: "sensitive array skips nested item properties",
			schema: map[string]any{
				"properties": map[string]any{
					"secrets": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type":                    "object",
							annotationRadiusSensitive: true,
							"properties": map[string]any{
								"key": map[string]any{
									"type": "string",
								},
								"value": map[string]any{
									"type":                    "string",
									annotationRadiusSensitive: true,
								},
							},
						},
					},
				},
			},
			expected: []string{"secrets[*]"},
		},
		{
			name: "sensitive additionalProperties skips nested value properties",
			schema: map[string]any{
				"properties": map[string]any{
					"secretMap": map[string]any{
						"type": "object",
						"additionalProperties": map[string]any{
							"type":                    "object",
							annotationRadiusSensitive: true,
							"properties": map[string]any{
								"data": map[string]any{
									"type": "string",
								},
								"secret": map[string]any{
									"type":                    "string",
									annotationRadiusSensitive: true,
								},
							},
						},
					},
				},
			},
			expected: []string{"secretMap[*]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractSensitiveFieldPaths(tt.schema, "")

			// Sort both slices for comparison since map iteration order is not guaranteed
			require.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestExtractSensitiveFieldPaths_WithPrefix(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"secret": map[string]any{
				"type":                    "string",
				annotationRadiusSensitive: true,
			},
		},
	}

	result := ExtractSensitiveFieldPaths(schema, "parent")

	require.Equal(t, []string{"parent.secret"}, result)
}

func TestGetSensitiveFieldPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("nil client returns nil", func(t *testing.T) {
		result, err := GetSensitiveFieldPaths(ctx, nil, "/planes/radius/local/resourceGroups/test/providers/Foo.Bar/myResources/test", "Foo.Bar/myResources", "2024-01-01")
		require.NoError(t, err)
		require.Nil(t, result)
	})

	t.Run("extracts sensitive fields from schema", func(t *testing.T) {
		clientFactory, err := testUCPClientFactory(map[string]any{
			"properties": map[string]any{
				"host": map[string]any{
					"type": "string",
				},
				"password": map[string]any{
					"type":                    "string",
					annotationRadiusSensitive: true,
				},
			},
		})
		require.NoError(t, err)

		result, err := GetSensitiveFieldPaths(
			ctx,
			clientFactory,
			"/planes/radius/local/resourceGroups/test/providers/Foo.Bar/myResources/test",
			"Foo.Bar/myResources",
			"2024-01-01",
		)

		require.NoError(t, err)
		require.Equal(t, []string{"password"}, result)
	})

	t.Run("returns empty for schema with no sensitive fields", func(t *testing.T) {
		clientFactory, err := testUCPClientFactory(map[string]any{
			"properties": map[string]any{
				"host": map[string]any{
					"type": "string",
				},
				"port": map[string]any{
					"type": "integer",
				},
			},
		})
		require.NoError(t, err)

		result, err := GetSensitiveFieldPaths(
			ctx,
			clientFactory,
			"/planes/radius/local/resourceGroups/test/providers/Foo.Bar/myResources/test",
			"Foo.Bar/myResources",
			"2024-01-01",
		)

		require.NoError(t, err)
		require.Empty(t, result)
	})

	t.Run("returns nil for nil schema", func(t *testing.T) {
		clientFactory, err := testUCPClientFactory(nil)
		require.NoError(t, err)

		result, err := GetSensitiveFieldPaths(
			ctx,
			clientFactory,
			"/planes/radius/local/resourceGroups/test/providers/Foo.Bar/myResources/test",
			"Foo.Bar/myResources",
			"2024-01-01",
		)

		require.NoError(t, err)
		require.Nil(t, result)
	})
}

func TestGetSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("nil client returns nil", func(t *testing.T) {
		result, err := GetSchema(ctx, nil, "/planes/radius/local/resourceGroups/test/providers/Foo.Bar/myResources/test", "Foo.Bar/myResources", "2024-01-01")
		require.NoError(t, err)
		require.Nil(t, result)
	})

	t.Run("returns schema from UCP", func(t *testing.T) {
		expectedSchema := map[string]any{
			"properties": map[string]any{
				"password": map[string]any{
					"type":                    "string",
					annotationRadiusSensitive: true,
				},
			},
		}

		clientFactory, err := testUCPClientFactory(expectedSchema)
		require.NoError(t, err)

		result, err := GetSchema(
			ctx,
			clientFactory,
			"/planes/radius/local/resourceGroups/test/providers/Foo.Bar/myResources/test",
			"Foo.Bar/myResources",
			"2024-01-01",
		)

		require.NoError(t, err)
		require.Equal(t, expectedSchema, result)
	})
}

// testUCPClientFactory creates a mock UCP client factory that returns the given schema.
func testUCPClientFactory(schema map[string]any) (*v20231001preview.ClientFactory, error) {
	apiVersionsServer := fake.APIVersionsServer{
		Get: func(ctx context.Context, planeName string, resourceProviderName string, resourceTypeName string, apiVersionName string, options *v20231001preview.APIVersionsClientGetOptions) (azfake.Responder[v20231001preview.APIVersionsClientGetResponse], azfake.ErrorResponder) {
			resp := azfake.Responder[v20231001preview.APIVersionsClientGetResponse]{}
			resp.SetResponse(http.StatusOK, v20231001preview.APIVersionsClientGetResponse{
				APIVersionResource: v20231001preview.APIVersionResource{
					Properties: &v20231001preview.APIVersionProperties{
						Schema: schema,
					},
				},
			}, nil)
			return resp, azfake.ErrorResponder{}
		},
	}

	return v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: fake.NewServerFactoryTransport(&fake.ServerFactory{
				APIVersionsServer: apiVersionsServer,
			}),
		},
	})
}

func TestGetSensitiveFieldPaths_InvalidResourceID(t *testing.T) {
	clientFactory, err := testUCPClientFactory(nil)
	require.NoError(t, err)

	_, err = GetSensitiveFieldPaths(
		context.Background(),
		clientFactory,
		"invalid-resource-id",
		"Foo.Bar/myResources",
		"2024-01-01",
	)

	require.Error(t, err)
}

// =============================================================================
// ParseFieldPath tests
// =============================================================================

func TestParseFieldPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []FieldPathSegment
	}{
		{
			name:     "simple field",
			path:     "data",
			expected: []FieldPathSegment{{Type: SegmentTypeField, Value: "data"}},
		},
		{
			name: "nested dot path",
			path: "credentials.password",
			expected: []FieldPathSegment{
				{Type: SegmentTypeField, Value: "credentials"},
				{Type: SegmentTypeField, Value: "password"},
			},
		},
		{
			name: "deeply nested",
			path: "config.database.connection.password",
			expected: []FieldPathSegment{
				{Type: SegmentTypeField, Value: "config"},
				{Type: SegmentTypeField, Value: "database"},
				{Type: SegmentTypeField, Value: "connection"},
				{Type: SegmentTypeField, Value: "password"},
			},
		},
		{
			name: "array wildcard",
			path: "secrets[*].value",
			expected: []FieldPathSegment{
				{Type: SegmentTypeField, Value: "secrets"},
				{Type: SegmentTypeWildcard},
				{Type: SegmentTypeField, Value: "value"},
			},
		},
		{
			name: "map wildcard",
			path: "config[*]",
			expected: []FieldPathSegment{
				{Type: SegmentTypeField, Value: "config"},
				{Type: SegmentTypeWildcard},
			},
		},
		{
			name: "specific index",
			path: "items[0].name",
			expected: []FieldPathSegment{
				{Type: SegmentTypeField, Value: "items"},
				{Type: SegmentTypeIndex, Value: "0"},
				{Type: SegmentTypeField, Value: "name"},
			},
		},
		{
			name: "multiple wildcards",
			path: "data[*].secrets[*].value",
			expected: []FieldPathSegment{
				{Type: SegmentTypeField, Value: "data"},
				{Type: SegmentTypeWildcard},
				{Type: SegmentTypeField, Value: "secrets"},
				{Type: SegmentTypeWildcard},
				{Type: SegmentTypeField, Value: "value"},
			},
		},
		{
			name: "wildcard with nested field",
			path: "backends[*].token",
			expected: []FieldPathSegment{
				{Type: SegmentTypeField, Value: "backends"},
				{Type: SegmentTypeWildcard},
				{Type: SegmentTypeField, Value: "token"},
			},
		},
		{
			name:     "empty path",
			path:     "",
			expected: nil,
		},
		{
			name:     "unterminated bracket",
			path:     "secrets[*",
			expected: nil,
		},
		{
			name:     "unterminated bracket with index",
			path:     "items[0",
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ParseFieldPath(tc.path)
			require.Equal(t, tc.expected, result)
		})
	}
}

// =============================================================================
// RedactFields tests
// =============================================================================

func TestRedactFields_SimpleField(t *testing.T) {
	properties := map[string]any{
		"name":     "test-resource",
		"password": "secret123",
		"data":     map[string]any{"key": "value"},
	}

	RedactFields(properties, []string{"password"})

	require.Equal(t, "test-resource", properties["name"])
	require.Nil(t, properties["password"])
	require.NotNil(t, properties["data"])
}

func TestRedactFields_DataField(t *testing.T) {
	properties := map[string]any{
		"environment": "/planes/radius/local/resourcegroups/default/providers/Radius.Core/environments/test",
		"application": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/applications/test",
		"data": map[string]any{
			"password": map[string]any{
				"value":    "secret123",
				"encoding": "string",
			},
			"apiKey": map[string]any{
				"value": "my-api-key",
			},
		},
	}

	RedactFields(properties, []string{"data"})

	require.NotNil(t, properties["environment"])
	require.NotNil(t, properties["application"])
	require.Nil(t, properties["data"])
}

func TestRedactFields_NonExistentField(t *testing.T) {
	properties := map[string]any{
		"name":  "test-resource",
		"value": "test-value",
	}

	RedactFields(properties, []string{"nonexistent"})

	require.Equal(t, "test-resource", properties["name"])
	require.Equal(t, "test-value", properties["value"])
}

func TestRedactFields_NilProperties(t *testing.T) {
	var properties map[string]any

	// Should not panic
	RedactFields(properties, []string{"anyfield"})
}

func TestRedactFields_EmptyProperties(t *testing.T) {
	properties := map[string]any{}

	RedactFields(properties, []string{"password"})

	require.Empty(t, properties)
}

func TestRedactFields_MultipleFields(t *testing.T) {
	properties := map[string]any{
		"name":     "test",
		"password": "secret",
		"apiKey":   "key123",
		"data":     "sensitive-data",
	}

	RedactFields(properties, []string{"password", "apiKey", "data"})

	require.Equal(t, "test", properties["name"])
	require.Nil(t, properties["password"])
	require.Nil(t, properties["apiKey"])
	require.Nil(t, properties["data"])
}

func TestRedactFields_NestedDotPath(t *testing.T) {
	properties := map[string]any{
		"config": map[string]any{
			"password": "secret",
			"host":     "localhost",
		},
	}

	RedactFields(properties, []string{"config.password"})

	config, ok := properties["config"].(map[string]any)
	require.True(t, ok)
	require.Nil(t, config["password"])
	require.Equal(t, "localhost", config["host"])
}

func TestRedactFields_DeeplyNestedDotPath(t *testing.T) {
	properties := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"secret": "top-secret",
				"other":  "keep-this",
			},
		},
	}

	RedactFields(properties, []string{"level1.level2.secret"})

	level1 := properties["level1"].(map[string]any)
	level2 := level1["level2"].(map[string]any)
	require.Nil(t, level2["secret"])
	require.Equal(t, "keep-this", level2["other"])
}

func TestRedactFields_ArrayWildcard(t *testing.T) {
	properties := map[string]any{
		"secrets": []any{
			map[string]any{"name": "secret1", "value": "s1"},
			map[string]any{"name": "secret2", "value": "s2"},
		},
	}

	RedactFields(properties, []string{"secrets[*].value"})

	secrets := properties["secrets"].([]any)
	s0 := secrets[0].(map[string]any)
	s1 := secrets[1].(map[string]any)
	require.Nil(t, s0["value"])
	require.Nil(t, s1["value"])
	require.Equal(t, "secret1", s0["name"])
	require.Equal(t, "secret2", s1["name"])
}

func TestRedactFields_MapWildcard(t *testing.T) {
	properties := map[string]any{
		"config": map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	}

	RedactFields(properties, []string{"config[*]"})

	config := properties["config"].(map[string]any)
	require.Nil(t, config["key1"])
	require.Nil(t, config["key2"])
}

func TestRedactFields_MapWildcardWithNestedField(t *testing.T) {
	properties := map[string]any{
		"backends": map[string]any{
			"kv":    map[string]any{"url": "http://vault", "token": "secret-token"},
			"azure": map[string]any{"url": "http://azure", "token": "azure-token"},
		},
	}

	RedactFields(properties, []string{"backends[*].token"})

	backends := properties["backends"].(map[string]any)
	kv := backends["kv"].(map[string]any)
	azure := backends["azure"].(map[string]any)
	require.Nil(t, kv["token"])
	require.Nil(t, azure["token"])
	require.Equal(t, "http://vault", kv["url"])
	require.Equal(t, "http://azure", azure["url"])
}

func TestRedactFields_NestedPathFieldNotFound(t *testing.T) {
	properties := map[string]any{
		"config": map[string]any{
			"host": "localhost",
		},
	}

	RedactFields(properties, []string{"config.nonexistent"})

	config := properties["config"].(map[string]any)
	require.Equal(t, "localhost", config["host"])
}

func TestRedactFields_EmptyPath(t *testing.T) {
	properties := map[string]any{
		"data": "value",
	}

	RedactFields(properties, []string{""})

	require.Equal(t, "value", properties["data"])
}

func TestRedactFields_ArrayWildcardAllElements(t *testing.T) {
	properties := map[string]any{
		"tokens": []any{"token1", "token2", "token3"},
	}

	RedactFields(properties, []string{"tokens[*]"})

	tokens := properties["tokens"].([]any)
	for _, token := range tokens {
		require.Nil(t, token)
	}
}

func TestRedactFields_FieldWithNilValue(t *testing.T) {
	properties := map[string]any{
		"name":     "test",
		"password": nil,
	}

	RedactFields(properties, []string{"password"})

	require.Equal(t, "test", properties["name"])
	require.Nil(t, properties["password"])
}

func TestRedactFields_AlreadyNilIdempotent(t *testing.T) {
	data := map[string]any{
		"secret":   nil,
		"username": "admin",
	}

	RedactFields(data, []string{"secret"})
	require.Nil(t, data["secret"])
	require.Equal(t, "admin", data["username"])

	// Redacting again is still a no-op
	RedactFields(data, []string{"secret"})
	require.Nil(t, data["secret"])
}

func TestRedactFields_SpecificIndex(t *testing.T) {
	properties := map[string]any{
		"items": []any{
			map[string]any{"name": "public", "password": "keep-this"},
			map[string]any{"name": "secret", "password": "redact-this"},
			map[string]any{"name": "public2", "password": "keep-this-too"},
		},
	}

	RedactFields(properties, []string{"items[1].password"})

	items := properties["items"].([]any)
	// First and third items should be unchanged
	require.Equal(t, "keep-this", items[0].(map[string]any)["password"])
	require.Equal(t, "keep-this-too", items[2].(map[string]any)["password"])
	// Second item's password should be redacted
	require.Nil(t, items[1].(map[string]any)["password"])
	// Names should be untouched
	require.Equal(t, "public", items[0].(map[string]any)["name"])
	require.Equal(t, "secret", items[1].(map[string]any)["name"])
	require.Equal(t, "public2", items[2].(map[string]any)["name"])
}

func TestRedactFields_SpecificIndex_OutOfBounds(t *testing.T) {
	properties := map[string]any{
		"items": []any{
			map[string]any{"value": "keep"},
		},
	}

	// Index 5 is out of bounds — should silently skip, no panic
	RedactFields(properties, []string{"items[5].value"})

	items := properties["items"].([]any)
	require.Equal(t, "keep", items[0].(map[string]any)["value"])
}

func TestRedactFields_SpecificIndex_BareElement(t *testing.T) {
	properties := map[string]any{
		"tokens": []any{"token0", "token1", "token2"},
	}

	// Redact the second element directly
	RedactFields(properties, []string{"tokens[1]"})

	tokens := properties["tokens"].([]any)
	require.Equal(t, "token0", tokens[0])
	require.Nil(t, tokens[1])
	require.Equal(t, "token2", tokens[2])
}

func TestRedactFields_FieldWithDifferentTypes(t *testing.T) {
	testCases := []struct {
		name       string
		fieldName  string
		properties map[string]any
	}{
		{
			name:       "string field",
			fieldName:  "secret",
			properties: map[string]any{"secret": "password123"},
		},
		{
			name:       "map field",
			fieldName:  "data",
			properties: map[string]any{"data": map[string]any{"key": "value"}},
		},
		{
			name:       "slice field",
			fieldName:  "tokens",
			properties: map[string]any{"tokens": []string{"token1", "token2"}},
		},
		{
			name:       "int field",
			fieldName:  "pin",
			properties: map[string]any{"pin": 1234},
		},
		{
			name:       "bool field",
			fieldName:  "enabled",
			properties: map[string]any{"enabled": true},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RedactFields(tc.properties, []string{tc.fieldName})
			require.Nil(t, tc.properties[tc.fieldName])
		})
	}
}
