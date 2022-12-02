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
	AzureCredentialType = "System.Azure/credentials"
	AWSCredentialType   = "System.AWS/credentials"
)

// ConvertTo converts from the versioned Credential resource to version-agnostic datamodel.
func (cr *CredentialResource) ConvertTo() (conv.DataModelInterface, error) {
	crendentialProperties, err := cr.getDataModelCredentialProperties()
	if err != nil {
		return nil, err
	}

	converted := &datamodel.Credential{
		TrackedResource: v1.TrackedResource{
			ID:       to.String(cr.ID),
			Name:     to.String(cr.Name),
			Type:     to.String(cr.Type),
			Location: *cr.Location,
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

	if *cr.Type == AzureCredentialType {
		p, ok := cr.Properties.(*AzureServicePrincipalProperties)
		if !ok {
			return nil, conv.ErrInvalidModelConversion
		}
		return &datamodel.CredentialResourceProperties{
			Kind: *p.Kind,
			AzureCredential: &datamodel.AzureCredentialProperties{
				TenantID: p.TenantID,
				ClientID: p.ClientID,
			},
			Storage: storage,
		}, nil
	} else if *cr.Type == AWSCredentialType {
		p, ok := cr.Properties.(*AWSCredentialProperties)
		if !ok {
			return nil, conv.ErrInvalidModelConversion
		}
		return &datamodel.CredentialResourceProperties{
			Kind: *p.Kind,
			AWSCredential: &datamodel.AWSCredentialProperties{
				AccessKeyID:     p.AccessKeyID,
				SecretAccessKey: p.SecretAccessKey,
			},
			Storage: storage,
		}, nil
	}
	return &datamodel.CredentialResourceProperties{
		Kind: *crendentialProperties.Kind,
		Storage: storage,
	}, nil
}

func (cr *CredentialResource) getCredentialStorageProperties() (*datamodel.CredentialStorageProperties,error) {
	storage := cr.Properties.GetCredentialResourceProperties().Storage
	if *storage.GetCredentialStorageProperties().Kind == CredentialStorageKind("Internal") {
		p, ok := storage.(*InternalCredentialStorageProperties)
		if !ok {
			return nil, conv.ErrInvalidModelConversion
		}
		return &datamodel.CredentialStorageProperties{
			Kind: (*datamodel.CredentialStorageKind)(p.Kind),
			InternalCredential: &datamodel.InternalCredentialStorageProperties{
				SecretName: p.SecretName,
			},
		}, nil
	}
	return &datamodel.CredentialStorageProperties{
		Kind: (*datamodel.CredentialStorageKind)(storage.GetCredentialStorageProperties().Kind),
	}, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Credential resource.
func (dst *CredentialResource) ConvertFrom(src conv.DataModelInterface) error {
	credential, ok := src.(*datamodel.Credential)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.Location = &credential.Location
	dst.ID = &credential.ID
	dst.Name = &credential.Name
	dst.Type = &credential.Type

	switch *dst.Type {
	case AzureCredentialType:
		dst.Properties = &AzureServicePrincipalProperties{
			Kind:     &credential.Properties.Kind,
			ClientID: credential.Properties.AzureCredential.ClientID,
			TenantID: credential.Properties.AzureCredential.TenantID,
			Storage: getStorage(credential),
		}
	case AWSCredentialType:
		dst.Properties = &AWSCredentialProperties{
			Kind:            &credential.Properties.Kind,
			SecretAccessKey: credential.Properties.AWSCredential.SecretAccessKey,
			AccessKeyID:     credential.Properties.AWSCredential.AccessKeyID,
			Storage: getStorage(credential),
		}
	default:
		dst.Properties = &CredentialResourceProperties{
			Kind: &credential.Properties.Kind,
			Storage: getStorage(credential),
		}
	}
	return nil
}

func getStorage(credential *datamodel.Credential) (CredentialStoragePropertiesClassification){
	if *credential.Properties.Storage.Kind == datamodel.CredentialStorageKind("Internal") {
		return &InternalCredentialStorageProperties{
			Kind: (*CredentialStorageKind)(credential.Properties.Storage.Kind),
			SecretName: credential.Properties.Storage.InternalCredential.SecretName,
		}
	}
	return &CredentialStorageProperties{
		Kind: (*CredentialStorageKind)(credential.Properties.Storage.Kind),
	}
}
