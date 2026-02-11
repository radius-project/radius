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
	"github.com/radius-project/radius/pkg/to"
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
		// The client must be scoped to the default resource group scope.
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(DefaultResourceGroupScope, nil, nil)
		require.NoError(t, err)

		recipePackClient := factory.NewRecipePacksClient()

		recipePackIDs, err := CreateSingletonRecipePacks(context.Background(), recipePackClient)
		require.NoError(t, err)

		// Verify the correct number of recipe packs were created
		definitions := GetSingletonRecipePackDefinitions()
		require.Len(t, recipePackIDs, len(definitions))

		// Verify the IDs are in the default scope
		for i, def := range definitions {
			expectedID := DefaultResourceGroupScope + "/providers/Radius.Core/recipePacks/" + def.Name
			require.Equal(t, expectedID, recipePackIDs[i])
		}
	})
}

func Test_InspectRecipePacks(t *testing.T) {
	scope := "/planes/radius/local/resourceGroups/test-rg"

	t.Run("collects resource types from packs", func(t *testing.T) {
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, nil, test_client_factory.WithRecipePackServerUniqueTypes)
		require.NoError(t, err)

		clientsByScope := map[string]*corerpv20250801.RecipePacksClient{
			scope: factory.NewRecipePacksClient(),
		}
		packIDs := []string{
			scope + "/providers/Radius.Core/recipePacks/pack-a",
			scope + "/providers/Radius.Core/recipePacks/pack-b",
		}

		coveredTypes, conflicts, err := InspectRecipePacks(context.Background(), clientsByScope, packIDs)
		require.NoError(t, err)
		require.Empty(t, conflicts)
		// Each pack has a unique type based on its name
		require.Len(t, coveredTypes, 2)
		require.Equal(t, "pack-a", coveredTypes["Test.Resource/pack-a"])
		require.Equal(t, "pack-b", coveredTypes["Test.Resource/pack-b"])
	})

	t.Run("detects conflicts", func(t *testing.T) {
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, nil, test_client_factory.WithRecipePackServerConflictingTypes)
		require.NoError(t, err)

		clientsByScope := map[string]*corerpv20250801.RecipePacksClient{
			scope: factory.NewRecipePacksClient(),
		}
		packIDs := []string{
			scope + "/providers/Radius.Core/recipePacks/pack1",
			scope + "/providers/Radius.Core/recipePacks/pack2",
		}

		_, conflicts, err := InspectRecipePacks(context.Background(), clientsByScope, packIDs)
		require.NoError(t, err)
		require.Len(t, conflicts, 1)
		require.Contains(t, conflicts, "Radius.Compute/containers")
		require.ElementsMatch(t, []string{"pack1", "pack2"}, conflicts["Radius.Compute/containers"])
	})

	t.Run("skips unparseable IDs", func(t *testing.T) {
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, nil, test_client_factory.WithRecipePackServerUniqueTypes)
		require.NoError(t, err)

		clientsByScope := map[string]*corerpv20250801.RecipePacksClient{
			scope: factory.NewRecipePacksClient(),
		}
		packIDs := []string{
			"not-a-valid-id",
			scope + "/providers/Radius.Core/recipePacks/valid-pack",
		}

		coveredTypes, conflicts, err := InspectRecipePacks(context.Background(), clientsByScope, packIDs)
		require.NoError(t, err)
		require.Empty(t, conflicts)
		require.Len(t, coveredTypes, 1)
	})

	t.Run("skips packs with unknown scope", func(t *testing.T) {
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, nil, test_client_factory.WithRecipePackServerUniqueTypes)
		require.NoError(t, err)

		clientsByScope := map[string]*corerpv20250801.RecipePacksClient{
			scope: factory.NewRecipePacksClient(),
		}
		// This pack is in a different scope not in the map
		otherScope := "/planes/radius/local/resourceGroups/other-rg"
		packIDs := []string{
			otherScope + "/providers/Radius.Core/recipePacks/remote-pack",
		}

		coveredTypes, conflicts, err := InspectRecipePacks(context.Background(), clientsByScope, packIDs)
		require.NoError(t, err)
		require.Empty(t, conflicts)
		require.Empty(t, coveredTypes)
	})

	t.Run("empty pack list", func(t *testing.T) {
		clientsByScope := map[string]*corerpv20250801.RecipePacksClient{}

		coveredTypes, conflicts, err := InspectRecipePacks(context.Background(), clientsByScope, nil)
		require.NoError(t, err)
		require.Empty(t, conflicts)
		require.Empty(t, coveredTypes)
	})
}

