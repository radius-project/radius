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

	dst.ID = new(recipePack.ID)
	dst.Name = new(recipePack.Name)
	dst.Type = new(recipePack.Type)
	dst.SystemData = fromSystemDataModel(&recipePack.SystemData)
	dst.Location = new(recipePack.Location)
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
			definition := &datamodel.RecipeDefinition{
				Kind:       toRecipeKindDataModel(recipe.Kind),
				Source:     to.String(recipe.Source),
				Parameters: recipe.Parameters,
				PlainHTTP:  to.Bool(recipe.PlainHTTP),
			}
			if recipe.Outputs != nil {
				definition.Outputs, definition.SecretOutputs = SplitRecipeOutputs(recipe.Outputs)
			}
			result[key] = definition
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
			definition := &RecipeDefinition{
				Kind:       fromRecipeKindDataModel(recipe.Kind),
				Source:     new(recipe.Source),
				Parameters: recipe.Parameters,
				PlainHTTP:  new(recipe.PlainHTTP),
			}
			if outputs := mergeRecipeOutputs(recipe.Outputs, recipe.SecretOutputs); outputs != nil {
				definition.Outputs = outputs
			}
			result[key] = definition
		}
	}
	return result
}

// recipeOutputsSecretsKey is the reserved key inside a recipe definition's `outputs` map whose value is
// a nested object mapping secret property names to module output names. Every other `outputs` entry maps
// a non-secret property name to a module output name (a string value). `secrets` is safe to reserve
// because it is the framework-owned secrets block name, so no resource type maps a real property named
// `secrets` through `outputs`.
const recipeOutputsSecretsKey = "secrets"

// SplitRecipeOutputs separates the API `outputs` map (property->output strings, plus a nested `secrets`
// object) into flat outputs and secret-outputs maps. It is exported so the recipe config loader, which
// reads the versioned model directly, can interpret `outputs` the same way as the datamodel converter.
func SplitRecipeOutputs(apiOutputs map[string]any) (map[string]string, map[string]string) {
	var outputs map[string]string
	var secretOutputs map[string]string
	for k, v := range apiOutputs {
		if k == recipeOutputsSecretsKey {
			nested, ok := v.(map[string]any)
			if !ok {
				continue
			}
			for sk, sv := range nested {
				s, ok := sv.(string)
				if !ok {
					continue
				}
				if secretOutputs == nil {
					secretOutputs = map[string]string{}
				}
				secretOutputs[sk] = s
			}
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		if outputs == nil {
			outputs = map[string]string{}
		}
		outputs[k] = s
	}
	return outputs, secretOutputs
}

// mergeRecipeOutputs combines the datamodel's flat Outputs and SecretOutputs maps back into the API
// `outputs` map, nesting the secret mappings under the reserved `secrets` key. It returns nil when both
// maps are empty.
func mergeRecipeOutputs(outputs map[string]string, secretOutputs map[string]string) map[string]any {
	if len(outputs) == 0 && len(secretOutputs) == 0 {
		return nil
	}
	apiOutputs := map[string]any{}
	for k, v := range outputs {
		apiOutputs[k] = v
	}
	if len(secretOutputs) > 0 {
		secrets := map[string]any{}
		for k, v := range secretOutputs {
			secrets[k] = v
		}
		apiOutputs[recipeOutputsSecretsKey] = secrets
	}
	return apiOutputs
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
