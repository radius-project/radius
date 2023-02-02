// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertFrom converts from version-agnostic datamodel to the versioned Environment recipe properties resource.
func (dst *EnvironmentRecipeProperties) ConvertFrom(src v1.DataModelInterface) error {
	recipe, ok := src.(*datamodel.EnvironmentRecipeProperties)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.LinkType = to.StringPtr(recipe.LinkType)
	dst.TemplatePath = to.StringPtr(recipe.TemplatePath)
	dst.Parameters = recipe.Parameters
	return nil
}
