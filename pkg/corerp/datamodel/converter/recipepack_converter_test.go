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

package converter

import (
	"encoding/json"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func TestRecipePackDataModelToVersioned(t *testing.T) {
	testCases := []struct {
		name         string
		dataModel    *datamodel.RecipePack
		version      string
		expected     v1.VersionedModelInterface
		expectError  bool
		expectedType any
	}{
		{
			name: "valid conversion to 2025-08-01-preview",
			dataModel: &datamodel.RecipePack{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/recipePacks/test-pack",
						Name:     "test-pack",
						Type:     "Radius.Core/recipePacks",
						Location: "global",
						Tags: map[string]string{
							"env": "test",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "2025-08-01-preview",
						UpdatedAPIVersion:      "2025-08-01-preview",
						AsyncProvisioningState: v1.ProvisioningStateSucceeded,
					},
				},
				Properties: datamodel.RecipePackProperties{
					Recipes: map[string]*datamodel.RecipeDefinition{
						"Applications.Core/containers": {
							RecipeKind:     "bicep",
							RecipeLocation: "br:myregistry.azurecr.io/recipes/container:1.0",
							Parameters: map[string]any{
								"param1": "value1",
							},
							PlainHTTP: false,
						},
						"Applications.Datastores/sqlDatabases": {
							RecipeKind:     "terraform",
							RecipeLocation: "https://github.com/radius-project/recipes.git//terraform/modules/sql",
							PlainHTTP:      false,
						},
					},
					ReferencedBy: []string{
						"/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/environments/env1",
						"/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/environments/env2",
					},
				},
			},
			version:      v20250801preview.Version,
			expectError:  false,
			expectedType: &v20250801preview.RecipePackResource{},
		},
		{
			name: "minimal recipe pack",
			dataModel: &datamodel.RecipePack{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/recipePacks/minimal-pack",
						Name:     "minimal-pack",
						Type:     "Radius.Core/recipePacks",
						Location: "global",
					},
				},
				Properties: datamodel.RecipePackProperties{},
			},
			version:      v20250801preview.Version,
			expectError:  false,
			expectedType: &v20250801preview.RecipePackResource{},
		},
		{
			name:        "unsupported version",
			dataModel:   &datamodel.RecipePack{},
			version:     "unsupported-version",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := RecipePackDataModelToVersioned(tc.dataModel, tc.version)

			if tc.expectError {
				require.Error(t, err)
				require.Equal(t, v1.ErrUnsupportedAPIVersion, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.IsType(t, tc.expectedType, result)

				// Validate the conversion worked correctly
				if tc.version == v20250801preview.Version {
					versionedResource := result.(*v20250801preview.RecipePackResource)
					require.Equal(t, tc.dataModel.ID, to.String(versionedResource.ID))
					require.Equal(t, tc.dataModel.Name, to.String(versionedResource.Name))
					require.Equal(t, tc.dataModel.Type, to.String(versionedResource.Type))
					require.Equal(t, tc.dataModel.Location, to.String(versionedResource.Location))

					if len(tc.dataModel.Properties.ReferencedBy) > 0 {
						require.Equal(t, len(tc.dataModel.Properties.ReferencedBy), len(versionedResource.Properties.ReferencedBy))
					}

					if tc.dataModel.Properties.Recipes != nil {
						require.NotNil(t, versionedResource.Properties.Recipes)
						require.Equal(t, len(tc.dataModel.Properties.Recipes), len(versionedResource.Properties.Recipes))
					}
				}
			}
		})
	}
}

