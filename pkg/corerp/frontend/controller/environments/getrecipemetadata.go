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
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	linkrp "github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp/util"
)

var _ ctrl.Controller = (*GetRecipeMetadata)(nil)

// GetRecipeMetadata is the controller implementation to get recipe metadata.
type GetRecipeMetadata struct {
	ctrl.Operation[*datamodel.Environment, datamodel.Environment]
}

// NewGetRecipeMetadata creates a new GetRecipeMetadata controller.
func NewGetRecipeMetadata(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetRecipeMetadata{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Environment]{
				RequestConverter:  converter.EnvironmentDataModelFromVersioned,
				ResponseConverter: converter.EnvironmentDataModelToVersioned,
			},
		),
	}, nil
}

func (e *GetRecipeMetadata) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	// Request route for getrecipemetadata has name of the recipe as suffix.
	// route id format: subscriptions/<subscription_id>/resourceGroups/<resource_group>/providers/Applications.Core/environments/<environment_name>/getrecipemetadata/<recipe_name>
	recipeName := strings.Split(serviceCtx.OrignalURL.Path, "getrecipemetadata/")[1]
	resource, _, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if resource == nil {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	recipe, exists := resource.Properties.Recipes[recipeName]
	if !exists {
		return rest.NewNotFoundMessageResponse(fmt.Sprintf("Recipe with name %q not found on environment with id %q", recipeName, serviceCtx.ResourceID)), nil
	}

	recipeParams, err := getRecipeMetadataFromRegistry(ctx, recipe.TemplatePath, recipeName)
	if err != nil {
		return nil, err
	}

	ret := datamodel.EnvironmentRecipeProperties{
		LinkType:     recipe.LinkType,
		TemplatePath: recipe.TemplatePath,
		Parameters:   recipeParams,
	}

	versioned, err := converter.EnvironmentRecipePropertiesDataModelToVersioned(&ret, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}
	return rest.NewOKResponse(versioned), nil
}

func getRecipeMetadataFromRegistry(ctx context.Context, templatePath string, recipeName string) (recipePrameters map[string]any, err error) {
	recipePrameters = make(map[string]any)
	recipeData := make(map[string]any)
	err = util.ReadFromRegistry(ctx, templatePath, &recipeData)
	if err != nil {
		return recipePrameters, err
	}

	// Recipe parameters can be found in the recipe data pulled from the registry in the following format:
	//	{
	//		"parameters": {
	//			<parameter-name>: {
	//				<parameter-constraint-name> : <parameter-constraint-value>
	// 			}
	//		}
	//	}
	// For example:
	//	{
	//		"parameters": {
	//			"location": {
	//				"type": "string",
	//				"defaultValue" : "[resourceGroup().location]"
	//			}
	//		}
	//	}

	params, ok := recipeData["parameters"]
	if !ok {
		return recipePrameters, nil
	}

	recipeParam, ok := params.(map[string]any)
	if !ok {
		return recipePrameters, fmt.Errorf("parameters are not in expected format")
	}

	for paramName, paramValue := range recipeParam {
		if paramName == linkrp.RecipeContextParameter {
			// context parameter is only revelant to operator and is generated and passed by linkrp instead of the developer/operators.
			continue
		}

		details := make(map[string]any)
		paramDetails, ok := paramValue.(map[string]any)
		if !ok {
			return recipePrameters, fmt.Errorf("parameter details are not in expected format")
		}

		if len(paramDetails) > 0 {
			keys := make([]string, 0, len(paramDetails))

			for k := range paramDetails {
				keys = append(keys, k)
			}

			// to keep order of parameters details consistent - sort.
			sort.Sort(sort.Reverse(sort.StringSlice(keys)))
			for _, paramDetailName := range keys {
				if paramDetailName == "metadata" {
					// skip metadata details for now as it is the description of the parameter.
					continue
				}

				details[paramDetailName] = paramDetails[paramDetailName]
			}

			recipePrameters[paramName] = details
		}
	}

	return recipePrameters, nil
}
