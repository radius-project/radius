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

func Test_NewDefaultRecipePackResource(t *testing.T) {
	resource := NewDefaultRecipePackResource()

	// Verify location
	require.NotNil(t, resource.Location)
	require.Equal(t, "global", *resource.Location)

	// Verify properties exist
	require.NotNil(t, resource.Properties)
	require.NotNil(t, resource.Properties.Recipes)

	// Verify expected recipes exist
	expectedRecipeTypes := []string{
		"Radius.Compute/containers",
		"Radius.Compute/persistentVolumes",
		"Radius.Data/mySqlDatabases",
		"Radius.Data/postgreSqlDatabases",
		"Radius.Security/secrets",
	}

	for _, recipeType := range expectedRecipeTypes {
		recipe, exists := resource.Properties.Recipes[recipeType]
		require.True(t, exists, "Expected recipe type %s to exist", recipeType)
		require.NotNil(t, recipe, "Recipe for %s should not be nil", recipeType)
		require.NotNil(t, recipe.RecipeKind, "RecipeKind for %s should not be nil", recipeType)
		require.Equal(t, corerpv20250801.RecipeKindBicep, *recipe.RecipeKind, "RecipeKind for %s should be Bicep", recipeType)
		require.NotNil(t, recipe.RecipeLocation, "RecipeLocation for %s should not be nil", recipeType)
		require.NotNil(t, recipe.PlainHTTP, "PlainHTTP for %s should not be nil", recipeType)
		require.True(t, *recipe.PlainHTTP, "PlainHTTP for %s should be true", recipeType)
	}

	// Verify the correct number of recipes
	require.Len(t, resource.Properties.Recipes, len(expectedRecipeTypes))
}

func Test_CreateDefaultRecipePackWithClient(t *testing.T) {
	t.Run("Success: creates recipe pack", func(t *testing.T) {
		rootScope := "/planes/radius/local/resourceGroups/test-rg"
		resourceGroupName := "test-rg"

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(rootScope, nil, nil)
		require.NoError(t, err)

		recipePackClient := factory.NewRecipePacksClient()

		recipePackID, err := CreateDefaultRecipePackWithClient(context.Background(), recipePackClient, resourceGroupName)
		require.NoError(t, err)

		expectedID := "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/recipePacks/local-dev"
		require.Equal(t, expectedID, recipePackID)
	})
}

func Test_DefaultRecipePackName(t *testing.T) {
	require.Equal(t, "local-dev", DefaultRecipePackName)
}

func Test_GetSingletonRecipePackDefinitions(t *testing.T) {
	definitions := GetSingletonRecipePackDefinitions()

	// Verify we have the expected number of definitions
	require.Len(t, definitions, 5)

	// Verify expected resource types and names
	expectedDefinitions := map[string]string{
		"containers":         "Radius.Compute/containers",
		"persistentvolumes":  "Radius.Compute/persistentVolumes",
		"mysqldatabases":     "Radius.Data/mySqlDatabases",
		"postgresqldatabases": "Radius.Data/postgreSqlDatabases",
		"secrets":            "Radius.Security/secrets",
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
	recipeLocation := "localhost:5000/test-recipe:latest"

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
	require.NotNil(t, recipe.PlainHTTP)
	require.True(t, *recipe.PlainHTTP)
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

func Test_GetRecipePackNameForResourceType(t *testing.T) {
	t.Run("finds existing resource type", func(t *testing.T) {
		name := GetRecipePackNameForResourceType("Radius.Compute/containers")
		require.Equal(t, "containers", name)
	})

	t.Run("finds resource type case-insensitive", func(t *testing.T) {
		name := GetRecipePackNameForResourceType("radius.compute/CONTAINERS")
		require.Equal(t, "containers", name)
	})

	t.Run("returns empty for unknown resource type", func(t *testing.T) {
		name := GetRecipePackNameForResourceType("Unknown/resourceType")
		require.Empty(t, name)
	})
}
