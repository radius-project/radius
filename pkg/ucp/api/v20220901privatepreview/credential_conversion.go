// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
	// AWSCredentialType represents the ucp aws crendetial type value.
	AWSCredentialType = "System.AWS/credentials"
)

// ConvertTo converts from the versioned Credential resource to version-agnostic datamodel.
func (cr *CredentialResource) ConvertTo() (v1.DataModelInterface, error) {
	prop, err := cr.getDataModelCredentialProperties()
	if err != nil {
		return nil, err
	}

	converted := &datamodel.Credential{
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

func (cr *CredentialResource) getDataModelCredentialProperties() (*datamodel.CredentialResourceProperties, error) {
	if cr.Properties == nil {
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties", ValidValue: "not nil"}
	}

	var storage *datamodel.CredentialStorageProperties

	switch p := cr.Properties.GetCredentialResourceProperties().Storage.(type) {
	case *InternalCredentialStorageProperties:
		if p.Kind == nil {
			return nil, &v1.ErrModelConversion{PropertyName: "$.properties", ValidValue: "not nil"}
		}
		storage = &datamodel.CredentialStorageProperties{
			Kind: datamodel.InternalStorageKind,
			InternalCredential: &datamodel.InternalCredentialStorageProperties{
				SecretName: to.String(p.SecretName),
			},
		}
	case nil:
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties.storage", ValidValue: "not nil"}
	default:
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties.storage.kind", ValidValue: fmt.Sprintf("one of %q", PossibleCredentialStorageKindValues())}
	}

	switch p := cr.Properties.(type) {
	case *AzureServicePrincipalProperties:
		return &datamodel.CredentialResourceProperties{
			Kind: datamodel.AzureCredentialKind,
			AzureCredential: &datamodel.AzureCredentialProperties{
				TenantID:     to.String(p.TenantID),
				ClientID:     to.String(p.ClientID),
				ClientSecret: to.String(p.ClientSecret),
			},
			Storage: storage,
		}, nil
	case *AWSCredentialProperties:
		return &datamodel.CredentialResourceProperties{
			Kind: datamodel.AWSCredentialKind,
			AWSCredential: &datamodel.AWSCredentialProperties{
				AccessKeyID:     to.String(p.AccessKeyID),
				SecretAccessKey: to.String(p.SecretAccessKey),
			},
			Storage: storage,
		}, nil
	default:
		return nil, v1.ErrInvalidModelConversion
	}
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Credential resource.
func (dst *CredentialResource) ConvertFrom(src v1.DataModelInterface) error {
	dm, ok := src.(*datamodel.Credential)
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
			Kind:       to.Ptr(CredentialStorageKindInternal),
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
	case datamodel.AWSCredentialKind:
		dst.Properties = &AWSCredentialProperties{
			Kind:        to.Ptr(dm.Properties.Kind),
			AccessKeyID: to.Ptr(dm.Properties.AWSCredential.AccessKeyID),
			Storage:     storage,
		}
	default:
		return v1.ErrInvalidModelConversion
	}

	return nil
}
