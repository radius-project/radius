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

package v20231001preview

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/to"
)

// ConvertTo converts from the versioned RecipePackResource to version-agnostic datamodel.
func (src *RecipePackResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.RecipePack{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				CreatedAPIVersion:      Version,
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			},
		},
		Properties: datamodel.RecipePackProperties{},
	}

	if src.Properties != nil {
		if src.Properties.Description != nil {
			converted.Properties.Description = src.Properties.Description
		}

		if src.Properties.Recipes != nil {
			recipes := make(map[string]datamodel.RecipePackDefinition)
			for resourceType, recipe := range src.Properties.Recipes {
				if recipe != nil {
					recipes[resourceType] = datamodel.RecipePackDefinition{
						RecipeKind:     string(*recipe.RecipeKind),
						RecipeLocation: to.String(recipe.RecipeLocation),
						Parameters:     recipe.Parameters,
					}
				}
			}
			converted.Properties.Recipes = recipes
		}
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned RecipePackResource.
func (dst *RecipePackResource) ConvertFrom(src v1.DataModelInterface) error {
	recipePack, ok := src.(*datamodel.RecipePack)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(recipePack.ID)
	dst.Name = to.Ptr(recipePack.Name)
	dst.Type = to.Ptr(recipePack.Type)
	dst.SystemData = fromSystemDataModel(recipePack.SystemData)
	dst.Location = to.Ptr(recipePack.Location)
	dst.Tags = *to.StringMapPtr(recipePack.Tags)
	dst.Properties = &RecipePackProperties{
		ProvisioningState: fromProvisioningStateDataModel(recipePack.InternalMetadata.AsyncProvisioningState),
	}

	if recipePack.Properties.Description != nil {
		dst.Properties.Description = recipePack.Properties.Description
	}

	if recipePack.Properties.Recipes != nil {
		recipes := make(map[string]*RecipeDefinition)
		for resourceType, recipe := range recipePack.Properties.Recipes {
			recipeKind := RecipeKind(recipe.RecipeKind)
			recipes[resourceType] = &RecipeDefinition{
				RecipeKind:     &recipeKind,
				RecipeLocation: to.Ptr(recipe.RecipeLocation),
				Parameters:     recipe.Parameters,
			}
		}
		dst.Properties.Recipes = recipes
	}

	return nil
}