func Test_EnsureMissingSingletons(t *testing.T) {
	t.Run("creates all singletons when none covered", func(t *testing.T) {
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(DefaultResourceGroupScope, nil, nil)
		require.NoError(t, err)

		client := factory.NewRecipePacksClient()
		coveredTypes := map[string]string{} // nothing covered

		ids, err := EnsureMissingSingletons(context.Background(), client, coveredTypes)
		require.NoError(t, err)
		require.Len(t, ids, 4)

		for _, def := range GetSingletonRecipePackDefinitions() {
			expected := DefaultResourceGroupScope + "/providers/Radius.Core/recipePacks/" + def.Name
			require.Contains(t, ids, expected)
		}
	})

	t.Run("skips already covered types", func(t *testing.T) {
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(DefaultResourceGroupScope, nil, nil)
		require.NoError(t, err)

		client := factory.NewRecipePacksClient()
		// Cover 2 of 4 types
		coveredTypes := map[string]string{
			"Radius.Compute/containers": "my-containers-pack",
			"Radius.Security/secrets":   "my-secrets-pack",
		}

		ids, err := EnsureMissingSingletons(context.Background(), client, coveredTypes)
		require.NoError(t, err)
		require.Len(t, ids, 2)

		require.Contains(t, ids, DefaultResourceGroupScope+"/providers/Radius.Core/recipePacks/persistentvolumes")
		require.Contains(t, ids, DefaultResourceGroupScope+"/providers/Radius.Core/recipePacks/routes")
	})

	t.Run("returns nil when all types covered", func(t *testing.T) {
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(DefaultResourceGroupScope, nil, nil)
		require.NoError(t, err)

		client := factory.NewRecipePacksClient()
		coveredTypes := map[string]string{
			"Radius.Compute/containers":        "a",
			"Radius.Compute/persistentVolumes": "b",
			"Radius.Compute/routes":            "c",
			"Radius.Security/secrets":          "d",
		}

		ids, err := EnsureMissingSingletons(context.Background(), client, coveredTypes)
		require.NoError(t, err)
		require.Nil(t, ids)
	})
}

func Test_FormatConflictError(t *testing.T) {
	conflicts := map[string][]string{
		"Radius.Compute/containers": {"pack1", "pack2"},
	}

	err := FormatConflictError(conflicts)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Recipe pack conflict detected")
	require.Contains(t, err.Error(), "Radius.Compute/containers")
	require.Contains(t, err.Error(), "pack1")
	require.Contains(t, err.Error(), "pack2")
}

func Test_RecipePackIDExists(t *testing.T) {
	packs := []*string{
		to.Ptr("/planes/radius/local/resourceGroups/rg/providers/Radius.Core/recipePacks/containers"),
		to.Ptr("/planes/radius/local/resourceGroups/rg/providers/Radius.Core/recipePacks/routes"),
	}

	require.True(t, RecipePackIDExists(packs, "/planes/radius/local/resourceGroups/rg/providers/Radius.Core/recipePacks/containers"))
	require.False(t, RecipePackIDExists(packs, "/planes/radius/local/resourceGroups/rg/providers/Radius.Core/recipePacks/secrets"))
	require.False(t, RecipePackIDExists(nil, "anything"))
}

func Test_ExtractRecipePackIDs(t *testing.T) {
	t.Run("nil properties returns nil", func(t *testing.T) {
		ids := ExtractRecipePackIDs(nil)
		require.Nil(t, ids)
	})

	t.Run("missing recipePacks key returns nil", func(t *testing.T) {
		ids := ExtractRecipePackIDs(map[string]any{"other": "value"})
		require.Nil(t, ids)
	})

	t.Run("recipePacks is not an array returns nil", func(t *testing.T) {
		ids := ExtractRecipePackIDs(map[string]any{"recipePacks": "not-an-array"})
		require.Nil(t, ids)
	})

	t.Run("empty array returns nil", func(t *testing.T) {
		ids := ExtractRecipePackIDs(map[string]any{"recipePacks": []any{}})
		require.Nil(t, ids)
	})

	t.Run("skips non-string elements", func(t *testing.T) {
		ids := ExtractRecipePackIDs(map[string]any{
			"recipePacks": []any{42, true, nil},
		})
		require.Nil(t, ids)
	})

	t.Run("extracts string elements", func(t *testing.T) {
		ids := ExtractRecipePackIDs(map[string]any{
			"recipePacks": []any{
				"/planes/radius/local/resourceGroups/rg/providers/Radius.Core/recipePacks/pack1",
				"/planes/radius/local/resourceGroups/rg/providers/Radius.Core/recipePacks/pack2",
			},
		})
		require.Len(t, ids, 2)
		require.Equal(t, "/planes/radius/local/resourceGroups/rg/providers/Radius.Core/recipePacks/pack1", ids[0])
		require.Equal(t, "/planes/radius/local/resourceGroups/rg/providers/Radius.Core/recipePacks/pack2", ids[1])
	})

	t.Run("mixed types extracts only strings", func(t *testing.T) {
		ids := ExtractRecipePackIDs(map[string]any{
			"recipePacks": []any{
				"/planes/radius/local/resourceGroups/rg/providers/Radius.Core/recipePacks/pack1",
				42,
				"/planes/radius/local/resourceGroups/rg/providers/Radius.Core/recipePacks/pack2",
			},
		})
		require.Len(t, ids, 2)
	})
}
