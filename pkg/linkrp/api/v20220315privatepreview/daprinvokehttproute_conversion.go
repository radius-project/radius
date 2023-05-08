/*
------------------------------------------------------------
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
------------------------------------------------------------
*/

package v20220315privatepreview

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
