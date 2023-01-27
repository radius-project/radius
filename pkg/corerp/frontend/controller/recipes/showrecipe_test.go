// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package recipes

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestShowRecipe(t *testing.T) {
	ctx := context.Background()

	t.Run("show recipe", func(t *testing.T) {
		recipeDetails := datamodel.EnvironmentRecipeProperties{
			TemplatePath: "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0",
		}
		err := ShowRecipe(ctx, &recipeDetails, "mongodb")
		require.NoError(t, err)
		expectedOutput := map[string]any{
			"mongodbName":    "type : string\t",
			"documentdbName": "type : string\t",
			"location":       "type : string\tdefaultValue : [resourceGroup().location]\t",
		}
		require.Equal(t, expectedOutput, recipeDetails.Parameters)
	})

	t.Run("show recipe with context parameter", func(t *testing.T) {
		recipeDetails := datamodel.EnvironmentRecipeProperties{
			TemplatePath: "radiusdev.azurecr.io/recipes/functionaltest/mongodatabases/azure:1.0",
		}
		err := ShowRecipe(ctx, &recipeDetails, "mongodb")
		require.NoError(t, err)
		expectedOutput := map[string]any{
			"location": "type : string\tdefaultValue : [resourceGroup().location]\t",
		}
		require.Equal(t, expectedOutput, recipeDetails.Parameters)
	})

	t.Run("show recipe with invalid path", func(t *testing.T) {
		recipeDetails := datamodel.EnvironmentRecipeProperties{
			TemplatePath: "radiusdev.azurecr.io/recipes/functionaltest/test/mongodatabases/azure:1.0",
		}
		err := ShowRecipe(ctx, &recipeDetails, "mongodb")
		require.Error(t, err, "failed to fetch template from the path \"radiusdev.azurecr.io/recipes/functionaltest/test/mongodatabases/azure:1.0\" for recipe \"mongodb\": radiusdev.azurecr.io/recipes/functionaltest/test/mongodatabases/azure:1.0: not found")
	})
}
