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

// ConvertTo converts from the versioned BicepSettingsResource to version-agnostic datamodel.
func (src *BicepSettingsResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.BicepSettings_v20250801preview{
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
		Properties: datamodel.BicepSettingsProperties_v20250801preview{},
	}

	// Convert Authentication
	if src.Properties.Authentication != nil {
		converted.Properties.Authentication = toBicepAuthenticationConfigurationDataModel(src.Properties.Authentication)
	}

	// Convert ReferencedBy
	if src.Properties.ReferencedBy != nil {
		converted.Properties.ReferencedBy = to.StringArray(src.Properties.ReferencedBy)
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned BicepSettingsResource.
func (dst *BicepSettingsResource) ConvertFrom(src v1.DataModelInterface) error {
	bs, ok := src.(*datamodel.BicepSettings_v20250801preview)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(bs.ID)
	dst.Name = to.Ptr(bs.Name)
	dst.Type = to.Ptr(bs.Type)
	dst.SystemData = fromSystemDataModel(&bs.SystemData)
	dst.Location = to.Ptr(bs.Location)
	dst.Tags = *to.StringMapPtr(bs.Tags)
	dst.Properties = &BicepSettingsProperties{
		ProvisioningState: fromProvisioningStateDataModel(bs.InternalMetadata.AsyncProvisioningState),
	}

	// Convert Authentication
	if bs.Properties.Authentication != nil {
		dst.Properties.Authentication = fromBicepAuthenticationConfigurationDataModel(bs.Properties.Authentication)
	}

	// Convert ReferencedBy
	if len(bs.Properties.ReferencedBy) > 0 {
		dst.Properties.ReferencedBy = to.ArrayofStringPtrs(bs.Properties.ReferencedBy)
	}

	return nil
}

func toBicepAuthenticationConfigurationDataModel(src *BicepAuthenticationConfiguration) *datamodel.BicepAuthenticationConfiguration {
	if src == nil {
		return nil
	}

	result := &datamodel.BicepAuthenticationConfiguration{}

	if src.Registries != nil {
		result.Registries = make(map[string]*datamodel.BicepRegistryAuthentication)
		for k, v := range src.Registries {
			if v != nil {
				result.Registries[k] = toBicepRegistryAuthenticationDataModel(v)
			}
		}
	}

	return result
}

func toBicepRegistryAuthenticationDataModel(src *BicepRegistryAuthentication) *datamodel.BicepRegistryAuthentication {
	if src == nil {
		return nil
	}

	result := &datamodel.BicepRegistryAuthentication{}

	if src.Basic != nil {
		result.Basic = &datamodel.BicepBasicAuthentication{
			Username: to.String(src.Basic.Username),
		}
		if src.Basic.Password != nil {
			result.Basic.Password = &datamodel.SecretRef{
				SecretID: to.String(src.Basic.Password.SecretID),
				Key:      to.String(src.Basic.Password.Key),
			}
		}
	}

	if src.AzureWorkloadIdentity != nil {
		result.AzureWorkloadIdentity = &datamodel.BicepAzureWorkloadIdentityAuthentication{
			ClientID: to.String(src.AzureWorkloadIdentity.ClientID),
			TenantID: to.String(src.AzureWorkloadIdentity.TenantID),
		}
		if src.AzureWorkloadIdentity.Token != nil {
			result.AzureWorkloadIdentity.Token = &datamodel.SecretRef{
				SecretID: to.String(src.AzureWorkloadIdentity.Token.SecretID),
				Key:      to.String(src.AzureWorkloadIdentity.Token.Key),
			}
		}
	}

	if src.AwsIrsa != nil {
		result.AwsIrsa = &datamodel.BicepAwsIrsaAuthentication{
			RoleArn: to.String(src.AwsIrsa.RoleArn),
		}
		if src.AwsIrsa.Token != nil {
			result.AwsIrsa.Token = &datamodel.SecretRef{
				SecretID: to.String(src.AwsIrsa.Token.SecretID),
				Key:      to.String(src.AwsIrsa.Token.Key),
			}
		}
	}

	return result
}

func fromBicepAuthenticationConfigurationDataModel(src *datamodel.BicepAuthenticationConfiguration) *BicepAuthenticationConfiguration {
	if src == nil {
		return nil
	}

	result := &BicepAuthenticationConfiguration{}

	if src.Registries != nil {
		result.Registries = make(map[string]*BicepRegistryAuthentication)
		for k, v := range src.Registries {
			if v != nil {
				result.Registries[k] = fromBicepRegistryAuthenticationDataModel(v)
			}
		}
	}

	return result
}

func fromBicepRegistryAuthenticationDataModel(src *datamodel.BicepRegistryAuthentication) *BicepRegistryAuthentication {
	if src == nil {
		return nil
	}

	result := &BicepRegistryAuthentication{}

	if src.Basic != nil {
		result.Basic = &BicepBasicAuthentication{
			Username: to.Ptr(src.Basic.Username),
		}
		if src.Basic.Password != nil {
			result.Basic.Password = &SecretReference{
				SecretID: to.Ptr(src.Basic.Password.SecretID),
				Key:      to.Ptr(src.Basic.Password.Key),
			}
		}
	}

	if src.AzureWorkloadIdentity != nil {
		result.AzureWorkloadIdentity = &BicepAzureWorkloadIdentityAuthentication{
			ClientID: to.Ptr(src.AzureWorkloadIdentity.ClientID),
			TenantID: to.Ptr(src.AzureWorkloadIdentity.TenantID),
		}
		if src.AzureWorkloadIdentity.Token != nil {
			result.AzureWorkloadIdentity.Token = &SecretReference{
				SecretID: to.Ptr(src.AzureWorkloadIdentity.Token.SecretID),
				Key:      to.Ptr(src.AzureWorkloadIdentity.Token.Key),
			}
		}
	}

	if src.AwsIrsa != nil {
		result.AwsIrsa = &BicepAwsIrsaAuthentication{
			RoleArn: to.Ptr(src.AwsIrsa.RoleArn),
		}
		if src.AwsIrsa.Token != nil {
			result.AwsIrsa.Token = &SecretReference{
				SecretID: to.Ptr(src.AwsIrsa.Token.SecretID),
				Key:      to.Ptr(src.AwsIrsa.Token.Key),
			}
		}
	}

	return result
}