func TestRecipePackDataModelFromVersioned(t *testing.T) {
	testCases := []struct {
		name        string
		content     []byte
		version     string
		expectError bool
		expected    *datamodel.RecipePack
	}{
		{
			name: "valid conversion from 2025-08-01-preview",
			content: []byte(`{
					"id": "/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/recipePacks/test-pack",
					"name": "test-pack",
					"type": "Radius.Core/recipePacks",
					"location": "global",
					"tags": {
						"env": "test"
					},
					"properties": {
						"recipes": {
							"Applications.Core/containers": {
								"recipeKind": "bicep",
								"recipeLocation": "br:myregistry.azurecr.io/recipes/container:1.0",
								"parameters": {
									"param1": "value1"
								},
								"plainHTTP": false
							},
							"Applications.Datastores/sqlDatabases": {
								"recipeKind": "terraform",
								"recipeLocation": "https://github.com/radius-project/recipes.git//terraform/modules/sql"
							}
						},
						"referencedBy": [
							"/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/environments/env1",
							"/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/environments/env2"
						],
						"provisioningState": "Succeeded"
					}
				}`),
			version:     v20250801preview.Version,
			expectError: false,
			expected: &datamodel.RecipePack{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/recipePacks/test-pack",
						Name:     "test-pack",
						Type:     "Radius.Core/recipePacks",
						Location: "global",
						Tags: map[string]string{
							"env": "test",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "2025-08-01-preview",
						UpdatedAPIVersion:      "2025-08-01-preview",
						AsyncProvisioningState: v1.ProvisioningStateSucceeded,
					},
				},
				Properties: datamodel.RecipePackProperties{
					Recipes: map[string]*datamodel.RecipeDefinition{
						"Applications.Core/containers": {
							RecipeKind:     "bicep",
							RecipeLocation: "br:myregistry.azurecr.io/recipes/container:1.0",
							Parameters: map[string]any{
								"param1": "value1",
							},
							PlainHTTP: false,
						},
						"Applications.Datastores/sqlDatabases": {
							RecipeKind:     "terraform",
							RecipeLocation: "https://github.com/radius-project/recipes.git//terraform/modules/sql",
							PlainHTTP:      false,
						},
					},
					ReferencedBy: []string{
						"/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/environments/env1",
						"/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/environments/env2",
					},
				},
			},
		},
		{
			name: "minimal recipe pack",
			content: []byte(`{
					"id": "/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/recipePacks/minimal-pack",
					"name": "minimal-pack",
					"type": "Radius.Core/recipePacks",
					"location": "global",
					"properties": {}
				}`),
			version:     v20250801preview.Version,
			expectError: false,
			expected: &datamodel.RecipePack{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/recipePacks/minimal-pack",
						Name:     "minimal-pack",
						Type:     "Radius.Core/recipePacks",
						Location: "global",
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion: "2025-08-01-preview",
						UpdatedAPIVersion: "2025-08-01-preview",
					},
				},
				Properties: datamodel.RecipePackProperties{},
			},
		},
		{
			name: "plainHTTP defaults to false when not specified",
			content: []byte(`{
					"id": "/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/recipePacks/default-plainhttp",
					"name": "default-plainhttp",
					"type": "Radius.Core/recipePacks",
					"location": "global",
					"properties": {
						"recipes": {
							"Applications.Core/containers": {
								"recipeKind": "bicep",
								"recipeLocation": "br:myregistry.azurecr.io/recipes/container:1.0"
							}
						}
					}
				}`),
			version:     v20250801preview.Version,
			expectError: false,
			expected: &datamodel.RecipePack{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/recipePacks/default-plainhttp",
						Name:     "default-plainhttp",
						Type:     "Radius.Core/recipePacks",
						Location: "global",
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion: "2025-08-01-preview",
						UpdatedAPIVersion: "2025-08-01-preview",
					},
				},
				Properties: datamodel.RecipePackProperties{
					Recipes: map[string]*datamodel.RecipeDefinition{
						"Applications.Core/containers": {
							RecipeKind:     "bicep",
							RecipeLocation: "br:myregistry.azurecr.io/recipes/container:1.0",
							PlainHTTP:      false, // Should default to false when not specified
						},
					},
				},
			},
		},
		{
			name: "plainHTTP explicitly set to true",
			content: []byte(`{
				"id": "/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/recipePacks/explicit-plainhttp",
				"name": "explicit-plainhttp",
				"type": "Radius.Core/recipePacks",
				"location": "global",
				"properties": {
					"recipes": {
						"Applications.Datastores/sqlDatabases": {
							"recipeKind": "terraform",
							"recipeLocation": "http://insecure-registry.example.com/recipes/sql",
							"plainHttp": true
						}
					}
				}
			}`),
			version:     v20250801preview.Version,
			expectError: false,
			expected: &datamodel.RecipePack{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/recipePacks/explicit-plainhttp",
						Name:     "explicit-plainhttp",
						Type:     "Radius.Core/recipePacks",
						Location: "global",
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion: "2025-08-01-preview",
						UpdatedAPIVersion: "2025-08-01-preview",
					},
				},
				Properties: datamodel.RecipePackProperties{
					Recipes: map[string]*datamodel.RecipeDefinition{
						"Applications.Datastores/sqlDatabases": {
							RecipeKind:     "terraform",
							RecipeLocation: "http://insecure-registry.example.com/recipes/sql",
							PlainHTTP:      true, // Explicitly set to true
						},
					},
				},
			},
		},
		{
			name:        "invalid JSON",
			content:     []byte(`{invalid json}`),
			version:     v20250801preview.Version,
			expectError: true,
		},
		{
			name:        "unsupported version",
			content:     []byte(`{}`),
			version:     "unsupported-version",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Debug: print the JSON being processed
			if tc.name == "plainHTTP explicitly set to true" {
				t.Logf("JSON content: %s", string(tc.content))
			}

			result, err := RecipePackDataModelFromVersioned(tc.content, tc.version)

			if tc.expectError {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)

				// Validate the conversion worked correctly
				require.Equal(t, tc.expected.ID, result.ID)
				require.Equal(t, tc.expected.Name, result.Name)
				require.Equal(t, tc.expected.Type, result.Type)
				require.Equal(t, tc.expected.Location, result.Location)
				// Note: StringMap converts nil maps to empty maps, so we need to handle this
				if tc.expected.Tags == nil {
					require.Empty(t, result.Tags)
				} else {
					require.Equal(t, tc.expected.Tags, result.Tags)
				}
				require.Equal(t, tc.expected.Properties.ReferencedBy, result.Properties.ReferencedBy)

				// Validate recipes
				if tc.expected.Properties.Recipes != nil {
					require.NotNil(t, result.Properties.Recipes)
					require.Equal(t, len(tc.expected.Properties.Recipes), len(result.Properties.Recipes))

					for key, expectedRecipe := range tc.expected.Properties.Recipes {
						actualRecipe, exists := result.Properties.Recipes[key]
						require.True(t, exists, "Recipe %s should exist", key)
						require.Equal(t, expectedRecipe.RecipeKind, actualRecipe.RecipeKind)
						require.Equal(t, expectedRecipe.RecipeLocation, actualRecipe.RecipeLocation)

						// Debug output for plainHTTP
						t.Logf("Recipe %s - Expected PlainHTTP: %v, Actual PlainHTTP: %v", key, expectedRecipe.PlainHTTP, actualRecipe.PlainHTTP)
						require.Equal(t, expectedRecipe.PlainHTTP, actualRecipe.PlainHTTP, "PlainHTTP for recipe %s should match. Expected: %v, Actual: %v", key, expectedRecipe.PlainHTTP, actualRecipe.PlainHTTP)

						// Note: JSON unmarshaling can change the type of parameters, especially for numbers
						if expectedRecipe.Parameters != nil {
							require.Equal(t, expectedRecipe.Parameters, actualRecipe.Parameters)
						}
					}
				}
			}
		})
	}
}

