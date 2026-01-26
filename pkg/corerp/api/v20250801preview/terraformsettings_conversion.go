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

// ConvertTo converts from the versioned TerraformSettingsResource to version-agnostic datamodel.
func (src *TerraformSettingsResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.TerraformSettings_v20250801preview{
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
		Properties: datamodel.TerraformSettingsProperties_v20250801preview{},
	}

	// Convert TerraformRC
	if src.Properties.Terraformrc != nil {
		converted.Properties.TerraformRC = toTerraformCliConfigurationDataModel(src.Properties.Terraformrc)
	}

	// Convert Backend
	if src.Properties.Backend != nil {
		converted.Properties.Backend = toTerraformBackendConfigurationDataModel(src.Properties.Backend)
	}

	// Convert Env
	if src.Properties.Env != nil {
		converted.Properties.Env = to.StringMap(src.Properties.Env)
	}

	// Convert Logging
	if src.Properties.Logging != nil {
		converted.Properties.Logging = toTerraformLoggingConfigurationDataModel(src.Properties.Logging)
	}

	// Convert ReferencedBy
	if src.Properties.ReferencedBy != nil {
		converted.Properties.ReferencedBy = to.StringArray(src.Properties.ReferencedBy)
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned TerraformSettingsResource.
func (dst *TerraformSettingsResource) ConvertFrom(src v1.DataModelInterface) error {
	ts, ok := src.(*datamodel.TerraformSettings_v20250801preview)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(ts.ID)
	dst.Name = to.Ptr(ts.Name)
	dst.Type = to.Ptr(ts.Type)
	dst.SystemData = fromSystemDataModel(&ts.SystemData)
	dst.Location = to.Ptr(ts.Location)
	dst.Tags = *to.StringMapPtr(ts.Tags)
	dst.Properties = &TerraformSettingsProperties{
		ProvisioningState: fromProvisioningStateDataModel(ts.InternalMetadata.AsyncProvisioningState),
	}

	// Convert TerraformRC
	if ts.Properties.TerraformRC != nil {
		dst.Properties.Terraformrc = fromTerraformCliConfigurationDataModel(ts.Properties.TerraformRC)
	}

	// Convert Backend
	if ts.Properties.Backend != nil {
		dst.Properties.Backend = fromTerraformBackendConfigurationDataModel(ts.Properties.Backend)
	}

	// Convert Env
	if len(ts.Properties.Env) > 0 {
		dst.Properties.Env = *to.StringMapPtr(ts.Properties.Env)
	}

	// Convert Logging
	if ts.Properties.Logging != nil {
		dst.Properties.Logging = fromTerraformLoggingConfigurationDataModel(ts.Properties.Logging)
	}

	// Convert ReferencedBy
	if len(ts.Properties.ReferencedBy) > 0 {
		dst.Properties.ReferencedBy = to.ArrayofStringPtrs(ts.Properties.ReferencedBy)
	}

	return nil
}

func toTerraformCliConfigurationDataModel(src *TerraformCliConfiguration) *datamodel.TerraformCliConfiguration {
	if src == nil {
		return nil
	}

	result := &datamodel.TerraformCliConfiguration{}

	// Convert ProviderInstallation
	if src.ProviderInstallation != nil {
		result.ProviderInstallation = &datamodel.TerraformProviderInstallationConfiguration{}

		if src.ProviderInstallation.NetworkMirror != nil {
			result.ProviderInstallation.NetworkMirror = &datamodel.TerraformNetworkMirrorConfiguration{
				URL:     to.String(src.ProviderInstallation.NetworkMirror.URL),
				Include: to.StringArray(src.ProviderInstallation.NetworkMirror.Include),
				Exclude: to.StringArray(src.ProviderInstallation.NetworkMirror.Exclude),
			}
		}

		if src.ProviderInstallation.Direct != nil {
			result.ProviderInstallation.Direct = &datamodel.TerraformDirectConfiguration{
				Include: to.StringArray(src.ProviderInstallation.Direct.Include),
				Exclude: to.StringArray(src.ProviderInstallation.Direct.Exclude),
			}
		}
	}

	// Convert Credentials
	if src.Credentials != nil {
		result.Credentials = make(map[string]*datamodel.TerraformCredentialConfiguration)
		for k, v := range src.Credentials {
			if v != nil {
				result.Credentials[k] = &datamodel.TerraformCredentialConfiguration{}
				if v.Token != nil {
					result.Credentials[k].Token = &datamodel.SecretRef{
						SecretID: to.String(v.Token.SecretID),
						Key:      to.String(v.Token.Key),
					}
				}
			}
		}
	}

	return result
}

func fromTerraformCliConfigurationDataModel(src *datamodel.TerraformCliConfiguration) *TerraformCliConfiguration {
	if src == nil {
		return nil
	}

	result := &TerraformCliConfiguration{}

	// Convert ProviderInstallation
	if src.ProviderInstallation != nil {
		result.ProviderInstallation = &TerraformProviderInstallationConfiguration{}

		if src.ProviderInstallation.NetworkMirror != nil {
			result.ProviderInstallation.NetworkMirror = &TerraformNetworkMirrorConfiguration{
				URL:     to.Ptr(src.ProviderInstallation.NetworkMirror.URL),
				Include: to.SliceOfPtrs(src.ProviderInstallation.NetworkMirror.Include...),
				Exclude: to.SliceOfPtrs(src.ProviderInstallation.NetworkMirror.Exclude...),
			}
		}

		if src.ProviderInstallation.Direct != nil {
			result.ProviderInstallation.Direct = &TerraformDirectConfiguration{
				Include: to.SliceOfPtrs(src.ProviderInstallation.Direct.Include...),
				Exclude: to.SliceOfPtrs(src.ProviderInstallation.Direct.Exclude...),
			}
		}
	}

	// Convert Credentials
	if src.Credentials != nil {
		result.Credentials = make(map[string]*TerraformCredentialConfiguration)
		for k, v := range src.Credentials {
			if v != nil {
				result.Credentials[k] = &TerraformCredentialConfiguration{}
				if v.Token != nil {
					result.Credentials[k].Token = &SecretReference{
						SecretID: to.Ptr(v.Token.SecretID),
						Key:      to.Ptr(v.Token.Key),
					}
				}
			}
		}
	}

	return result
}

func toTerraformBackendConfigurationDataModel(src *TerraformBackendConfiguration) *datamodel.TerraformBackendConfiguration {
	if src == nil {
		return nil
	}

	result := &datamodel.TerraformBackendConfiguration{
		Type: to.String(src.Type),
	}

	// Convert map[string]*string to map[string]string
	if src.Config != nil {
		result.Config = to.StringMap(src.Config)
	}

	return result
}

func fromTerraformBackendConfigurationDataModel(src *datamodel.TerraformBackendConfiguration) *TerraformBackendConfiguration {
	if src == nil {
		return nil
	}

	result := &TerraformBackendConfiguration{
		Type: to.Ptr(src.Type),
	}

	// Convert map[string]string to map[string]*string
	if src.Config != nil {
		result.Config = *to.StringMapPtr(src.Config)
	}

	return result
}

func toTerraformLoggingConfigurationDataModel(src *TerraformLoggingConfiguration) *datamodel.TerraformLoggingConfiguration {
	if src == nil {
		return nil
	}

	result := &datamodel.TerraformLoggingConfiguration{
		Path: to.String(src.Path),
	}

	if src.Level != nil {
		result.Level = datamodel.TerraformLogLevel(*src.Level)
	}

	return result
}

func fromTerraformLoggingConfigurationDataModel(src *datamodel.TerraformLoggingConfiguration) *TerraformLoggingConfiguration {
	if src == nil {
		return nil
	}

	result := &TerraformLoggingConfiguration{
		Path: to.Ptr(src.Path),
	}

	if src.Level != "" {
		level := TerraformLogLevel(src.Level)
		result.Level = &level
	}

	return result
}
