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

// ConvertTo converts from the versioned DaprInvokeHttpRoute resource to version-agnostic datamodel.
func (src *DaprInvokeHTTPRouteResource) ConvertTo() (api.DataModelInterface, error) {
	outputResources := basedatamodel.ResourceStatus{}.OutputResources
	if src.Properties.Status != nil {
		outputResources = src.Properties.Status.OutputResources
	}
	converted := &datamodel.DaprInvokeHttpRoute{
		TrackedResource: basedatamodel.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.DaprInvokeHttpRouteProperties{
			BasicResourceProperties: basedatamodel.BasicResourceProperties{
				Status: basedatamodel.ResourceStatus{
					OutputResources: outputResources,
				},
			},
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Environment:       to.String(src.Properties.Environment),
			Application:       to.String(src.Properties.Application),
			AppId:             to.String(src.Properties.AppID),
		},
		InternalMetadata: basedatamodel.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned DaprInvokeHttpRoute resource.
func (dst *DaprInvokeHTTPRouteResource) ConvertFrom(src api.DataModelInterface) error {
	daprHttpRoute, ok := src.(*datamodel.DaprInvokeHttpRoute)
	if !ok {
		return api.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(daprHttpRoute.ID)
	dst.Name = to.StringPtr(daprHttpRoute.Name)
	dst.Type = to.StringPtr(daprHttpRoute.Type)
	dst.SystemData = fromSystemDataModel(daprHttpRoute.SystemData)
	dst.Location = to.StringPtr(daprHttpRoute.Location)
	dst.Tags = *to.StringMapPtr(daprHttpRoute.Tags)
	var outputresources []map[string]interface{}
	if !(reflect.DeepEqual(daprHttpRoute.Properties.Status, basedatamodel.ResourceStatus{})) {
		outputresources = daprHttpRoute.Properties.Status.OutputResources
	}
	dst.Properties = &DaprInvokeHTTPRouteProperties{
		BasicResourceProperties: BasicResourceProperties{
			Status: &ResourceStatus{
				OutputResources: outputresources,
			},
		},
		ProvisioningState: fromProvisioningStateDataModel(daprHttpRoute.Properties.ProvisioningState),
		Environment:       to.StringPtr(daprHttpRoute.Properties.Environment),
		Application:       to.StringPtr(daprHttpRoute.Properties.Application),
		AppID:             to.StringPtr(daprHttpRoute.Properties.AppId),
	}
	return nil
}
