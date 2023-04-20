// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20230415preview

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
)

// ConvertTo converts from the versioned HTTPRoute resource to version-agnostic datamodel.
func (src *VolumeResource) ConvertTo() (v1.DataModelInterface, error) {
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
			BasicResourceProperties: rpv1.BasicResourceProperties{
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
func (dst *VolumeResource) ConvertFrom(src v1.DataModelInterface) error {
	resource, ok := src.(*datamodel.VolumeResource)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(resource.ID)
	dst.Name = to.Ptr(resource.Name)
	dst.Type = to.Ptr(resource.Type)
	dst.SystemData = fromSystemDataModel(resource.SystemData)
	dst.Location = to.Ptr(resource.Location)
	dst.Tags = *to.StringMapPtr(resource.Tags)

	switch resource.Properties.Kind {
	case datamodel.AzureKeyVaultVolume:
		azProp := resource.Properties.AzureKeyVault
		p := &AzureKeyVaultVolumeProperties{
			Status: &ResourceStatus{
				OutputResources: rpv1.BuildExternalOutputResources(resource.Properties.Status.OutputResources),
			},
			Kind:              to.Ptr(resource.Properties.Kind),
			Application:       to.Ptr(resource.Properties.Application),
			Resource:          to.Ptr(azProp.Resource),
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

func toStringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func fromKeyDataModel(dm *datamodel.KeyObjectProperties) *KeyObjectProperties {
	return &KeyObjectProperties{
		Name:    to.Ptr(dm.Name),
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
		Name:     to.Ptr(dm.Name),
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
			prop.Format = to.Ptr(datamodel.CertificateFormatPEM)
		case FormatPfx:
			prop.Format = to.Ptr(datamodel.CertificateFormatPFX)
		default:
			prop.Format = to.Ptr(datamodel.CertificateFormatPEM)
		}
	}

	if c.CertType != nil {
		switch *c.CertType {
		case CertTypeCertificate:
			prop.CertType = to.Ptr(datamodel.CertificateTypeCertificate)
		case CertTypePrivatekey:
			prop.CertType = to.Ptr(datamodel.CertificateTypePrivateKey)
		case CertTypePublickey:
			prop.CertType = to.Ptr(datamodel.CertificateTypePublicKey)
		}
	}

	return prop
}

func fromCertDataModel(dm *datamodel.CertificateObjectProperties) *CertificateObjectProperties {
	prop := &CertificateObjectProperties{
		Name:     to.Ptr(dm.Name),
		Alias:    toStringPtr(dm.Alias),
		Version:  toStringPtr(dm.Version),
		Encoding: fromEncoding(dm.Encoding),
	}

	if dm.Format != nil {
		switch *dm.Format {
		case datamodel.CertificateFormatPEM:
			prop.Format = to.Ptr(FormatPem)
		case datamodel.CertificateFormatPFX:
			prop.Format = to.Ptr(FormatPfx)
		default:
			prop.Format = to.Ptr(FormatPem)
		}
	}

	if dm.CertType != nil {
		switch *dm.CertType {
		case datamodel.CertificateTypeCertificate:
			prop.CertType = to.Ptr(CertTypeCertificate)
		case datamodel.CertificateTypePrivateKey:
			prop.CertType = to.Ptr(CertTypePrivatekey)
		case datamodel.CertificateTypePublicKey:
			prop.CertType = to.Ptr(CertTypePublickey)
		}
	}

	return prop
}
