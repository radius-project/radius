// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20230415preview

import (
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/to"
)

// ConvertTo converts from the versioned Environment Recipe Properties resource to version-agnostic datamodel.
func (src *EnvironmentRecipeProperties) ConvertTo() (v1.DataModelInterface, error) {
	return nil, fmt.Errorf("converting Environment Recipe Properties to a version-agnostic object is not supported")
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Environment recipe properties resource.
func (dst *EnvironmentRecipeProperties) ConvertFrom(src v1.DataModelInterface) error {
	recipe, ok := src.(*datamodel.EnvironmentRecipeProperties)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.LinkType = to.Ptr(recipe.LinkType)
	dst.TemplatePath = to.Ptr(recipe.TemplatePath)
	dst.Parameters = recipe.Parameters
	return nil
}
