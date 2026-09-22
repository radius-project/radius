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
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/to"
)

// ConvertTo converts from the versioned TerraformSettings resource to version-agnostic datamodel.
func (src *TerraformSettingsResource) ConvertTo() (v1.DataModelInterface, error) {
	properties := src.Properties
	if properties == nil {
		properties = &TerraformSettingsProperties{}
	}
	converted := &datamodel.TerraformSettings{
		ID:                     to.String(src.ID),
		Name:                   to.String(src.Name),
		Type:                   to.String(src.Type),
		Location:               to.String(src.Location),
		Tags:                   to.StringMap(src.Tags),
		CreatedAPIVersion:      Version,
		UpdatedAPIVersion:      Version,
		AsyncProvisioningState: toProvisioningStateDataModel(properties.ProvisioningState),
		Properties:             datamodel.TerraformSettingsResourceProperties{},
	}

	if properties.Terraformrc != nil {
		converted.Properties.Terraformrc = toTerraformrcDataModel(properties.Terraformrc)
	}

	backend, err := toTerraformBackendDataModel(properties.Backend)
	if err != nil {
		return nil, v1.NewClientErrInvalidRequest(err.Error())
	}
	converted.Properties.Backend = backend

	if properties.Env != nil {
		converted.Properties.Env = to.StringMap(properties.Env)
	}

	if properties.ReferencedBy != nil {
		converted.Properties.ReferencedBy = to.StringArray(properties.ReferencedBy)
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned TerraformSettings resource.
func (dst *TerraformSettingsResource) ConvertFrom(src v1.DataModelInterface) error {
	tc, ok := src.(*datamodel.TerraformSettings)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = &tc.ID
	dst.Name = &tc.Name
	dst.Type = &tc.Type
	dst.SystemData = fromSystemDataModel(&tc.SystemData)
	dst.Location = &tc.Location
	dst.Tags = *to.StringMapPtr(tc.Tags)
	dst.Properties = &TerraformSettingsProperties{
		ProvisioningState: fromProvisioningStateDataModel(tc.InternalMetadata.AsyncProvisioningState),
	}

	dst.Properties.Terraformrc = fromTerraformrcDataModel(tc.Properties.Terraformrc)
	dst.Properties.Backend = fromTerraformBackendDataModel(tc.Properties.Backend)

	if tc.Properties.Env != nil {
		dst.Properties.Env = *to.StringMapPtr(tc.Properties.Env)
	}

	if len(tc.Properties.ReferencedBy) > 0 {
		dst.Properties.ReferencedBy = to.ArrayofStringPtrs(tc.Properties.ReferencedBy)
	}

	return nil
}

func toTerraformBackendDataModel(src TerraformBackendClassification) (*datamodel.TerraformBackend, error) {
	if src == nil {
		return nil, nil
	}
	var backend *datamodel.TerraformBackend
	switch b := src.(type) {
	case *TerraformS3Backend:
		backend = &datamodel.TerraformBackend{
			Type: "s3", Bucket: to.String(b.Bucket), Region: to.String(b.Region), KeyPrefix: b.KeyPrefix,
		}
	case *TerraformAzureRMBackend:
		backend = &datamodel.TerraformBackend{
			Type: "azurerm", StorageAccountName: to.String(b.StorageAccountName),
			ContainerName: to.String(b.ContainerName), KeyPrefix: b.KeyPrefix,
		}
	default:
		return nil, fmt.Errorf("backend.type must be s3 or azurerm")
	}
	if err := backend.Validate(); err != nil {
		return nil, err
	}
	return backend, nil
}

func fromTerraformBackendDataModel(b *datamodel.TerraformBackend) TerraformBackendClassification {
	if b == nil {
		return nil
	}
	switch b.Type {
	case "s3":
		return &TerraformS3Backend{Type: &b.Type, Bucket: &b.Bucket, Region: &b.Region, KeyPrefix: b.KeyPrefix}
	case "azurerm":
		return &TerraformAzureRMBackend{Type: &b.Type, StorageAccountName: &b.StorageAccountName, ContainerName: &b.ContainerName, KeyPrefix: b.KeyPrefix}
	default:
		return &TerraformBackend{Type: &b.Type, KeyPrefix: b.KeyPrefix}
	}
}

func toTerraformrcDataModel(src *TerraformrcConfig) datamodel.TerraformrcConfig {
	result := datamodel.TerraformrcConfig{}

	if src.ProviderInstallation != nil {
		result.ProviderInstallation = &datamodel.TerraformProviderInstallation{}
		if src.ProviderInstallation.NetworkMirror != nil {
			result.ProviderInstallation.NetworkMirror = &datamodel.TerraformProviderMirror{
				URL:     to.String(src.ProviderInstallation.NetworkMirror.URL),
				Include: to.StringArray(src.ProviderInstallation.NetworkMirror.Include),
				Exclude: to.StringArray(src.ProviderInstallation.NetworkMirror.Exclude),
			}
		}
		if src.ProviderInstallation.Direct != nil {
			result.ProviderInstallation.Direct = &datamodel.TerraformProviderDirect{
				Include: to.StringArray(src.ProviderInstallation.Direct.Include),
				Exclude: to.StringArray(src.ProviderInstallation.Direct.Exclude),
			}
		}
	}

	if src.Credentials != nil {
		result.Credentials = make(map[string]datamodel.TerraformCredentialConfig)
		for host, cfg := range src.Credentials {
			if cfg != nil {
				result.Credentials[host] = datamodel.TerraformCredentialConfig{
					Secret: to.String(cfg.Secret),
				}
			}
		}
	}

	return result
}

func fromTerraformrcDataModel(src datamodel.TerraformrcConfig) *TerraformrcConfig {
	result := &TerraformrcConfig{}

	if src.ProviderInstallation != nil {
		result.ProviderInstallation = &TerraformProviderInstallation{}
		if src.ProviderInstallation.NetworkMirror != nil {
			result.ProviderInstallation.NetworkMirror = &TerraformProviderMirror{
				URL:     &src.ProviderInstallation.NetworkMirror.URL,
				Include: to.ArrayofStringPtrs(src.ProviderInstallation.NetworkMirror.Include),
				Exclude: to.ArrayofStringPtrs(src.ProviderInstallation.NetworkMirror.Exclude),
			}
		}
		if src.ProviderInstallation.Direct != nil {
			result.ProviderInstallation.Direct = &TerraformProviderDirect{
				Include: to.ArrayofStringPtrs(src.ProviderInstallation.Direct.Include),
				Exclude: to.ArrayofStringPtrs(src.ProviderInstallation.Direct.Exclude),
			}
		}
	}

	if len(src.Credentials) > 0 {
		result.Credentials = make(map[string]*TerraformCredentialConfig)
		for host, cfg := range src.Credentials {
			s := cfg.Secret
			result.Credentials[host] = &TerraformCredentialConfig{
				Secret: &s,
			}
		}
	}

	return result
}
