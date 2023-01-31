// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/rp/util"
)

var _ ctrl.Controller = (*GetRecipeDetails)(nil)

// GetRecipe is the controller implementation to create or update environment resource.
type GetRecipeDetails struct {
	ctrl.Operation[*datamodel.Environment, datamodel.Environment]
}

// NewGetRecipe creates a new CreateOrUpdateEnvironment.
func NewGetRecipDetailse(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetRecipeDetails{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Environment]{
				RequestConverter:  converter.EnvironmentDataModelFromVersioned,
				ResponseConverter: converter.EnvironmentDataModelToVersioned,
			},
		),
	}, nil
}

func (e *GetRecipeDetails) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	res, err := e.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	// Expected input:
	// {
	//  <required-attributes>
	// 	"properties": {
	// 		"recipes": {
	// 			<recipe-name>: {
	// 				"linkType": <link-type>,
	// 				"templatePath": <template-path>,
	// 			}
	// 		},
	// 	},
	// }
	var recipe datamodel.EnvironmentRecipeProperties
	var recipeName = ""
	for k, v := range res.Properties.Recipes {
		if recipeName != "" {
			return rest.NewBadRequestResponse("Only one recipe should be specified in the request."), nil
		}
		recipeName = k
		recipe = v
	}

	GetRecipeDetailsFromRegistry(ctx, &recipe, recipeName)
	res.Properties.Recipes[recipeName] = recipe
	versioned, err := e.ResponseConverter()(res, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}
	return rest.NewOKResponse(versioned), nil
}

func GetRecipeDetailsFromRegistry(ctx context.Context, recipeDetails *datamodel.EnvironmentRecipeProperties, recipeName string) error {
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
		keys := make([]string, 0, len(values))

		for k := range values {
			keys = append(keys, k)
		}

		// to keep order of parameters details consistent - sort.
		sort.Sort(sort.Reverse(sort.StringSlice(keys)))
		for _, k := range keys {
			details += k + " : " + values[k].(string) + "\t"
		}

		recipeDetails.Parameters[key] = details
	}
	return nil
}