func TestRecipePackRoundTripConversion(t *testing.T) {
	// Test round-trip conversion: datamodel -> versioned -> JSON -> versioned -> datamodel
	originalDataModel := &datamodel.RecipePack{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       "/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/recipePacks/round-trip-pack",
				Name:     "round-trip-pack",
				Type:     "Radius.Core/recipePacks",
				Location: "global",
				Tags: map[string]string{
					"purpose": "testing",
				},
			},
			InternalMetadata: v1.InternalMetadata{
				CreatedAPIVersion:      "2025-08-01-preview",
				UpdatedAPIVersion:      "2025-08-01-preview",
				AsyncProvisioningState: v1.ProvisioningStateSucceeded,
			},
		},
		Properties: datamodel.RecipePackProperties{
			Recipes: map[string]*datamodel.RecipeDefinition{
				"Applications.Core/containers": {
					RecipeKind:     "bicep",
					RecipeLocation: "br:test.azurecr.io/recipes/container:latest",
					Parameters: map[string]interface{}{
						"cpu":    "0.5",
						"memory": "1Gi",
					},
					PlainHTTP: false,
				},
			},
			ReferencedBy: []string{
				"/subscriptions/test-sub/resourceGroups/test-rg/providers/Radius.Core/environments/test-env",
			},
		},
	}

	// Convert to versioned model
	versionedModel, err := RecipePackDataModelToVersioned(originalDataModel, v20250801preview.Version)
	require.NoError(t, err)
	require.NotNil(t, versionedModel)

	// Serialize to JSON
	jsonBytes, err := json.Marshal(versionedModel)
	require.NoError(t, err)

	// Convert back to datamodel
	resultDataModel, err := RecipePackDataModelFromVersioned(jsonBytes, v20250801preview.Version)
	require.NoError(t, err)
	require.NotNil(t, resultDataModel)

	// Validate that the round-trip preserved all data
	require.Equal(t, originalDataModel.ID, resultDataModel.ID)
	require.Equal(t, originalDataModel.Name, resultDataModel.Name)
	require.Equal(t, originalDataModel.Type, resultDataModel.Type)
	require.Equal(t, originalDataModel.Location, resultDataModel.Location)
	require.Equal(t, originalDataModel.Tags, resultDataModel.Tags)
	require.Equal(t, originalDataModel.Properties.ReferencedBy, resultDataModel.Properties.ReferencedBy)

	// Validate recipes
	require.Equal(t, len(originalDataModel.Properties.Recipes), len(resultDataModel.Properties.Recipes))
	for key, originalRecipe := range originalDataModel.Properties.Recipes {
		resultRecipe, exists := resultDataModel.Properties.Recipes[key]
		require.True(t, exists, "Recipe %s should exist after round-trip", key)
		require.Equal(t, originalRecipe.RecipeKind, resultRecipe.RecipeKind)
		require.Equal(t, originalRecipe.RecipeLocation, resultRecipe.RecipeLocation)
		require.Equal(t, originalRecipe.PlainHTTP, resultRecipe.PlainHTTP)
		require.Equal(t, originalRecipe.Parameters, resultRecipe.Parameters)
	}
}

