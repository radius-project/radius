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
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

// ConvertTo converts from the versioned ResourceProviderResource resource to version-agnostic datamodel.
func (src *ResourceProviderResource) ConvertTo() (v1.DataModelInterface, error) {
	dst := &datamodel.ResourceProvider{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
		},
	}

	// Note: we omit SystemData and Tags for this type. They cannot be specified by the user.

	dst.Properties = datamodel.ResourceProviderProperties{
		Locations: map[string]datamodel.ResourceProviderLocation{},
	}

	for name, location := range src.Properties.Locations {
		dst.Properties.Locations[name] = fromResourceProviderLocation(location)
	}

	for _, rt := range src.Properties.ResourceTypes {
		dst.Properties.ResourceTypes = append(dst.Properties.ResourceTypes, fromResourceType(rt))
	}

	return dst, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned ResourceProviderResource resource.
func (dst *ResourceProviderResource) ConvertFrom(src v1.DataModelInterface) error {
	dm, ok := src.(*datamodel.ResourceProvider)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(dm.ID)
	dst.Name = to.Ptr(dm.Name)
	dst.Type = to.Ptr(datamodel.ResourceProviderResourceType)
	dst.Location = to.Ptr(dm.Location)

	// Note: we omit SystemData and Tags for this type. They cannot be specified by the user.

	dst.Properties = &ResourceProviderProperties{
		ProvisioningState: to.Ptr(ProvisioningState(dm.InternalMetadata.AsyncProvisioningState)),
		Locations:         map[string]*ResourceProviderLocation{},
	}

	for name, location := range dm.Properties.Locations {
		dst.Properties.Locations[name] = toResourceProviderLocation(location)
	}

	for _, rt := range dm.Properties.ResourceTypes {
		dst.Properties.ResourceTypes = append(dst.Properties.ResourceTypes, toResourceType(rt))
	}

	return nil
}

func fromResourceType(rt *ResourceType) datamodel.ResourceType {
	dm := datamodel.ResourceType{
		ResourceType:      to.String(rt.ResourceType),
		DefaultAPIVersion: to.String(rt.DefaultAPIVersion),
		APIVersions:       map[string]datamodel.ResourceTypeAPIVersion{},
	}

	for name, apiVersion := range rt.APIVersions {
		dm.APIVersions[name] = fromResourceTypeAPIVersion(apiVersion)
	}

	for _, capability := range rt.Capabilities {
		dm.Capabilities = append(dm.Capabilities, to.String(capability))
	}

	for _, location := range rt.Locations {
		dm.Locations = append(dm.Locations, to.String(location))
	}

	return dm
}

func toResourceType(dm datamodel.ResourceType) *ResourceType {
	rt := &ResourceType{
		ResourceType:      to.Ptr(dm.ResourceType),
		APIVersions:       map[string]*ResourceTypeAPIVersion{},
		Capabilities:      to.SliceOfPtrs(dm.Capabilities...),
		DefaultAPIVersion: to.Ptr(dm.DefaultAPIVersion),
		Locations:         to.SliceOfPtrs(dm.Locations...),
	}

	for name, apiVersion := range dm.APIVersions {
		rt.APIVersions[name] = toResourceTypeAPIVersion(apiVersion)
	}

	return rt
}

func fromResourceProviderLocation(location *ResourceProviderLocation) datamodel.ResourceProviderLocation {
	return datamodel.ResourceProviderLocation{
		Address: to.String(location.Address),
	}
}

func toResourceProviderLocation(d datamodel.ResourceProviderLocation) *ResourceProviderLocation {
	return &ResourceProviderLocation{
		Address: to.Ptr(d.Address),
	}
}

func fromResourceTypeAPIVersion(version *ResourceTypeAPIVersion) datamodel.ResourceTypeAPIVersion {
	return datamodel.ResourceTypeAPIVersion{
		Schema: version.Schema,
	}
}

func toResourceTypeAPIVersion(d datamodel.ResourceTypeAPIVersion) *ResourceTypeAPIVersion {
	return &ResourceTypeAPIVersion{
		Schema: d.Schema,
	}
}
