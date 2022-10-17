// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/rp"

	azto "github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned HTTPRoute resource to version-agnostic datamodel.
func (src *VolumeResource) ConvertTo() (conv.DataModelInterface, error) {
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
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: to.String(src.Properties.GetVolumeProperties().Application),
			},
			Kind: to.String(src.Properties.GetVolumeProperties().Kind),
		},
	}

	switch p := src.Properties.(type) {
	case *AzureKeyVaultVolumeProperties:
		dm := &datamodel.AzureKeyVaultVolumeProperties{
			Resource: to.String(p.Resource),
		}

		if p.Identity != nil {
			dm.Identity = datamodel.AzureIdentity{
				Kind:     toAzureIdentityKind(p.Identity.Kind),
				ClientID: to.String(p.Identity.ClientID),
				TenantID: to.String(p.Identity.TenantID),
			}
		}

		if p.Certificates != nil {
			dm.Certificates = map[string]datamodel.CertificateObjectProperties{}
			for k, v := range p.Certificates {
				dm.Certificates[k] = *toCertDataModel(v)
			}
		}
		if p.Keys != nil {
			dm.Keys = map[string]datamodel.KeyObjectProperties{}
			for k, v := range p.Keys {
				dm.Keys[k] = *toKeyDataModel(v)
			}
		}
		if p.Secrets != nil {
			dm.Secrets = map[string]datamodel.SecretObjectProperties{}
			for k, v := range p.Secrets {
				dm.Secrets[k] = *toSecretDataModel(v)
			}
		}
		converted.Properties.AzureKeyVault = dm
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned VolumeResource resource.
func (dst *VolumeResource) ConvertFrom(src conv.DataModelInterface) error {
	resource, ok := src.(*datamodel.VolumeResource)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = azto.Ptr(resource.ID)
	dst.Name = azto.Ptr(resource.Name)
	dst.Type = azto.Ptr(resource.Type)
	dst.SystemData = fromSystemDataModel(resource.SystemData)
	dst.Location = azto.Ptr(resource.Location)
	dst.Tags = *to.StringMapPtr(resource.Tags)

	switch resource.Properties.Kind {
	case datamodel.AzureKeyVaultVolume:
		azProp := resource.Properties.AzureKeyVault
		p := &AzureKeyVaultVolumeProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(resource.Properties.Status.OutputResources),
			},
			Kind:        azto.Ptr(resource.Properties.Kind),
			Application: azto.Ptr(resource.Properties.Application),
			Identity: &AzureIdentity{
				Kind:     fromAzureIdentityKind(azProp.Identity.Kind),
				ClientID: toStringPtr(azProp.Identity.ClientID),
				TenantID: toStringPtr(azProp.Identity.TenantID),
			},
			Resource:          azto.Ptr(azProp.Resource),
			ProvisioningState: fromProvisioningStateDataModel(resource.InternalMetadata.AsyncProvisioningState),
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

func fromAzureIdentityKind(kind datamodel.AzureIdentityKind) *AzureIdentityKind {
	switch kind {
	case datamodel.AzureIdentitySystemAssigned:
		return azto.Ptr(AzureIdentityKindSystemAssigned)
	case datamodel.AzureIdentityWorkload:
		return azto.Ptr(AzureIdentityKindWorkload)
	default:
		return nil
	}
}

func toAzureIdentityKind(kind *AzureIdentityKind) datamodel.AzureIdentityKind {
	if kind == nil {
		return datamodel.AzureIdentityNone
	}

	switch *kind {
	case AzureIdentityKindSystemAssigned:
		return datamodel.AzureIdentitySystemAssigned
	case AzureIdentityKindWorkload:
		return datamodel.AzureIdentityWorkload
	default:
		return datamodel.AzureIdentityNone
	}
}

func toStringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func fromKeyDataModel(dm *datamodel.KeyObjectProperties) *KeyObjectProperties {
	return &KeyObjectProperties{
		Name:    azto.Ptr(dm.Name),
		Alias:   toStringPtr(dm.Alias),
		Version: toStringPtr(dm.Version),
	}
}

func toKeyDataModel(k *KeyObjectProperties) *datamodel.KeyObjectProperties {
	return &datamodel.KeyObjectProperties{
		Name:    to.String(k.Name),
		Alias:   to.String(k.Alias),
		Version: to.String(k.Version),
	}
}

func fromSecretDataModel(dm *datamodel.SecretObjectProperties) *SecretObjectProperties {
	return &SecretObjectProperties{
		Name:     azto.Ptr(dm.Name),
		Alias:    toStringPtr(dm.Alias),
		Version:  toStringPtr(dm.Version),
		Encoding: fromEncoding(dm.Encoding),
	}
}

func toSecretDataModel(s *SecretObjectProperties) *datamodel.SecretObjectProperties {
	return &datamodel.SecretObjectProperties{
		Name:     to.String(s.Name),
		Alias:    to.String(s.Alias),
		Version:  to.String(s.Version),
		Encoding: toEncoding(s.Encoding),
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

func toEncoding(e *Encoding) *datamodel.SecretEncoding {
	enc := datamodel.SecretObjectPropertiesEncodingUTF8

	if e == nil {
		return &enc
	}

	switch *e {
	case EncodingBase64:
		enc = datamodel.SecretObjectPropertiesEncodingBase64
	case EncodingHex:
		enc = datamodel.SecretObjectPropertiesEncodingHex
	case EncodingUTF8:
		enc = datamodel.SecretObjectPropertiesEncodingUTF8
	default:
		enc = datamodel.SecretObjectPropertiesEncodingUTF8
	}
	return &enc
}

func toCertDataModel(c *CertificateObjectProperties) *datamodel.CertificateObjectProperties {
	prop := &datamodel.CertificateObjectProperties{
		Name:     to.String(c.Name),
		Alias:    to.String(c.Alias),
		Version:  to.String(c.Version),
		Encoding: toEncoding(c.Encoding),
	}

	if c.Format != nil {
		switch *c.Format {
		case FormatPem:
			prop.Format = azto.Ptr(datamodel.CertificateFormatPEM)
		case FormatPfx:
			prop.Format = azto.Ptr(datamodel.CertificateFormatPFX)
		default:
			prop.Format = azto.Ptr(datamodel.CertificateFormatPEM)
		}
	}

	if c.CertType != nil {
		switch *c.CertType {
		case CertTypeCertificate:
			prop.CertType = azto.Ptr(datamodel.CertificateTypeCertificate)
		case CertTypePrivatekey:
			prop.CertType = azto.Ptr(datamodel.CertificateTypePrivateKey)
		case CertTypePublickey:
			prop.CertType = azto.Ptr(datamodel.CertificateTypePublicKey)
		}
	}

	return prop
}

func fromCertDataModel(dm *datamodel.CertificateObjectProperties) *CertificateObjectProperties {
	prop := &CertificateObjectProperties{
		Name:     azto.Ptr(dm.Name),
		Alias:    toStringPtr(dm.Alias),
		Version:  toStringPtr(dm.Version),
		Encoding: fromEncoding(dm.Encoding),
	}

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
			prop.CertType = azto.Ptr(CertTypeCertificate)
		case datamodel.CertificateTypePrivateKey:
			prop.CertType = azto.Ptr(CertTypePrivatekey)
		case datamodel.CertificateTypePublicKey:
			prop.CertType = azto.Ptr(CertTypePublickey)
		}
	}

	return prop
}
