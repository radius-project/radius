/*
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
*/

package v20250801preview

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
)

// ConvertTo converts from the versioned ApplicationResource to version-agnostic datamodel.
func (src *ApplicationResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.Application_v20250801preview{
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
		Properties: datamodel.ApplicationProperties_v20250801preview{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: to.String(src.Properties.Environment),
			},
		},
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned ApplicationResource.
func (dst *ApplicationResource) ConvertFrom(src v1.DataModelInterface) error {
	app, ok := src.(*datamodel.Application_v20250801preview)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(app.ID)
	dst.Name = to.Ptr(app.Name)
	dst.Type = to.Ptr(app.Type)
	dst.SystemData = fromSystemDataModel(&app.SystemData)
	dst.Location = to.Ptr(app.Location)
	dst.Tags = *to.StringMapPtr(app.Tags)
	dst.Properties = &ApplicationProperties{
		ProvisioningState: fromProvisioningStateDataModel(app.InternalMetadata.AsyncProvisioningState),
		Environment:       to.Ptr(app.Properties.Environment),
		Status:            &ResourceStatus{},
	}

	return nil
}
