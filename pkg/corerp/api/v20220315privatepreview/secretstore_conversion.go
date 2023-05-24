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

package v20220315privatepreview

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
)

// ConvertTo converts from the versioned SecretStoreResource resource to version-agnostic datamodel.
func (src *SecretStoreResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.SecretStore{
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
		Properties: &datamodel.SecretStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: to.String(src.Properties.Application),
			},
			Resource: to.String(src.Properties.Resource),
			Type:     toSecretStoreDataTypeDataModel(src.Properties.Type),
			Data:     toSecretValuePropertiesDataModel(src.Properties.Data),
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned SecretStoreResource resource.
func (dst *SecretStoreResource) ConvertFrom(src v1.DataModelInterface) error {
	ss, ok := src.(*datamodel.SecretStore)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(ss.ID)
	dst.Name = to.Ptr(ss.Name)
	dst.Type = to.Ptr(ss.Type)
	dst.SystemData = fromSystemDataModel(ss.SystemData)
	dst.Location = to.Ptr(ss.Location)
	dst.Tags = *to.StringMapPtr(ss.Tags)
	dst.Properties = &SecretStoreProperties{
		Status: &ResourceStatus{
			OutputResources: rpv1.BuildExternalOutputResources(ss.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(ss.InternalMetadata.AsyncProvisioningState),
		Application:       to.Ptr(ss.Properties.Application),
		Type:              fromSecretStoreDataTypeDataModel(ss.Properties.Type),
		Resource:          to.Ptr(ss.Properties.Resource),
		Data:              fromSecretStoreDataPropertiesDataModel(ss.Properties.Data),
	}

	return nil
}

// ConvertTo does no-op because SecretStoresClientListSecretsResponse model is used only for response.
func (src *SecretStoresClientListSecretsResponse) ConvertTo() (v1.DataModelInterface, error) {
	return nil, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned SecretStoresClientListSecretsResponse resource.
func (dst *SecretStoresClientListSecretsResponse) ConvertFrom(src v1.DataModelInterface) error {
	ss, ok := src.(*datamodel.SecretStoreListSecrets)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.Type = fromSecretStoreDataTypeDataModel(ss.Type)
	dst.Data = fromSecretStoreDataPropertiesDataModel(ss.Data)

	return nil
}

func toSecretStoreDataTypeDataModel(src *SecretStoreDataType) datamodel.SecretType {
	if src == nil {
		return datamodel.SecretTypeGeneric
	}

	switch *src {
	case SecretStoreDataTypeGeneric:
		return datamodel.SecretTypeGeneric
	case SecretStoreDataTypeCertificate:
		return datamodel.SecretTypeCert
	}

	return datamodel.SecretTypeGeneric
}

func fromSecretStoreDataTypeDataModel(src datamodel.SecretType) *SecretStoreDataType {
	switch src {
	case datamodel.SecretTypeGeneric:
		return to.Ptr(SecretStoreDataTypeGeneric)
	case datamodel.SecretTypeCert:
		return to.Ptr(SecretStoreDataTypeCertificate)
	}
	return nil
}

func toSecretValuePropertiesDataModel(src map[string]*SecretValueProperties) map[string]*datamodel.SecretStoreDataValue {
	if src == nil {
		return nil
	}

	dst := map[string]*datamodel.SecretStoreDataValue{}
	for k, v := range src {
		dst[k] = &datamodel.SecretStoreDataValue{}
		if to.String(v.Value) != "" {
			dst[k].Value = v.Value
		}

		if v.Encoding != nil {
			switch *v.Encoding {
			case SecretValueEncodingRaw:
				dst[k].Encoding = datamodel.SecretValueEncodingRaw
			case SecretValueEncodingBase64:
				dst[k].Encoding = datamodel.SecretValueEncodingBase64
			}
		}

		if v.ValueFrom != nil {
			dst[k].ValueFrom = &datamodel.SecretStoreDataValueFrom{
				Name:    to.String(v.ValueFrom.Name),
				Version: to.String(v.ValueFrom.Version),
			}
		}
	}
	return dst
}

func fromSecretStoreDataPropertiesDataModel(src map[string]*datamodel.SecretStoreDataValue) map[string]*SecretValueProperties {
	if src == nil {
		return nil
	}

	dst := map[string]*SecretValueProperties{}
	for k, v := range src {
		dst[k] = &SecretValueProperties{}

		switch v.Encoding {
		case datamodel.SecretValueEncodingRaw:
			dst[k].Encoding = to.Ptr(SecretValueEncodingRaw)
		case datamodel.SecretValueEncodingBase64:
			dst[k].Encoding = to.Ptr(SecretValueEncodingBase64)
		}

		if v.ValueFrom != nil {
			dst[k].ValueFrom = &ValueFromProperties{
				Name:    to.Ptr(v.ValueFrom.Name),
				Version: to.Ptr(v.ValueFrom.Version),
			}
		}
	}
	return dst
}
