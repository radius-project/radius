// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220901privatepreview

import (
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

// ConvertTo converts from the versioned Environment resource to version-agnostic datamodel.
func (src *ResourceGroupResource) ConvertTo() (conv.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.

	converted := &datamodel.ResourceGroup{
		TrackedResource: v1.TrackedResource{
			Name: to.String(src.Name),
			ID:   to.String(src.ID),
		},
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned ResourceGroup resource.
func (dst *ResourceGroupResource) ConvertFrom(src conv.DataModelInterface) error {
	// TODO: Improve the validation.
	rg, ok := src.(*datamodel.ResourceGroup)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = &rg.TrackedResource.ID
	dst.Name = &rg.TrackedResource.Name
	dst.Type = &rg.TrackedResource.Type

	return nil
}
