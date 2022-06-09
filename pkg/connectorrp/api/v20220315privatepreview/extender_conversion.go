// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"reflect"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned Extender resource to version-agnostic datamodel.
func (src *ExtenderResource) ConvertTo() (conv.DataModelInterface, error) {
	outputResources := v1.ResourceStatus{}.OutputResources
	if src.Properties.Status != nil {
		outputResources = src.Properties.Status.OutputResources
	}
	converted := &datamodel.Extender{
		TrackedResource: v1.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.ExtenderProperties{
			BasicResourceProperties: v1.BasicResourceProperties{
				Status: v1.ResourceStatus{
					OutputResources: outputResources,
				},
			},
			ProvisioningState:    toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Environment:          to.String(src.Properties.Environment),
			Application:          to.String(src.Properties.Application),
			AdditionalProperties: src.Properties.AdditionalProperties,
			Secrets:              src.Properties.Secrets,
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
	var outputresources []map[string]interface{}
	if !(reflect.DeepEqual(extender.Properties.Status, v1.ResourceStatus{})) {
		outputresources = extender.Properties.Status.OutputResources
	}
	dst.Properties = &ExtenderProperties{
		BasicResourceProperties: BasicResourceProperties{
			Status: &ResourceStatus{
				OutputResources: outputresources,
			},
		},
		ProvisioningState:    fromProvisioningStateDataModel(extender.Properties.ProvisioningState),
		Environment:          to.StringPtr(extender.Properties.Environment),
		Application:          to.StringPtr(extender.Properties.Application),
		AdditionalProperties: extender.Properties.AdditionalProperties,
		Secrets:              extender.Properties.Secrets,
	}
	return nil
}
