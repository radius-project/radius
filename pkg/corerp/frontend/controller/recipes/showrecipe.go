// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package recipes

import (
	"context"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/rp/util"
)

func ShowRecipe(ctx context.Context, recipeDetails *datamodel.EnvironmentRecipeProperties, recipeName string) error {
	recipeData := make(map[string]any)
	err := util.ReadFromRegistry(ctx, recipeDetails.TemplatePath, &recipeData)
	if err != nil {
		return v1.NewClientErrInvalidRequest(fmt.Sprintf("failed to fetch template from the path %q for recipe %q: %s", recipeDetails.TemplatePath, recipeName, err.Error()))
	}

	recipeDetails.Parameters = make(map[string]any)

	for key, value := range recipeData["parameters"].(map[string]interface{}) {
		if key == "context" {
			// context parameter is only revelant to operator.
			continue
		}

		details := ""
		values := value.(map[string]interface{})

		for k, v := range values {
			details += k + " : " + v.(string) + "\t"
		}

		recipeDetails.Parameters[key] = details
	}
	return nil
}
