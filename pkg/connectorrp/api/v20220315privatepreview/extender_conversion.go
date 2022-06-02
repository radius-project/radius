// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"reflect"

	"github.com/project-radius/radius/pkg/api"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned Extender resource to version-agnostic datamodel.
func (src *ExtenderResource) ConvertTo() (api.DataModelInterface, error) {
	outputResources := basedatamodel.ResourceStatus{}.OutputResources
	if src.Properties.Status != nil {
		outputResources = src.Properties.Status.OutputResources
	}
	converted := &datamodel.Extender{
		TrackedResource: basedatamodel.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.ExtenderProperties{
			BasicResourceProperties: basedatamodel.BasicResourceProperties{
				Status: basedatamodel.ResourceStatus{
					OutputResources: outputResources,
				},
			},
			ProvisioningState:    toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Environment:          to.String(src.Properties.Environment),
			Application:          to.String(src.Properties.Application),
			AdditionalProperties: src.Properties.AdditionalProperties,
			Secrets:              src.Properties.Secrets,
		},
		InternalMetadata: basedatamodel.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Extender resource.
func (dst *ExtenderResource) ConvertFrom(src api.DataModelInterface) error {
	extender, ok := src.(*datamodel.Extender)
	if !ok {
		return api.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(extender.ID)
	dst.Name = to.StringPtr(extender.Name)
	dst.Type = to.StringPtr(extender.Type)
	dst.SystemData = fromSystemDataModel(extender.SystemData)
	dst.Location = to.StringPtr(extender.Location)
	dst.Tags = *to.StringMapPtr(extender.Tags)
	var outputresources []map[string]interface{}
	if !(reflect.DeepEqual(extender.Properties.Status, basedatamodel.ResourceStatus{})) {
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
