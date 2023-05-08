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
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
)

// ConvertTo converts from the versioned HTTPRoute resource to version-agnostic datamodel.
func (src *HTTPRouteResource) ConvertTo() (v1.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.
	// TODO: Improve the validation.
	converted := &datamodel.HTTPRoute{
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
		Properties: &datamodel.HTTPRouteProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: to.String(src.Properties.Application),
			},
			Hostname: to.String(src.Properties.Hostname),
			Port:     to.Int32(src.Properties.Port),
			Scheme:   to.String(src.Properties.Scheme),
			URL:      to.String(src.Properties.URL),
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned HTTPRoute resource.
func (dst *HTTPRouteResource) ConvertFrom(src v1.DataModelInterface) error {
	// TODO: Improve the validation.
	route, ok := src.(*datamodel.HTTPRoute)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(route.ID)
	dst.Name = to.Ptr(route.Name)
	dst.Type = to.Ptr(route.Type)
	dst.SystemData = fromSystemDataModel(route.SystemData)
	dst.Location = to.Ptr(route.Location)
	dst.Tags = *to.StringMapPtr(route.Tags)
	dst.Properties = &HTTPRouteProperties{
		Status: &ResourceStatus{
			OutputResources: rpv1.BuildExternalOutputResources(route.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(route.InternalMetadata.AsyncProvisioningState),
		Application:       to.Ptr(route.Properties.Application),
		Hostname:          to.Ptr(route.Properties.Hostname),
		Port:              to.Ptr(route.Properties.Port),
		Scheme:            to.Ptr(route.Properties.Scheme),
		URL:               to.Ptr(route.Properties.URL),
	}

	return nil
}
