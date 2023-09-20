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

package v20220315privatepreview

import (
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	types "github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/to"
)

// ConvertTo returns an error as it does not support converting Environment Recipe Properties to a version-agnostic object.
func (src *RecipeGetMetadataResponse) ConvertTo() (v1.DataModelInterface, error) {
	return nil, fmt.Errorf("converting Environment Recipe Properties to a version-agnostic object is not supported")
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Environment recipe properties resource.
func (dst *RecipeGetMetadataResponse) ConvertFrom(src v1.DataModelInterface) error {
	recipe, ok := src.(*datamodel.EnvironmentRecipeProperties)
	if !ok {
		return v1.ErrInvalidModelConversion
	}
	dst.TemplateKind = to.Ptr(recipe.TemplateKind)
	dst.TemplatePath = to.Ptr(recipe.TemplatePath)
	if recipe.TemplateKind == types.TemplateKindTerraform {
		dst.TemplateVersion = to.Ptr(recipe.TemplateVersion)
	}
	dst.Parameters = recipe.Parameters
	return nil
}

// ConvertTo converts from the versioned Environment Recipe Properties resource to version-agnostic datamodel.
func (src *RecipeGetMetadata) ConvertTo() (v1.DataModelInterface, error) {
	return &datamodel.Recipe{
		Name:         to.String(src.Name),
		ResourceType: to.String(src.ResourceType),
	}, nil
}
