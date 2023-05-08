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

package v20220901privatepreview

import (
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

// ConvertTo converts from the versioned Plane resource to version-agnostic datamodel.
func (src *PlaneResource) ConvertTo() (v1.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.

	if src.Properties.Kind == nil {
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties.kind", ValidValue: "not nil"}
	}

	var found bool
	for _, k := range PossiblePlaneKindValues() {
		if *src.Properties.Kind == k {
			found = true
			break
		}
	}
	if !found {
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties.kind", ValidValue: fmt.Sprintf("one of %s", PossiblePlaneKindValues())}
	}

	// Plane validation
	if *src.Properties.Kind == PlaneKindUCPNative && (src.Properties.ResourceProviders == nil || len(src.Properties.ResourceProviders) == 0) {
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties.resourceProviders", ValidValue: "at least one provided"}
	} else if *src.Properties.Kind == PlaneKindAzure && (src.Properties.URL == nil || *src.Properties.URL == "") {
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties.URL", ValidValue: "non-empty string"}
	}
	// No validation for AWS plane.

	converted := &datamodel.Plane{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   to.String(src.ID),
				Name: to.String(src.Name),
				Type: to.String(src.Type),
			},
		},
		Properties: datamodel.PlaneProperties{
			Kind:              datamodel.PlaneKind(*src.Properties.Kind),
			URL:               src.Properties.URL,
			ResourceProviders: src.Properties.ResourceProviders,
		},
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Plane resource.
func (dst *PlaneResource) ConvertFrom(src v1.DataModelInterface) error {
	plane, ok := src.(*datamodel.Plane)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = &plane.TrackedResource.ID
	dst.Name = &plane.TrackedResource.Name
	dst.Type = &plane.TrackedResource.Type

	dst.Properties = &PlaneResourceProperties{
		Kind:              (*PlaneKind)(&plane.Properties.Kind),
		URL:               plane.Properties.URL,
		ResourceProviders: plane.Properties.ResourceProviders,
	}

	return nil
}
