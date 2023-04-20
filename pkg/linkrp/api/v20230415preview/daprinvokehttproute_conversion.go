// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20230415preview

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
)

// ConvertTo converts from the versioned DaprInvokeHttpRoute resource to version-agnostic datamodel.
func (src *DaprInvokeHTTPRouteResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.DaprInvokeHttpRoute{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			},
		},
		Properties: datamodel.DaprInvokeHttpRouteProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: to.String(src.Properties.Environment),
				Application: to.String(src.Properties.Application),
			},
			AppId: to.String(src.Properties.AppID),
		},
	}

	if src.Properties.Recipe != nil {
		converted.Properties.Recipe = toRecipeDataModel(src.Properties.Recipe)
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned DaprInvokeHttpRoute resource.
func (dst *DaprInvokeHTTPRouteResource) ConvertFrom(src v1.DataModelInterface) error {
	daprHttpRoute, ok := src.(*datamodel.DaprInvokeHttpRoute)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(daprHttpRoute.ID)
	dst.Name = to.Ptr(daprHttpRoute.Name)
	dst.Type = to.Ptr(daprHttpRoute.Type)
	dst.SystemData = fromSystemDataModel(daprHttpRoute.SystemData)
	dst.Location = to.Ptr(daprHttpRoute.Location)
	dst.Tags = *to.StringMapPtr(daprHttpRoute.Tags)
	dst.Properties = &DaprInvokeHTTPRouteProperties{
		Status: &ResourceStatus{
			OutputResources: rpv1.BuildExternalOutputResources(daprHttpRoute.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(daprHttpRoute.InternalMetadata.AsyncProvisioningState),
		Environment:       to.Ptr(daprHttpRoute.Properties.Environment),
		Application:       to.Ptr(daprHttpRoute.Properties.Application),
		AppID:             to.Ptr(daprHttpRoute.Properties.AppId),
	}

	if daprHttpRoute.Properties.Recipe.Name != "" {
		dst.Properties.Recipe = fromRecipeDataModel(daprHttpRoute.Properties.Recipe)
	}

	return nil
}
