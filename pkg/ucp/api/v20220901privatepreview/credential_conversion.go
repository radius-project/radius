// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220901privatepreview

import (
	"fmt"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

const (
	// AzureCredentialType represents the ucp azure crendetial type value.
	AzureCredentialType = "System.Azure/credentials"
	// AWSCredentialType represents the ucp aws crendetial type value.
	AWSCredentialType = "System.AWS/credentials"
)

// ConvertTo converts from the versioned Credential resource to version-agnostic datamodel.
func (cr *CredentialResource) ConvertTo() (conv.DataModelInterface, error) {
	crendentialProperties, err := cr.getDataModelCredentialProperties()
	if err != nil {
		return nil, err
	}

	converted := &datamodel.Credential{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(cr.ID),
				Name:     to.String(cr.Name),
				Type:     to.String(cr.Type),
				Location: *cr.Location,
				Tags:     to.StringMap(cr.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		},

		Properties: crendentialProperties,
	}

	return converted, nil
}

func (cr *CredentialResource) getDataModelCredentialProperties() (*datamodel.CredentialResourceProperties, error) {
	if cr.Properties == nil {
		return nil, &conv.ErrModelConversion{PropertyName: "$.properties", ValidValue: "not nil"}
	}
	crendentialProperties := cr.Properties.GetCredentialResourceProperties()

	if crendentialProperties.Storage == nil {
		return nil, &conv.ErrModelConversion{PropertyName: "$.properties.storage", ValidValue: "not nil"}
	}

	storageProperties := crendentialProperties.Storage.GetCredentialStorageProperties()
	if storageProperties.GetCredentialStorageProperties().Kind == nil {
		return nil, &conv.ErrModelConversion{PropertyName: "$.properties.storage.kind", ValidValue: "not nil"}
	}
	// Check for storage type value
	var found bool
	for _, k := range PossibleCredentialStorageKindValues() {
		if *storageProperties.Kind == k {
			found = true
			break
		}
	}
	if !found {
		return nil, &conv.ErrModelConversion{PropertyName: "$.properties.storage.kind", ValidValue: fmt.Sprintf("one of %s", PossibleCredentialStorageKindValues())}
	}
	storage, err := cr.getCredentialStorageProperties()
	if err != nil {
		return nil, err
	}

	switch p := cr.Properties.(type) {
	case *AzureServicePrincipalProperties:
		return &datamodel.CredentialResourceProperties{
			Kind: to.String(p.Kind),
			AzureCredential: &datamodel.AzureCredentialProperties{
				TenantID: p.TenantID,
				ClientID: p.ClientID,
			},
			Storage: storage,
		}, nil
	case *AWSCredentialProperties:
		return &datamodel.CredentialResourceProperties{
			Kind: *p.Kind,
			AWSCredential: &datamodel.AWSCredentialProperties{
				AccessKeyID:     p.AccessKeyID,
				SecretAccessKey: p.SecretAccessKey,
			},
			Storage: storage,
		}, nil
	default:
		return nil, conv.ErrInvalidModelConversion
	}
}

func (cr *CredentialResource) getCredentialStorageProperties() (*datamodel.CredentialStorageProperties, error) {
	storageKind := datamodel.CredentialStorageKind(*cr.Properties.GetCredentialResourceProperties().Storage.GetCredentialStorageProperties().Kind)
	switch p := cr.Properties.GetCredentialResourceProperties().Storage.(type) {
	case *InternalCredentialStorageProperties:
		return &datamodel.CredentialStorageProperties{
			Kind: &storageKind,
			InternalCredential: &datamodel.InternalCredentialStorageProperties{
				SecretName: p.SecretName,
			},
		}, nil
	default:
		return &datamodel.CredentialStorageProperties{
			Kind: (*datamodel.CredentialStorageKind)(p.GetCredentialStorageProperties().Kind),
		}, nil
	}
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Credential resource.
func (dst *CredentialResource) ConvertFrom(src conv.DataModelInterface) error {
	credential, ok := src.(*datamodel.Credential)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = &credential.ID
	dst.Name = &credential.Name
	dst.Type = &credential.Type
	dst.Location = &credential.Location
	dst.Tags = *to.StringMapPtr(credential.Tags)

	switch *dst.Type {
	case AzureCredentialType:
		dst.Properties = &AzureServicePrincipalProperties{
			Kind:     &credential.Properties.Kind,
			ClientID: credential.Properties.AzureCredential.ClientID,
			TenantID: credential.Properties.AzureCredential.TenantID,
			Storage:  getStorage(credential),
		}
	case AWSCredentialType:
		dst.Properties = &AWSCredentialProperties{
			Kind:            &credential.Properties.Kind,
			SecretAccessKey: credential.Properties.AWSCredential.SecretAccessKey,
			AccessKeyID:     credential.Properties.AWSCredential.AccessKeyID,
			Storage:         getStorage(credential),
		}
	default:
		dst.Properties = &CredentialResourceProperties{
			Kind:    &credential.Properties.Kind,
			Storage: getStorage(credential),
		}
	}
	return nil
}

func getStorage(credential *datamodel.Credential) CredentialStoragePropertiesClassification {
	credentialStorageKind := CredentialStorageKind(*credential.Properties.Storage.Kind)
	switch *credential.Properties.Storage.Kind {
	case datamodel.InternalStorageKind:
		return &InternalCredentialStorageProperties{
			Kind:       &credentialStorageKind,
			SecretName: credential.Properties.Storage.InternalCredential.SecretName,
		}
	default:
		return nil
	}
}
