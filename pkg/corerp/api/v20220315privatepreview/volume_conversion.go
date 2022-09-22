// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"

	azto "github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned HTTPRoute resource to version-agnostic datamodel.
func (src *VolumeResource) ConvertTo() (conv.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.
	// TODO: Improve the validation.
	converted := &datamodel.VolumeResource{
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
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.GetVolumeProperties().ProvisioningState),
			},
		},
		Properties: datamodel.VolumeResourceProperties{
			BasicResourceProperties: v1.BasicResourceProperties{
				Application: to.String(src.Properties.GetVolumeProperties().Application),
			},
			Kind: to.String(src.Properties.GetVolumeProperties().Kind),
		},
	}

	switch p := src.Properties.(type) {
	case *AzureKeyVaultVolumeProperties:
		converted.Properties.AzureKeyVault = &datamodel.AzureKeyVaultVolumeProperties{
			Resource:     to.String(p.Resource),
			Certificates: nil,
			Keys:         nil,
			Secrets:      nil,
		}
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned VolumeResource resource.
func (dst *VolumeResource) ConvertFrom(src conv.DataModelInterface) error {
	// TODO: Improve the validation.
	resource, ok := src.(*datamodel.VolumeResource)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(resource.ID)
	dst.Name = to.StringPtr(resource.Name)
	dst.Type = to.StringPtr(resource.Type)
	dst.SystemData = fromSystemDataModel(resource.SystemData)
	dst.Location = to.StringPtr(resource.Location)
	dst.Tags = *to.StringMapPtr(resource.Tags)

	switch resource.Properties.Kind {
	case "azure.com.keyvault":
		azProp := resource.Properties.AzureKeyVault
		p := &AzureKeyVaultVolumeProperties{
			Status: &ResourceStatus{
				OutputResources: v1.BuildExternalOutputResources(resource.Properties.Status.OutputResources),
			},
			Kind:        to.StringPtr(resource.Properties.Kind),
			Application: to.StringPtr(resource.Properties.Application),
			Resource:    to.StringPtr(azProp.Resource),
		}
		if azProp.Certificates != nil {
			p.Certificates = map[string]*CertificateObjectProperties{}
			for k, v := range azProp.Certificates {
				p.Certificates[k] = fromCertDataModel(&v)
			}
		}
		if azProp.Keys != nil {
			p.Keys = map[string]*KeyObjectProperties{}
			for k, v := range azProp.Keys {
				p.Keys[k] = fromKeyDataModel(&v)
			}
		}
		if azProp.Secrets != nil {
			p.Secrets = map[string]*SecretObjectProperties{}
			for k, v := range azProp.Secrets {
				p.Secrets[k] = fromSecretDataModel(&v)
			}
		}
		dst.Properties = p
	}

	return nil
}

func fromKeyDataModel(dm *datamodel.KeyObjectProperties) *KeyObjectProperties {
	return &KeyObjectProperties{
		Name:    azto.Ptr(dm.Name),
		Alias:   azto.Ptr(dm.Alias),
		Version: azto.Ptr(dm.Version),
	}
}

func fromSecretDataModel(dm *datamodel.SecretObjectProperties) *SecretObjectProperties {
	return &SecretObjectProperties{
		Name:     azto.Ptr(dm.Name),
		Alias:    azto.Ptr(dm.Alias),
		Version:  azto.Ptr(dm.Version),
		Encoding: fromEncoding(dm.Encoding),
	}
}

func fromEncoding(encode *datamodel.SecretEncoding) *Encoding {
	enc := EncodingUTF8

	if encode == nil {
		return &enc
	}

	switch *encode {
	case datamodel.SecretObjectPropertiesEncodingBase64:
		enc = EncodingBase64
	case datamodel.SecretObjectPropertiesEncodingHex:
		enc = EncodingHex
	case datamodel.SecretObjectPropertiesEncodingUTF8:
		enc = EncodingUTF8
	default:
		enc = EncodingUTF8
	}
	return &enc
}

func fromCertDataModel(dm *datamodel.CertificateObjectProperties) *CertificateObjectProperties {
	prop := &CertificateObjectProperties{
		Name:    &dm.Name,
		Alias:   &dm.Alias,
		Version: &dm.Version,
	}

	prop.Encoding = fromEncoding(dm.Encoding)

	if dm.Format != nil {
		switch *dm.Format {
		case datamodel.CertificateFormatPEM:
			prop.Format = azto.Ptr(FormatPem)
		case datamodel.CertificateFormatPFX:
			prop.Format = azto.Ptr(FormatPfx)
		default:
			prop.Format = azto.Ptr(FormatPem)
		}
	}

	if dm.CertType != nil {
		switch *dm.CertType {
		case datamodel.CertificateTypeCertificate:
			prop.CertType = azto.Ptr(TypeCertificate)
		case datamodel.CertificateTypePrivateKey:
			prop.CertType = azto.Ptr(TypePrivatekey)
		case datamodel.CertificateTypePublicKey:
			prop.CertType = azto.Ptr(TypePublickey)
		}
	}

	return prop
}
