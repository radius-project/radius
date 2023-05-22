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
	resource, _, err := r.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}
	content, err := ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}
	recipeDatamodel, err := converter.RecipeDataModelFromVersioned(content, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}
	var recipeProperties datamodel.EnvironmentRecipeProperties
	recipe, exists := resource.Properties.Recipes[recipeDatamodel.LinkType]
	if exists {
		recipeProperties, exists = recipe[recipeDatamodel.Name]
	}
	if !exists {
		return rest.NewNotFoundMessageResponse(fmt.Sprintf("Either recipe with name %q or resource type %q not found on environment with id %q", recipeDatamodel.Name, recipeDatamodel.LinkType, serviceCtx.ResourceID)), nil
	}

	recipeParams, err := getRecipeMetadataFromRegistry(ctx, recipeProperties.TemplatePath, recipeDatamodel.Name)
	if err != nil {
		return nil, err
	}

	ret := datamodel.EnvironmentRecipeProperties{
		TemplateKind: recipeProperties.TemplateKind,
		TemplatePath: recipeProperties.TemplatePath,
		Parameters:   recipeParams,
	}

	versioned, err := converter.EnvironmentRecipePropertiesDataModelToVersioned(&ret, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}
	return rest.NewOKResponse(versioned), nil
}

func getRecipeMetadataFromRegistry(ctx context.Context, templatePath string, recipeName string) (recipeParameters map[string]any, err error) {
	recipeParameters = make(map[string]any)
	recipeData := make(map[string]any)
	err = util.ReadFromRegistry(ctx, templatePath, &recipeData)
	if err != nil {
		return recipeParameters, err
	}

	err = parseAndFormatRecipeParams(recipeData, recipeParameters)
	if err != nil {
		return recipeParameters, err
	}

	return recipeParameters, nil
}

func parseAndFormatRecipeParams(recipeData map[string]any, recipeParameters map[string]any) error {
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
		return nil
	}
	recipeParam, ok := recipeData["parameters"].(map[string]any)
	if !ok {
		return fmt.Errorf("parameters are not in expected format")
	}

	for paramName, paramValue := range recipeParam {
		if paramName == linkrp.RecipeContextParameter {
			// context parameter is only revelant to operator and is generated and passed by linkrp instead of the developer/operators.
			continue
		}

		details := make(map[string]any)
		paramDetails, ok := paramValue.(map[string]any)
		if !ok {
			return fmt.Errorf("parameter details are not in expected format")
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

			recipeParameters[paramName] = details
		}
	}

	return nil
}
