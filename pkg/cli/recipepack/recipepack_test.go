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

package recipepack

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/stretchr/testify/require"
)

func Test_DefaultRecipePackName(t *testing.T) {
	require.Equal(t, "local-dev", DefaultRecipePackName)
}

func Test_GetSingletonRecipePackDefinitions(t *testing.T) {
	definitions := GetSingletonRecipePackDefinitions()

	// Verify we have the expected number of definitions
	require.Len(t, definitions, 4)

	// Verify expected resource types and names
	expectedDefinitions := map[string]string{
		"containers":        "Radius.Compute/containers",
		"persistentvolumes": "Radius.Compute/persistentVolumes",
		"routes":            "Radius.Compute/routes",
		"secrets":           "Radius.Security/secrets",
	}

	for _, def := range definitions {
		expectedResourceType, exists := expectedDefinitions[def.Name]
		require.True(t, exists, "Unexpected definition name: %s", def.Name)
		require.Equal(t, expectedResourceType, def.ResourceType, "Resource type mismatch for %s", def.Name)
		require.NotEmpty(t, def.RecipeLocation, "RecipeLocation should not be empty for %s", def.Name)
	}
}

func Test_NewSingletonRecipePackResource(t *testing.T) {
	resourceType := "Radius.Compute/containers"
	recipeLocation := "ghcr.io/radius-project/kube-recipes/containers@latest"

	resource := NewSingletonRecipePackResource(resourceType, recipeLocation)

	// Verify location
	require.NotNil(t, resource.Location)
	require.Equal(t, "global", *resource.Location)

	// Verify properties exist
	require.NotNil(t, resource.Properties)
	require.NotNil(t, resource.Properties.Recipes)

	// Verify the resource contains exactly one recipe
	require.Len(t, resource.Properties.Recipes, 1)

	// Verify the recipe
	recipe, exists := resource.Properties.Recipes[resourceType]
	require.True(t, exists, "Expected recipe for resource type %s to exist", resourceType)
	require.NotNil(t, recipe.RecipeKind)
	require.Equal(t, corerpv20250801.RecipeKindBicep, *recipe.RecipeKind)
	require.NotNil(t, recipe.RecipeLocation)
	require.Equal(t, recipeLocation, *recipe.RecipeLocation)
}

func Test_CreateSingletonRecipePacksWithClient(t *testing.T) {
	t.Run("Success: creates all singleton recipe packs", func(t *testing.T) {
		rootScope := "/planes/radius/local/resourceGroups/test-rg"
		resourceGroupName := "test-rg"

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(rootScope, nil, nil)
		require.NoError(t, err)

		recipePackClient := factory.NewRecipePacksClient()

		recipePackIDs, err := CreateSingletonRecipePacksWithClient(context.Background(), recipePackClient, resourceGroupName)
		require.NoError(t, err)

		// Verify the correct number of recipe packs were created
		definitions := GetSingletonRecipePackDefinitions()
		require.Len(t, recipePackIDs, len(definitions))

		// Verify the IDs are in the expected format
		for i, def := range definitions {
			expectedID := "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/recipePacks/" + def.Name
			require.Equal(t, expectedID, recipePackIDs[i])
		}
	})
}
