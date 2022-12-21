// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned Extender resource to version-agnostic datamodel.
func (src *ExtenderResource) ConvertTo() (conv.DataModelInterface, error) {
	converted := &datamodel.Extender{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		},
		Properties: datamodel.ExtenderProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: to.String(src.Properties.Environment),
				Application: to.String(src.Properties.Application),
			},
			ProvisioningState:    toProvisioningStateDataModel(src.Properties.ProvisioningState),
			AdditionalProperties: src.Properties.AdditionalProperties,
			Secrets:              src.Properties.Secrets,
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Extender resource.
func (dst *ExtenderResource) ConvertFrom(src conv.DataModelInterface) error {
	extender, ok := src.(*datamodel.Extender)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(extender.ID)
	dst.Name = to.StringPtr(extender.Name)
	dst.Type = to.StringPtr(extender.Type)
	dst.SystemData = fromSystemDataModel(extender.SystemData)
	dst.Location = to.StringPtr(extender.Location)
	dst.Tags = *to.StringMapPtr(extender.Tags)
	dst.Properties = &ExtenderProperties{
		Status: &ResourceStatus{
			OutputResources: rp.BuildExternalOutputResources(extender.Properties.Status.OutputResources),
		},
		ProvisioningState:    fromProvisioningStateDataModel(extender.Properties.ProvisioningState),
		Environment:          to.StringPtr(extender.Properties.Environment),
		Application:          to.StringPtr(extender.Properties.Application),
		AdditionalProperties: extender.Properties.AdditionalProperties,

		// Secrets are omitted.
	}
	return nil
}
