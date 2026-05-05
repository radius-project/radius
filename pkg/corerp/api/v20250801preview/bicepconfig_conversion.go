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

	if len(src.Properties.RegistryAuthentications) > 0 {
		converted.Properties.RegistryAuthentications = make(map[string]datamodel.BicepRegistryAuthentication, len(src.Properties.RegistryAuthentications))
		for host, auth := range src.Properties.RegistryAuthentications {
			if auth == nil {
				continue
			}
			converted.Properties.RegistryAuthentications[host] = toBicepRegistryAuthDataModel(auth)
		}
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

	if len(bc.Properties.RegistryAuthentications) > 0 {
		dst.Properties.RegistryAuthentications = make(map[string]*BicepRegistryAuthentication, len(bc.Properties.RegistryAuthentications))
		for host, auth := range bc.Properties.RegistryAuthentications {
			authCopy := auth
			dst.Properties.RegistryAuthentications[host] = fromBicepRegistryAuthDataModel(&authCopy)
		}
	}

	if len(bc.Properties.ReferencedBy) > 0 {
		dst.Properties.ReferencedBy = to.ArrayofStringPtrs(bc.Properties.ReferencedBy)
	}

	return nil
}

func toBicepRegistryAuthDataModel(src *BicepRegistryAuthentication) datamodel.BicepRegistryAuthentication {
	result := datamodel.BicepRegistryAuthentication{
		BasicAuthSecretId: to.String(src.BasicAuthSecretID),
		AzureWiClientId:   to.String(src.AzureWiClientID),
		AzureWiTenantId:   to.String(src.AzureWiTenantID),
		AwsIamRoleArn:     to.String(src.AwsIamRoleArn),
	}
	if src.AuthenticationMethod != nil {
		result.AuthenticationMethod = string(*src.AuthenticationMethod)
	}
	return result
}

func fromBicepRegistryAuthDataModel(src *datamodel.BicepRegistryAuthentication) *BicepRegistryAuthentication {
	result := &BicepRegistryAuthentication{}
	if src.AuthenticationMethod != "" {
		method := BicepAuthenticationMethod(src.AuthenticationMethod)
		result.AuthenticationMethod = &method
	}
	if src.BasicAuthSecretId != "" {
		result.BasicAuthSecretID = &src.BasicAuthSecretId
	}
	if src.AzureWiClientId != "" {
		result.AzureWiClientID = &src.AzureWiClientId
	}
	if src.AzureWiTenantId != "" {
		result.AzureWiTenantID = &src.AzureWiTenantId
	}
	if src.AwsIamRoleArn != "" {
		result.AwsIamRoleArn = &src.AwsIamRoleArn
	}
	return result
}
