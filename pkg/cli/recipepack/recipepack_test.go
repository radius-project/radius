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
	"strings"
	"testing"

	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/version"
	"github.com/stretchr/testify/require"
)

func Test_GetDefaultRecipePackDefinition(t *testing.T) {
	definitions := GetCoreTypesRecipeInfo()

	// Verify we have the expected number of definitions
	require.Len(t, definitions, 4)

	// Verify expected resource types
	expectedResourceTypes := []string{
		"Radius.Compute/containers",
		"Radius.Compute/persistentVolumes",
		"Radius.Compute/routes",
		"Radius.Security/secrets",
	}
	actualResourceTypes := make([]string, len(definitions))
	for i, def := range definitions {
		actualResourceTypes[i] = def.ResourceType
		require.NotEmpty(t, def.RecipeLocation, "RecipeLocation should not be empty for %s", def.ResourceType)
	}
	require.ElementsMatch(t, expectedResourceTypes, actualResourceTypes)
}

func Test_GetDefaultRecipePackDefinition_UsesLatestTagForEdgeChannel(t *testing.T) {
	// The test binary is built without ldflags, so channel defaults to "edge".
	require.True(t, version.IsEdgeChannel(), "default should be on edge channel")

	definitions := GetCoreTypesRecipeInfo()
	for _, def := range definitions {
		require.True(t, strings.HasSuffix(def.RecipeLocation, ":latest"),
			"Expected :latest tag for edge channel, got %s", def.RecipeLocation)
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
	definitions := GetCoreTypesRecipeInfo()
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
