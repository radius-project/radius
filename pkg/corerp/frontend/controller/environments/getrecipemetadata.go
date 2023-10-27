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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	pr_dm "github.com/radius-project/radius/pkg/portableresources/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/engine"
	"golang.org/x/exp/maps"
)

var _ ctrl.Controller = (*GetRecipeMetadata)(nil)

// GetRecipeMetadata is the controller implementation to get recipe metadata such as parameters and the details of those parameters(type/minValue/etc.).
type GetRecipeMetadata struct {
	ctrl.Operation[*datamodel.Environment, datamodel.Environment]
	engine.Engine
}

// NewGetRecipeMetadata creates a new controller for retrieving recipe metadata from an environment.
func NewGetRecipeMetadata(opts ctrl.Options, engine engine.Engine) (ctrl.Controller, error) {
	return &GetRecipeMetadata{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Environment]{
				RequestConverter:  converter.EnvironmentDataModelFromVersioned,
				ResponseConverter: converter.EnvironmentDataModelToVersioned,
			},
		),
		engine,
	}, nil
}

// Run retrieves the recipe metadata from the registry for a given recipe name and template path, and returns
// a response containing the recipe parameters.
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
	recipe, exists := resource.Properties.Recipes[recipeDatamodel.ResourceType]
	if exists {
		recipeProperties, exists = recipe[recipeDatamodel.Name]
	}
	if !exists {
		return rest.NewNotFoundMessageResponse(fmt.Sprintf("Either recipe with name %q or resource type %q not found on environment with id %q", recipeDatamodel.Name, recipeDatamodel.ResourceType, serviceCtx.ResourceID)), nil
	}

	recipeParams, err := r.GetRecipeMetadataFromRegistry(ctx, recipeProperties, recipeDatamodel)
	if err != nil {
		return nil, err
	}

	ret := datamodel.EnvironmentRecipeProperties{
		TemplateKind:    recipeProperties.TemplateKind,
		TemplatePath:    recipeProperties.TemplatePath,
		TemplateVersion: recipeProperties.TemplateVersion,
		Parameters:      recipeParams,
	}

	versioned, err := converter.EnvironmentRecipePropertiesDataModelToVersioned(&ret, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}
	return rest.NewOKResponse(versioned), nil
}

func (r *GetRecipeMetadata) GetRecipeMetadataFromRegistry(ctx context.Context, recipeProperties datamodel.EnvironmentRecipeProperties, recipeDataModel *datamodel.Recipe) (recipeParameters map[string]any, err error) {
	recipeDefinition := recipes.EnvironmentDefinition{
		Name:            recipeDataModel.Name,
		Driver:          recipeProperties.TemplateKind,
		Parameters:      recipeProperties.Parameters,
		TemplatePath:    recipeProperties.TemplatePath,
		TemplateVersion: recipeProperties.TemplateVersion,
		ResourceType:    recipeDataModel.ResourceType,
	}

	recipeParameters = make(map[string]any)
	recipeData, err := r.Engine.GetRecipeMetadata(ctx, recipeDefinition)
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
	if recipeData["parameters"] == nil {
		return nil
	}
	recipeParam, ok := recipeData["parameters"].(map[string]any)
	if !ok {
		return fmt.Errorf("parameters are not in expected format")
	}

	for paramName, paramValue := range recipeParam {
		if paramName == pr_dm.RecipeContextParameter {
			// context parameter is only relevant to operator and is generated and passed by resource provider instead of the developer/operators.
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
