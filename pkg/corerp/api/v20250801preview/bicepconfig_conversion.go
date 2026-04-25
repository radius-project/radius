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
	"github.com/radius-project/radius/pkg/to"
)

// ConvertTo converts from the versioned BicepConfig resource to version-agnostic datamodel.
func (src *BicepConfigResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.BicepConfig{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				CreatedAPIVersion:      Version,
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			},
		},
		Properties: datamodel.BicepConfigResourceProperties{},
	}

	if src.Properties.Authentication != nil {
		converted.Properties.Authentication = toBicepAuthDataModel(src.Properties.Authentication)
	}

	if src.Properties.ReferencedBy != nil {
		converted.Properties.ReferencedBy = to.StringArray(src.Properties.ReferencedBy)
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned BicepConfig resource.
func (dst *BicepConfigResource) ConvertFrom(src v1.DataModelInterface) error {
	bc, ok := src.(*datamodel.BicepConfig)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = &bc.ID
	dst.Name = &bc.Name
	dst.Type = &bc.Type
	dst.SystemData = fromSystemDataModel(&bc.SystemData)
	dst.Location = &bc.Location
	dst.Tags = *to.StringMapPtr(bc.Tags)
	dst.Properties = &BicepConfigProperties{
		ProvisioningState: fromProvisioningStateDataModel(bc.InternalMetadata.AsyncProvisioningState),
	}

	if bc.Properties.Authentication != nil {
		dst.Properties.Authentication = fromBicepAuthDataModel(bc.Properties.Authentication)
	}

	if len(bc.Properties.ReferencedBy) > 0 {
		dst.Properties.ReferencedBy = to.ArrayofStringPtrs(bc.Properties.ReferencedBy)
	}

	return nil
}

func toBicepAuthDataModel(src map[string]*BicepRegistrySecretConfig) map[string]datamodel.RegistrySecretConfig {
	result := make(map[string]datamodel.RegistrySecretConfig)
	for host, cfg := range src {
		if cfg != nil {
			result[host] = datamodel.RegistrySecretConfig{
				Secret: to.String(cfg.Secret),
			}
		}
	}
	return result
}

func fromBicepAuthDataModel(src map[string]datamodel.RegistrySecretConfig) map[string]*BicepRegistrySecretConfig {
	result := make(map[string]*BicepRegistrySecretConfig)
	for host, cfg := range src {
		s := cfg.Secret
		result[host] = &BicepRegistrySecretConfig{
			Secret: &s,
		}
	}
	return result
}