func TestRecipePackEdgeCases(t *testing.T) {
	testCases := []struct {
		name      string
		dataModel *datamodel.RecipePack
		version   string
	}{
		{
			name: "empty recipes map",
			dataModel: &datamodel.RecipePack{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/test/empty-recipes",
						Name:     "empty-recipes",
						Type:     "Radius.Core/recipePacks",
						Location: "global",
					},
				},
				Properties: datamodel.RecipePackProperties{
					Recipes: map[string]*datamodel.RecipeDefinition{},
				},
			},
			version: v20250801preview.Version,
		},
		{
			name: "nil recipes map",
			dataModel: &datamodel.RecipePack{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/test/nil-recipes",
						Name:     "nil-recipes",
						Type:     "Radius.Core/recipePacks",
						Location: "global",
					},
				},
				Properties: datamodel.RecipePackProperties{
					Recipes: nil,
				},
			},
			version: v20250801preview.Version,
		},
		{
			name: "empty referenced by list",
			dataModel: &datamodel.RecipePack{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/test/empty-refs",
						Name:     "empty-refs",
						Type:     "Radius.Core/recipePacks",
						Location: "global",
					},
				},
				Properties: datamodel.RecipePackProperties{
					ReferencedBy: []string{},
				},
			},
			version: v20250801preview.Version,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test conversion to versioned
			versionedModel, err := RecipePackDataModelToVersioned(tc.dataModel, tc.version)
			require.NoError(t, err)
			require.NotNil(t, versionedModel)

			// Serialize and deserialize
			jsonBytes, err := json.Marshal(versionedModel)
			require.NoError(t, err)

			resultDataModel, err := RecipePackDataModelFromVersioned(jsonBytes, tc.version)
			require.NoError(t, err)
			require.NotNil(t, resultDataModel)

			// Basic validation
			require.Equal(t, tc.dataModel.ID, resultDataModel.ID)
			require.Equal(t, tc.dataModel.Name, resultDataModel.Name)
		})
	}
}
