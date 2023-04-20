// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20230415preview

import (
	"fmt"

	azto "github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/go-autorest/autorest/to"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

const (
	// AwsCredentialType represents the ucp azure crendetial type value.
	AWSCredentialType = "System.AWS/credentials"
)

// ConvertTo converts from the versioned Credential resource to version-agnostic datamodel.
func (cr *AWSCredentialResource) ConvertTo() (v1.DataModelInterface, error) {
	prop, err := cr.getDataModelCredentialProperties()
	if err != nil {
		return nil, err
	}

	converted := &datamodel.AWSCredential{
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

func (cr *AWSCredentialResource) getDataModelCredentialProperties() (*datamodel.AWSCredentialResourceProperties, error) {
	if cr.Properties == nil {
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties", ValidValue: "not nil"}
	}

	switch p := cr.Properties.(type) {
	case *AWSAccessKeyCredentialProperties:
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

		return &datamodel.AWSCredentialResourceProperties{
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
func (dst *AWSCredentialResource) ConvertFrom(src v1.DataModelInterface) error {
	dm, ok := src.(*datamodel.AWSCredential)
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
			Kind:       azto.Ptr(string(CredentialStorageKindInternal)),
			SecretName: azto.Ptr(dm.Properties.Storage.InternalCredential.SecretName),
		}
	default:
		return v1.ErrInvalidModelConversion
	}

	// DO NOT convert any secret values to versioned model.
	switch dm.Properties.Kind {
	case datamodel.AWSCredentialKind:
		dst.Properties = &AWSAccessKeyCredentialProperties{
			Kind:        azto.Ptr(dm.Properties.Kind),
			AccessKeyID: azto.Ptr(dm.Properties.AWSCredential.AccessKeyID),
			Storage:     storage,
		}
	default:
		return v1.ErrInvalidModelConversion
	}

	return nil
}
