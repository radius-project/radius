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

package manifest

import (
	"context"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestResourceProviderNamespaceValidation(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		valid     bool
	}{
		{
			name:      "valid namespace",
			namespace: "MyCompany.Resources",
			valid:     true,
		},
		{
			name:      "valid namespace with numbers",
			namespace: "Company1.Resources2",
			valid:     true,
		},
		{
			name:      "invalid - lowercase first part",
			namespace: "myCompany.Resources",
			valid:     false,
		},
		{
			name:      "invalid - lowercase second part",
			namespace: "MyCompany.resources",
			valid:     false,
		},
		{
			name:      "invalid - no dot",
			namespace: "MyCompanyResources",
			valid:     false,
		},
		{
			name:      "invalid - starts with number",
			namespace: "1Company.Resources",
			valid:     false,
		},
		{
			name:      "invalid - special characters",
			namespace: "My-Company.Resources",
			valid:     false,
		},
		{
			name:      "invalid - empty",
			namespace: "",
			valid:     false,
		},
	}

	// Create a mock validator to test the validation function
	v := validator.New()
	err := v.RegisterValidation("resourceProviderNamespace", resourceProviderNamespace)
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test struct to validate
			testStruct := struct {
				Name string `validate:"resourceProviderNamespace"`
			}{
				Name: tt.namespace,
			}

			err := v.Struct(testStruct)
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestResourceTypeValidation(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		valid        bool
	}{
		{
			name:         "valid resource type",
			resourceType: "widgets",
			valid:        true,
		},
		{
			name:         "valid with numbers",
			resourceType: "widgets2",
			valid:        true,
		},
		{
			name:         "valid mixed case",
			resourceType: "myWidgets",
			valid:        true,
		},
		{
			name:         "invalid - starts with uppercase",
			resourceType: "Widgets",
			valid:        false,
		},
		{
			name:         "invalid - starts with number",
			resourceType: "2widgets",
			valid:        false,
		},
		{
			name:         "invalid - special characters",
			resourceType: "my-widgets",
			valid:        false,
		},
		{
			name:         "invalid - empty",
			resourceType: "",
			valid:        false,
		},
	}

	v := validator.New()
	err := v.RegisterValidation("resourceType", validateResourceType)
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testStruct := struct {
				Type string `validate:"resourceType"`
			}{
				Type: tt.resourceType,
			}

			err := v.Struct(testStruct)
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestAPIVersionValidation(t *testing.T) {
	tests := []struct {
		name       string
		apiVersion string
		valid      bool
	}{
		{
			name:       "valid api version",
			apiVersion: "2023-10-01",
			valid:      true,
		},
		{
			name:       "valid preview version",
			apiVersion: "2023-10-01-preview",
			valid:      true,
		},
		{
			name:       "invalid format - no dashes",
			apiVersion: "20231001",
			valid:      false,
		},
		{
			name:       "invalid format - wrong date format",
			apiVersion: "23-10-01",
			valid:      false,
		},
		{
			name:       "invalid format - invalid preview suffix",
			apiVersion: "2023-10-01-beta",
			valid:      false,
		},
		{
			name:       "invalid - empty",
			apiVersion: "",
			valid:      false,
		},
	}

	v := validator.New()
	err := v.RegisterValidation("apiVersion", validateAPIVersion)
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testStruct := struct {
				Version string `validate:"apiVersion"`
			}{
				Version: tt.apiVersion,
			}

			err := v.Struct(testStruct)
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestCapabilityValidation(t *testing.T) {
	tests := []struct {
		name       string
		capability string
		valid      bool
	}{
		{
			name:       "valid capability",
			capability: "Synchronous",
			valid:      true,
		},
		{
			name:       "valid with numbers",
			capability: "Sync2",
			valid:      true,
		},
		{
			name:       "invalid - starts with lowercase",
			capability: "synchronous",
			valid:      false,
		},
		{
			name:       "invalid - starts with number",
			capability: "2Sync",
			valid:      false,
		},
		{
			name:       "invalid - special characters",
			capability: "Sync-Async",
			valid:      false,
		},
		{
			name:       "invalid - empty",
			capability: "",
			valid:      false,
		},
	}

	v := validator.New()
	err := v.RegisterValidation("capability", validateCapability)
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testStruct := struct {
				Cap string `validate:"capability"`
			}{
				Cap: tt.capability,
			}

			err := v.Struct(testStruct)
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestValidateManifestSchemas(t *testing.T) {
	ctx := context.Background()

	t.Run("nil provider", func(t *testing.T) {
		err := validateManifestSchemas(ctx, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "provider is nil")
	})

	t.Run("provider with no resource types", func(t *testing.T) {
		provider := &ResourceProvider{
			Name:  "Test.Provider",
			Types: map[string]*ResourceType{},
		}
		err := validateManifestSchemas(ctx, provider)
		require.NoError(t, err) // Empty types should be valid
	})

	t.Run("provider with valid schemas", func(t *testing.T) {
		provider := &ResourceProvider{
			Name: "Test.Provider",
			Types: map[string]*ResourceType{
				"widgets": {
					APIVersions: map[string]*ResourceTypeAPIVersion{
						"2023-10-01": {
							Schema: map[string]any{
								"type": "object",
								"properties": map[string]any{
									"name": map[string]any{
										"type": "string",
									},
									"count": map[string]any{
										"type": "integer",
									},
									"environment": map[string]any{
										"type": "string",
									},
								},
							},
						},
					},
				},
			},
		}
		err := validateManifestSchemas(ctx, provider)
		require.NoError(t, err)
	})

	t.Run("provider with invalid schema - unsupported type", func(t *testing.T) {
		provider := &ResourceProvider{
			Name: "Test.Provider",
			Types: map[string]*ResourceType{
				"widgets": {
					APIVersions: map[string]*ResourceTypeAPIVersion{
						"2023-10-01": {
							Schema: map[string]any{
								"type": "invalidtype", // Not supported
								"items": map[string]any{
									"type": "string",
								},
							},
						},
					},
				},
			},
		}
		err := validateManifestSchemas(ctx, provider)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported type: invalidtype")
		require.Contains(t, err.Error(), "Test.Provider/widgets@2023-10-01")
	})

	t.Run("provider with invalid schema - prohibited feature", func(t *testing.T) {
		provider := &ResourceProvider{
			Name: "Test.Provider",
			Types: map[string]*ResourceType{
				"widgets": {
					APIVersions: map[string]*ResourceTypeAPIVersion{
						"2023-10-01": {
							Schema: map[string]any{
								"allOf": []map[string]any{
									{"type": "string"},
									{"type": "object"},
								},
							},
						},
					},
				},
			},
		}
		err := validateManifestSchemas(ctx, provider)
		require.Error(t, err)
		require.Contains(t, err.Error(), "allOf is not supported")
	})

	t.Run("provider with invalid JSON schema", func(t *testing.T) {
		provider := &ResourceProvider{
			Name: "Test.Provider",
			Types: map[string]*ResourceType{
				"widgets": {
					APIVersions: map[string]*ResourceTypeAPIVersion{
						"2023-10-01": {
							Schema: func() {}, // Cannot be marshaled to JSON
						},
					},
				},
			},
		}
		err := validateManifestSchemas(ctx, provider)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to parse schema")
	})

	t.Run("provider with nil schema", func(t *testing.T) {
		provider := &ResourceProvider{
			Name: "Test.Provider",
			Types: map[string]*ResourceType{
				"widgets": {
					APIVersions: map[string]*ResourceTypeAPIVersion{
						"2023-10-01": {
							Schema: nil, // This should be skipped
						},
					},
				},
			},
		}
		err := validateManifestSchemas(ctx, provider)
		require.NoError(t, err) // nil schema should be skipped
	})

	t.Run("provider with multiple resource types and versions", func(t *testing.T) {
		provider := &ResourceProvider{
			Name: "Test.Provider",
			Types: map[string]*ResourceType{
				"widgets": {
					APIVersions: map[string]*ResourceTypeAPIVersion{
						"2023-10-01": {
							Schema: map[string]any{
								"type": "object",
								"properties": map[string]any{
									"name":        map[string]any{"type": "string"},
									"environment": map[string]any{"type": "string"},
								},
							},
						},
						"2023-11-01": {
							Schema: map[string]any{
								"type": "object",
								"properties": map[string]any{
									"name":        map[string]any{"type": "string"},
									"description": map[string]any{"type": "string"},
									"environment": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
				"gadgets": {
					APIVersions: map[string]*ResourceTypeAPIVersion{
						"2023-10-01": {
							Schema: map[string]any{
								"type": "object",
								"properties": map[string]any{
									"id":          map[string]any{"type": "string"},
									"active":      map[string]any{"type": "boolean"},
									"environment": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			},
		}
		err := validateManifestSchemas(ctx, provider)
		require.NoError(t, err)
	})

	t.Run("provider with multiple errors", func(t *testing.T) {
		provider := &ResourceProvider{
			Name: "Test.Provider",
			Types: map[string]*ResourceType{
				"widgets": {
					APIVersions: map[string]*ResourceTypeAPIVersion{
						"2023-10-01": {
							Schema: map[string]any{
								"type": "invalidtype", // Error 1: invalid type
							},
						},
					},
				},
				"gadgets": {
					APIVersions: map[string]*ResourceTypeAPIVersion{
						"2023-10-01": {
							Schema: map[string]any{
								"allOf": []map[string]any{ // Error 2: prohibited feature
									{"type": "string"},
								},
							},
						},
					},
				},
			},
		}
		err := validateManifestSchemas(ctx, provider)
		require.Error(t, err)

		// Should be a ValidationErrors with multiple errors
		var validationErrors *schema.ValidationErrors
		require.ErrorAs(t, err, &validationErrors)
		require.True(t, validationErrors.HasErrors())
		require.Len(t, validationErrors.Errors, 2)
	})
}
