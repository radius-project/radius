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

// GetRecipe is the controller implementation to get recipe details.
type GetRecipeDetails struct {
	ctrl.Operation[*datamodel.Environment, datamodel.Environment]
}

// NewGetRecipeDetails creates a new GetRecipeDetails.
func NewGetRecipeDetails(opts ctrl.Options) (ctrl.Controller, error) {
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

	// Request route for getrecipedetails has name of the operation as suffix which should be removed to get the resource id.
	// route id format: subscriptions/<subscription_id>/resourceGroups/<resource_group>/providers/Applications.Core/environments/<environment_name>/getrecipedetails/<recipe_name>
	parsedResourceID := serviceCtx.ResourceID.Truncate()
	recipeName := serviceCtx.ResourceID.FindType("getrecipedetails")
	resource, _, err := e.GetResource(ctx, parsedResourceID)
	if err != nil {
		return nil, err
	}

	if resource == nil {
		return rest.NewNotFoundResponse(parsedResourceID), nil
	}

	recipe, exists := resource.Properties.Recipes[recipeName]
	if !exists {
		return rest.NewNotFoundMessageResponse(fmt.Sprintf("Recipe with name %q not found on environment with id %q", recipeName, parsedResourceID)), nil
	}

	err = GetRecipeDetailsFromRegistry(ctx, &recipe, recipeName)
	if err != nil {
		return nil, v1.NewClientErrInvalidRequest(err.Error())
	}

	versioned, err := converter.EnvironmentRecipePropertiesDataModelToVersioned(&recipe, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}
	return rest.NewOKResponse(versioned), nil
}

func GetRecipeDetailsFromRegistry(ctx context.Context, recipeDetails *datamodel.EnvironmentRecipeProperties, recipeName string) error {
	recipeData := make(map[string]any)
	err := util.ReadFromRegistry(ctx, recipeDetails.TemplatePath, &recipeData)
	if err != nil {
		return fmt.Errorf("failed to fetch template from the path %q for recipe %q: %s", recipeDetails.TemplatePath, recipeName, err.Error())
	}

	recipeDetails.Parameters = make(map[string]any)

	// Recipe parameters can be found in the recipe data pulled from the registry in the following format:
	// {
	//     "parameters": {
	//         <parameter-name>: {
	//				<parameter-constraint-name> : <parameter-constraint-value>
	//            }
	//        }
	// }
	// For example:
	// {
	//     "parameters": {
	//         "location": {
	//				"type" : "string",
	//              "tdefaultValue" : "[resourceGroup().location]"
	//            }
	//     }
	// }
	// We want to extract the parameters with their constraints and return the following:
	// {
	// 		<recipe-name>: {
	// 			"linkType": <link-type>,
	// 			"templatePath": <template-path>,
	// 			"parameters": {
	// 				<parameter-name>: <parameter-constraint-name> : <parameter-constraint-value>\t<parameter-constraint-name> : <parameter-constraint-value>
	//         	}
	// 		}
	// }
	// For example:
	// {
	// 		mongodb: {
	// 			"linkType": "Applications.Link/mongoDatabases",
	// 			"templatePath": "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0",
	// 			"parameters": {
	// 				"location": "type : string\tdefaultValue : [resourceGroup().location]\t"
	//        	 }
	// 		}
	// }

	recipeParam, ok := recipeData["parameters"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("failed to fetch parameters")
	}

	for key, value := range recipeParam {
		if key == "context" {
			// context parameter is only revelant to operator.
			continue
		}

		details := ""
		values, ok := value.(map[string]interface{})
		if !ok {
			return fmt.Errorf("failed to fetch parameter names")
		}

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
