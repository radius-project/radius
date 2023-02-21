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
	"golang.org/x/exp/maps"
)

var _ ctrl.Controller = (*GetRecipeMetadata)(nil)

// GetRecipeMetadata is the controller implementation to get recipe metadata such as parameters and the details of those parameters(type/minValue/etc.).
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

func (r *GetRecipeMetadata) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	// Request route for getrecipemetadata has name of the recipe as a part of the url.
	// route id format: subscriptions/<subscription_id>/resourceGroups/<resource_group>/providers/Applications.Core/environments/<environment_name>/<recipe_name>/recipemetadata
	recipeSuffix := strings.Split(serviceCtx.OrignalURL.Path, "/environments/")[1]
	recipeName := strings.Split(recipeSuffix, "/")[1]
	resource, _, err := r.GetResource(ctx, serviceCtx.ResourceID)
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

	if recipeData["parameters"] == nil {
		return recipePrameters, nil
	}
	recipeParam, ok := recipeData["parameters"].(map[string]any)
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
			keys := maps.Keys(paramDetails)

			// to keep order of parameters details consistent - sort. Reverse sorting will ensure type (a required detail) is always first.
			sort.Sort(sort.Reverse(sort.StringSlice(keys)))
			for _, paramDetailName := range keys {
				if paramDetailName == "metadata" {
					// skip metadata for now as it is a nested object.
					continue
				}

				details[paramDetailName] = paramDetails[paramDetailName]
			}

			recipePrameters[paramName] = details
		}
	}

	return recipePrameters, nil
}
