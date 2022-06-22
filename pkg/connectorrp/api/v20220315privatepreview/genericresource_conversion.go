// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
)

// ConvertTo converts from the versioned resource to version-agnostic datamodel.
func (gr *GenericResource) ConvertTo() (conv.DataModelInterface, error) {
	return &datamodel.GenericResourceVersionAgnostic{
		TrackedResource: v1.TrackedResource{
			ID:       to.String(gr.ID),
			Name:     to.String(gr.Name),
			Type:     to.String(gr.Type),
			Location: to.String(gr.Location),
			Tags:     to.StringMap(gr.Tags),
		},
		ResourceProperties: gr.ResourceProperties,
	}, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned resource.
func (dst *GenericResource) ConvertFrom(src conv.DataModelInterface) error {
	resource, ok := src.(*datamodel.GenericResourceVersionAgnostic)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(resource.ID)
	dst.Name = to.StringPtr(resource.Name)
	dst.Type = to.StringPtr(resource.Type)
	dst.SystemData = fromSystemDataModel(resource.SystemData)
	dst.Location = to.StringPtr(resource.Location)
	dst.Tags = *to.StringMapPtr(resource.Tags)
	dst.ResourceProperties = resource.ResourceProperties

	return nil
}
