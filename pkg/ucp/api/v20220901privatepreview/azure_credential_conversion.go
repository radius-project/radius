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

package v20220901privatepreview

import (
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

const (
	// AzureCredentialType represents the ucp azure crendetial type value.
	AzureCredentialType = "System.Azure/credentials"
)

// ConvertTo converts from the versioned Credential resource to version-agnostic datamodel.
func (cr *AzureCredentialResource) ConvertTo() (v1.DataModelInterface, error) {
	prop, err := cr.getDataModelCredentialProperties()
	if err != nil {
		return nil, err
	}

	converted := &datamodel.AzureCredential{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(cr.ID),
				Name:     to.String(cr.Name),
				Type:     to.String(cr.Type),
				Location: to.String(cr.Location),
				Tags:     to.StringMap(cr.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		},
		Properties: prop,
	}

	return converted, nil
}

func (cr *AzureCredentialResource) getDataModelCredentialProperties() (*datamodel.AzureCredentialResourceProperties, error) {
	if cr.Properties == nil {
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties", ValidValue: "not nil"}
	}

	switch p := cr.Properties.(type) {
	case *AzureServicePrincipalProperties:
		var storage *datamodel.CredentialStorageProperties

		switch c := p.Storage.(type) {
		case *InternalCredentialStorageProperties:
			if c.Kind == nil {
				return nil, &v1.ErrModelConversion{PropertyName: "$.properties", ValidValue: "not nil"}
			}
			storage = &datamodel.CredentialStorageProperties{
				Kind: datamodel.InternalStorageKind,
				InternalCredential: &datamodel.InternalCredentialStorageProperties{
					SecretName: to.String(c.SecretName),
				},
			}
		case nil:
			return nil, &v1.ErrModelConversion{PropertyName: "$.properties.storage", ValidValue: "not nil"}
		default:
			return nil, &v1.ErrModelConversion{PropertyName: "$.properties.storage.kind", ValidValue: fmt.Sprintf("one of %q", PossibleCredentialStorageKindValues())}
		}

		return &datamodel.AzureCredentialResourceProperties{
			Kind: datamodel.AzureCredentialKind,
			AzureCredential: &datamodel.AzureCredentialProperties{
				TenantID:     to.String(p.TenantID),
				ClientID:     to.String(p.ClientID),
				ClientSecret: to.String(p.ClientSecret),
			},
			Storage: storage,
		}, nil
	default:
		return nil, v1.ErrInvalidModelConversion
	}
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Credential resource.
func (dst *AzureCredentialResource) ConvertFrom(src v1.DataModelInterface) error {
	dm, ok := src.(*datamodel.AzureCredential)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = &dm.ID
	dst.Name = &dm.Name
	dst.Type = &dm.Type
	dst.Location = &dm.Location
	dst.Tags = *to.StringMapPtr(dm.Tags)

	var storage CredentialStoragePropertiesClassification
	switch dm.Properties.Storage.Kind {
	case datamodel.InternalStorageKind:
		storage = &InternalCredentialStorageProperties{
			Kind:       to.Ptr(string(CredentialStorageKindInternal)),
			SecretName: to.Ptr(dm.Properties.Storage.InternalCredential.SecretName),
		}
	default:
		return v1.ErrInvalidModelConversion
	}

	// DO NOT convert any secret values to versioned model.
	switch dm.Properties.Kind {
	case datamodel.AzureCredentialKind:
		dst.Properties = &AzureServicePrincipalProperties{
			Kind:     to.Ptr(dm.Properties.Kind),
			ClientID: to.Ptr(dm.Properties.AzureCredential.ClientID),
			TenantID: to.Ptr(dm.Properties.AzureCredential.TenantID),
			Storage:  storage,
		}
	default:
		return v1.ErrInvalidModelConversion
	}

	return nil
}
