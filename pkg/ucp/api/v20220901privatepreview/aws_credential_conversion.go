// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220901privatepreview

import (
	"fmt"

	"github.com/Azure/go-autorest/autorest/to"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

const (
	// AWSCredentialType represents the ucp aws crendetial type value.
	AWSCredentialType = "System.AWS/credentials"
)

// ConvertTo converts from the versioned Credential resource to version-agnostic datamodel.
func (cr *AWSCredentialResource) ConvertTo() (v1.DataModelInterface, error) {
	crendentialProperties, err := cr.getDataModelCredentialProperties()
	if err != nil {
		return nil, err
	}

	converted := &datamodel.AWSCredential{
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

func (cr *AWSCredentialResource) getDataModelCredentialProperties() (*datamodel.AWSCredentialResourceProperties, error) {
	if cr.Properties == nil {
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties", ValidValue: "not nil"}
	}
	credentialProperties := cr.Properties

	if credentialProperties.Storage == nil {
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties.storage", ValidValue: "not nil"}
	}

	storageProperties := credentialProperties.Storage.GetCredentialStorageProperties()
	if storageProperties.GetCredentialStorageProperties().Kind == nil {
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties.storage.kind", ValidValue: "not nil"}
	}

	if cr.Type == nil || *cr.Type != AWSCredentialType {
		return nil, &v1.ErrModelConversion{PropertyName: "$.type", ValidValue: AWSCredentialType}
	}

	// Check for storage type value
	var found bool
	for _, k := range PossibleCredentialStorageKindValues() {
		if CredentialStorageKind(*storageProperties.Kind) == k {
			found = true
			break
		}
	}
	if !found {
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties.storage.kind", ValidValue: fmt.Sprintf("one of %s", PossibleCredentialStorageKindValues())}
	}
	storage, err := cr.getCredentialStorageProperties()
	if err != nil {
		return nil, err
	}

	return &datamodel.AWSCredentialResourceProperties{
		AWSCredential: &datamodel.AWSCredentialProperties{
			AccessKeyID:     credentialProperties.AccessKeyID,
			SecretAccessKey: credentialProperties.SecretAccessKey,
		},
		Storage: storage,
	}, nil
}

func (cr *AWSCredentialResource) getCredentialStorageProperties() (*datamodel.CredentialStorageProperties, error) {
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
func (dst *AWSCredentialResource) ConvertFrom(src v1.DataModelInterface) error {
	credential, ok := src.(*datamodel.AWSCredential)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = &credential.ID
	dst.Name = &credential.Name
	dst.Type = &credential.Type
	dst.Location = &credential.Location
	dst.Tags = *to.StringMapPtr(credential.Tags)

	dst.Properties = &AWSCredentialProperties{
		SecretAccessKey: credential.Properties.AWSCredential.SecretAccessKey,
		AccessKeyID:     credential.Properties.AWSCredential.AccessKeyID,
		Storage:         getAWSStorage(credential),
	}

	return nil
}

func getAWSStorage(credential *datamodel.AWSCredential) CredentialStoragePropertiesClassification {
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
