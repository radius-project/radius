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
