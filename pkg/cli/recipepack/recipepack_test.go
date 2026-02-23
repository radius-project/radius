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
	"testing"

	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/stretchr/testify/require"
)

func Test_GetDefaultRecipePackDefinition(t *testing.T) {
	definitions := GetDefaultRecipePackDefinition()

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

func Test_NewDefaultRecipePackResource(t *testing.T) {
	resource := NewDefaultRecipePackResource()

	// Verify location
	require.NotNil(t, resource.Location)
	require.Equal(t, "global", *resource.Location)

	// Verify properties exist
	require.NotNil(t, resource.Properties)
	require.NotNil(t, resource.Properties.Recipes)

	// Verify the resource contains recipes for all core types.
	definitions := GetDefaultRecipePackDefinition()
	require.Len(t, resource.Properties.Recipes, len(definitions))

	for _, def := range definitions {
		recipe, exists := resource.Properties.Recipes[def.ResourceType]
		require.True(t, exists, "Expected recipe for resource type %s to exist", def.ResourceType)
		require.NotNil(t, recipe.RecipeKind)
		require.Equal(t, corerpv20250801.RecipeKindBicep, *recipe.RecipeKind)
		require.NotNil(t, recipe.RecipeLocation)
		require.Equal(t, def.RecipeLocation, *recipe.RecipeLocation)
	}
}
