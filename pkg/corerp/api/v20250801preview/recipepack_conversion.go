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

package v20250801preview

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/to"
)

// ConvertTo converts from the versioned RecipePack resource to version-agnostic datamodel.
func (src *RecipePackResource) ConvertTo() (v1.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.
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

	// Convert Recipes
	if src.Properties.Recipes != nil {
		converted.Properties.Recipes = toRecipesDataModel(src.Properties.Recipes)
	}

	// Convert ReferencedBy
	if src.Properties.ReferencedBy != nil {
		converted.Properties.ReferencedBy = to.StringArray(src.Properties.ReferencedBy)
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned RecipePack resource.
func (dst *RecipePackResource) ConvertFrom(src v1.DataModelInterface) error {
	recipePack, ok := src.(*datamodel.RecipePack)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(recipePack.ID)
	dst.Name = to.Ptr(recipePack.Name)
	dst.Type = to.Ptr(recipePack.Type)
	dst.SystemData = fromSystemDataModel(&recipePack.SystemData)
	dst.Location = to.Ptr(recipePack.Location)
	dst.Tags = *to.StringMapPtr(recipePack.Tags)
	dst.Properties = &RecipePackProperties{
		ProvisioningState: fromProvisioningStateDataModel(recipePack.InternalMetadata.AsyncProvisioningState),
	}

	// Convert Recipes
	if recipePack.Properties.Recipes != nil {
		dst.Properties.Recipes = fromRecipesDataModel(recipePack.Properties.Recipes)
	}

	// Convert ReferencedBy
	if len(recipePack.Properties.ReferencedBy) > 0 {
		dst.Properties.ReferencedBy = to.ArrayofStringPtrs(recipePack.Properties.ReferencedBy)
	}

	return nil
}

func toRecipesDataModel(recipes map[string]*RecipeDefinition) map[string]*datamodel.RecipeDefinition {
	if recipes == nil {
		return nil
	}

	result := make(map[string]*datamodel.RecipeDefinition)
	for key, recipe := range recipes {
		if recipe != nil {
			result[key] = &datamodel.RecipeDefinition{
				RecipeKind:     toRecipeKindDataModel(recipe.RecipeKind),
				RecipeLocation: to.String(recipe.RecipeLocation),
				Parameters:     recipe.Parameters,
				PlainHTTP:      to.Bool(recipe.PlainHTTP),
			}
		}
	}
	return result
}

func fromRecipesDataModel(recipes map[string]*datamodel.RecipeDefinition) map[string]*RecipeDefinition {
	if recipes == nil {
		return nil
	}

	result := make(map[string]*RecipeDefinition)
	for key, recipe := range recipes {
		if recipe != nil {
			result[key] = &RecipeDefinition{
				RecipeKind:     fromRecipeKindDataModel(recipe.RecipeKind),
				RecipeLocation: to.Ptr(recipe.RecipeLocation),
				Parameters:     recipe.Parameters,
				PlainHTTP:      to.Ptr(recipe.PlainHTTP),
			}
		}
	}
	return result
}

func toRecipeKindDataModel(kind *RecipeKind) string {
	if kind == nil {
		return ""
	}
	return string(*kind)
}

func fromRecipeKindDataModel(kind string) *RecipeKind {
	if kind == "" {
		return nil
	}
	recipeKind := RecipeKind(kind)
	return &recipeKind
}
