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

// ConvertTo converts from the versioned TerraformConfig resource to version-agnostic datamodel.
func (src *TerraformConfigResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.TerraformConfig{
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
		Properties: datamodel.TerraformConfigResourceProperties{},
	}

	if src.Properties.Authentication != nil {
		converted.Properties.Authentication = toTerraformAuthConfigDataModel(src.Properties.Authentication)
	}

	if src.Properties.Providers != nil {
		converted.Properties.Providers = toTerraformProvidersDataModel(src.Properties.Providers)
	}

	if src.Properties.Env != nil {
		converted.Properties.Env = datamodel.EnvironmentVariables{
			AdditionalProperties: to.StringMap(src.Properties.Env),
		}
	}

	if src.Properties.EnvSecrets != nil {
		converted.Properties.EnvSecrets = toTerraformSecretReferencesDataModel(src.Properties.EnvSecrets)
	}

	if src.Properties.ReferencedBy != nil {
		converted.Properties.ReferencedBy = to.StringArray(src.Properties.ReferencedBy)
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned TerraformConfig resource.
func (dst *TerraformConfigResource) ConvertFrom(src v1.DataModelInterface) error {
	tc, ok := src.(*datamodel.TerraformConfig)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = &tc.ID
	dst.Name = &tc.Name
	dst.Type = &tc.Type
	dst.SystemData = fromSystemDataModel(&tc.SystemData)
	dst.Location = &tc.Location
	dst.Tags = *to.StringMapPtr(tc.Tags)
	dst.Properties = &TerraformConfigProperties{
		ProvisioningState: fromProvisioningStateDataModel(tc.InternalMetadata.AsyncProvisioningState),
	}

	if tc.Properties.Authentication.Git.PAT != nil {
		dst.Properties.Authentication = fromTerraformAuthConfigDataModel(tc.Properties.Authentication)
	}

	if tc.Properties.Providers != nil {
		dst.Properties.Providers = fromTerraformProvidersDataModel(tc.Properties.Providers)
	}

	if tc.Properties.Env.AdditionalProperties != nil {
		dst.Properties.Env = *to.StringMapPtr(tc.Properties.Env.AdditionalProperties)
	}

	if tc.Properties.EnvSecrets != nil {
		dst.Properties.EnvSecrets = fromTerraformSecretReferencesDataModel(tc.Properties.EnvSecrets)
	}

	if len(tc.Properties.ReferencedBy) > 0 {
		dst.Properties.ReferencedBy = to.ArrayofStringPtrs(tc.Properties.ReferencedBy)
	}

	return nil
}

func toTerraformAuthConfigDataModel(src *TerraformAuthConfig) datamodel.AuthConfig {
	result := datamodel.AuthConfig{}
	if src.Git != nil && src.Git.Pat != nil {
		result.Git = datamodel.GitAuthConfig{
			PAT: make(map[string]datamodel.SecretConfig),
		}
		for host, cfg := range src.Git.Pat {
			result.Git.PAT[host] = datamodel.SecretConfig{
				Secret: to.String(cfg.Secret),
			}
		}
	}
	return result
}

func fromTerraformAuthConfigDataModel(src datamodel.AuthConfig) *TerraformAuthConfig {
	result := &TerraformAuthConfig{}
	if len(src.Git.PAT) > 0 {
		result.Git = &TerraformGitAuthConfig{
			Pat: make(map[string]*TerraformSecretConfig),
		}
		for host, cfg := range src.Git.PAT {
			s := cfg.Secret
			result.Git.Pat[host] = &TerraformSecretConfig{
				Secret: &s,
			}
		}
	}
	return result
}

func toTerraformProvidersDataModel(src map[string][]*TerraformProviderConfigProperties) map[string][]datamodel.ProviderConfigProperties {
	result := make(map[string][]datamodel.ProviderConfigProperties)
	for name, configs := range src {
		var dmConfigs []datamodel.ProviderConfigProperties
		for _, cfg := range configs {
			if cfg == nil {
				continue
			}
			dmCfg := datamodel.ProviderConfigProperties{
				AdditionalProperties: cfg.AdditionalProperties,
			}
			if cfg.Secrets != nil {
				dmCfg.Secrets = make(map[string]datamodel.SecretReference)
				for k, v := range cfg.Secrets {
					if v != nil {
						dmCfg.Secrets[k] = datamodel.SecretReference{
							Source: to.String(v.Source),
							Key:    to.String(v.Key),
						}
					}
				}
			}
			dmConfigs = append(dmConfigs, dmCfg)
		}
		result[name] = dmConfigs
	}
	return result
}

func fromTerraformProvidersDataModel(src map[string][]datamodel.ProviderConfigProperties) map[string][]*TerraformProviderConfigProperties {
	result := make(map[string][]*TerraformProviderConfigProperties)
	for name, configs := range src {
		var apiConfigs []*TerraformProviderConfigProperties
		for _, cfg := range configs {
			apiCfg := &TerraformProviderConfigProperties{
				AdditionalProperties: cfg.AdditionalProperties,
			}
			if cfg.Secrets != nil {
				apiCfg.Secrets = make(map[string]*TerraformSecretReference)
				for k, v := range cfg.Secrets {
					s := v.Source
					key := v.Key
					apiCfg.Secrets[k] = &TerraformSecretReference{
						Source: &s,
						Key:    &key,
					}
				}
			}
			apiConfigs = append(apiConfigs, apiCfg)
		}
		result[name] = apiConfigs
	}
	return result
}

func toTerraformSecretReferencesDataModel(src map[string]*TerraformSecretReference) map[string]datamodel.SecretReference {
	result := make(map[string]datamodel.SecretReference)
	for k, v := range src {
		if v != nil {
			result[k] = datamodel.SecretReference{
				Source: to.String(v.Source),
				Key:    to.String(v.Key),
			}
		}
	}
	return result
}

func fromTerraformSecretReferencesDataModel(src map[string]datamodel.SecretReference) map[string]*TerraformSecretReference {
	result := make(map[string]*TerraformSecretReference)
	for k, v := range src {
		s := v.Source
		key := v.Key
		result[k] = &TerraformSecretReference{
			Source: &s,
			Key:    &key,
		}
	}
	return result
}
