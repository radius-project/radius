// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220901privatepreview

import (
	"fmt"

	azto "github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/go-autorest/autorest/to"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

const (
	// AzureCredentialType represents the ucp azure crendetial type value.
	AzureCredentialType = "System.Azure/credentials"
)

// ConvertTo converts from the versioned Credential resource to version-agnostic datamodel.
func (cr *AzureCredentialResource) ConvertTo() (v1.DataModelInterface, error) {
	crendentialProperties, err := cr.getDataModelCredentialProperties()
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
	crendentialProperties := cr.Properties

	var storage *datamodel.CredentialStorageProperties

	storageProperties := crendentialProperties.Storage.GetCredentialStorageProperties()
	if storageProperties.GetCredentialStorageProperties().Kind == nil {
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties.storage.kind", ValidValue: "not nil"}
	}

	if cr.Type == nil || *cr.Type != AzureCredentialType {
		return nil, &v1.ErrModelConversion{PropertyName: "$.type", ValidValue: AzureCredentialType}
	}
	
	// Check for storage type value
	var found bool
	for _, k := range PossibleCredentialStorageKindValues() {
		if CredentialStorageKind(*storageProperties.Kind) == k {
			found = true
			break
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

	return &datamodel.AzureCredentialResourceProperties{
		AzureCredential: &datamodel.AzureCredentialProperties{
			TenantID: crendentialProperties.TenantID,
			ClientID: crendentialProperties.ClientID,
		},
		Storage: storage,
	}, nil

}

func (cr *AzureCredentialResource) getCredentialStorageProperties() (*datamodel.CredentialStorageProperties, error) {
	storageKind := datamodel.CredentialStorageKind(*cr.Properties.Storage.GetCredentialStorageProperties().Kind)
	switch p := cr.Properties.Storage.(type) {
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
func (dst *AzureCredentialResource) ConvertFrom(src v1.DataModelInterface) error {
	credential, ok := src.(*datamodel.AzureCredential)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = &dm.ID
	dst.Name = &dm.Name
	dst.Type = &dm.Type
	dst.Location = &dm.Location
	dst.Tags = *to.StringMapPtr(dm.Tags)

	dst.Properties = &AzureServicePrincipalProperties{
		ClientID: credential.Properties.AzureCredential.ClientID,
		TenantID: credential.Properties.AzureCredential.TenantID,
		Storage:  getAzureStorage(credential),
	}

	return nil
}

func getAzureStorage(credential *datamodel.AzureCredential) CredentialStoragePropertiesClassification {
	credentialStorageKind := string(*credential.Properties.Storage.Kind)
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
