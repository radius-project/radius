// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned Extender resource to version-agnostic datamodel.
func (src *ExtenderResource) ConvertTo() (conv.DataModelInterface, error) {
	converted := &datamodel.Extender{
		TrackedResource: v1.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.ExtenderProperties{
			ExtenderResponseProperties: datamodel.ExtenderResponseProperties{
				ProvisioningState:    toProvisioningStateDataModel(src.Properties.ProvisioningState),
				Environment:          to.String(src.Properties.Environment),
				Application:          to.String(src.Properties.Application),
				AdditionalProperties: src.Properties.AdditionalProperties,
			},
			Secrets: src.Properties.Secrets,
		},
		InternalMetadata: v1.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertTo converts from the versioned ExtenderResponse resource to version-agnostic datamodel.
func (src *ExtenderResponseResource) ConvertTo() (conv.DataModelInterface, error) {
	converted := &datamodel.ExtenderResponse{
		TrackedResource: v1.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.ExtenderResponseProperties{
			ProvisioningState:    toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Environment:          to.String(src.Properties.Environment),
			Application:          to.String(src.Properties.Application),
			AdditionalProperties: src.Properties.AdditionalProperties,
		},
		InternalMetadata: v1.InternalMetadata{
			UpdatedAPIVersion: Version,
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
		ExtenderResponseProperties: ExtenderResponseProperties{
			BasicResourceProperties: BasicResourceProperties{
				Status: &ResourceStatus{
					OutputResources: v1.BuildExternalOutputResources(extender.Properties.Status.OutputResources),
				},
			},
			ProvisioningState:    fromProvisioningStateDataModel(extender.Properties.ProvisioningState),
			Environment:          to.StringPtr(extender.Properties.Environment),
			Application:          to.StringPtr(extender.Properties.Application),
			AdditionalProperties: extender.Properties.AdditionalProperties,
		},
		Secrets: extender.Properties.Secrets,
	}
	return nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned ExtenderResponse resource.
func (dst *ExtenderResponseResource) ConvertFrom(src conv.DataModelInterface) error {
	extender, ok := src.(*datamodel.ExtenderResponse)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(extender.ID)
	dst.Name = to.StringPtr(extender.Name)
	dst.Type = to.StringPtr(extender.Type)
	dst.SystemData = fromSystemDataModel(extender.SystemData)
	dst.Location = to.StringPtr(extender.Location)
	dst.Tags = *to.StringMapPtr(extender.Tags)
	dst.Properties = &ExtenderResponseProperties{
		BasicResourceProperties: BasicResourceProperties{
			Status: &ResourceStatus{
				OutputResources: v1.BuildExternalOutputResources(extender.Properties.Status.OutputResources),
			},
		},
		ProvisioningState:    fromProvisioningStateDataModel(extender.Properties.ProvisioningState),
		Environment:          to.StringPtr(extender.Properties.Environment),
		Application:          to.StringPtr(extender.Properties.Application),
		AdditionalProperties: extender.Properties.AdditionalProperties,
	}
	return nil
}
