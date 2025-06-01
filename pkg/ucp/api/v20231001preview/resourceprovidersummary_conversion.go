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

package v20231001preview

import (
	"errors"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

// ConvertTo converts from the versioned ResourceProviderSummary resource to version-agnostic datamodel.
//
// NOTE: ResourceProviderSummary is READONLY. There is no conversion from versioned to datamodel.
func (src *ResourceProviderSummary) ConvertTo() (v1.DataModelInterface, error) {
	return nil, errors.New("the ResourceProviderSummary is READONLY. There is no conversion from versioned to datamodel")
}

// ConvertFrom converts from version-agnostic datamodel to the versioned ResourceProviderSummary resource.
func (dst *ResourceProviderSummary) ConvertFrom(src v1.DataModelInterface) error {
	dm, ok := src.(*datamodel.ResourceProviderSummary)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.Name = to.Ptr(dm.Name)

	dst.Locations = map[string]map[string]any{}
	for locationName := range dm.Properties.Locations {
		dst.Locations[locationName] = map[string]any{}
	}

	dst.ResourceTypes = map[string]*ResourceProviderSummaryResourceType{}
	for resourceTypeName, resourceType := range dm.Properties.ResourceTypes {
		// Populate API versions and associated schema details for each resource type.
		apiVersions := map[string]*ResourceTypeSummaryResultAPIVersion{}
		for k, v := range resourceType.APIVersions {
			apiVersions[k] = &ResourceTypeSummaryResultAPIVersion{
				Schema: v.Schema,
			}
		}

		dst.ResourceTypes[resourceTypeName] = &ResourceProviderSummaryResourceType{
			Capabilities:      to.SliceOfPtrs(resourceType.Capabilities...),
			DefaultAPIVersion: resourceType.DefaultAPIVersion,
			APIVersions:       apiVersions,
			Description:       resourceType.Description,
		}
	}

	return nil
}
