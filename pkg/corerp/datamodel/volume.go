/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

const VolumeResourceType = "Applications.Core/volumes"

const (
	// AzureKeyVaultVolume represents the resource of azure keyvault volume.
	AzureKeyVaultVolume string = "azure.com.keyvault"
)

// VolumeResource represents VolumeResource resource.
type VolumeResource struct {
	v1.BaseResource

	// TODO: remove this from CoreRP
	LinkMetadata

	// Properties is the properties of the resource.
	Properties VolumeResourceProperties `json:"properties"`
}

// ResourceTypeName returns the qualified name of the resource
func (h *VolumeResource) ResourceTypeName() string {
	return VolumeResourceType
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (h *VolumeResource) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	h.Properties.Status.OutputResources = do.DeployedOutputResources
	h.ComputedValues = do.ComputedValues
	h.SecretValues = do.SecretValues
	return nil
}

// OutputResources returns the output resources array.
func (h *VolumeResource) OutputResources() []rpv1.OutputResource {
	return h.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (h *VolumeResource) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &h.Properties.BasicResourceProperties
}

// VolumeResourceProperties represents the properties of VolumeResource.
type VolumeResourceProperties struct {
	rpv1.BasicResourceProperties
	// Kind represents the type of Volume resource.
	Kind string `json:"kind,omitempty"`
	// AzureKeyVault represents Azure Keyvault volume properties
	AzureKeyVault *AzureKeyVaultVolumeProperties `json:"azureKeyVault,omitempty"`
}

// AzureKeyVaultVolumeProperties represents the volume for Azure Keyvault.
type AzureKeyVaultVolumeProperties struct {
	// The KeyVault certificates that this volume exposes
	Certificates map[string]CertificateObjectProperties `json:"certificates,omitempty"`
	// The KeyVault keys that this volume exposes
	Keys map[string]KeyObjectProperties `json:"keys,omitempty"`
	// The ID of the keyvault to use for this volume resource
	Resource string `json:"resource,omitempty"`
	// The KeyVault secrets that this volume exposes
	Secrets map[string]SecretObjectProperties `json:"secrets,omitempty"`
}

// CertificateObjectProperties represents the certificate for Volume.
type CertificateObjectProperties struct {
	// The name of the certificate
	Name string `json:"name"`
	// File name when written to disk.
	Alias string `json:"alias,omitempty"`
	// Encoding format. Default utf-8
	Encoding *SecretEncoding `json:"encoding,omitempty"`
	// Certificate format. Default pem
	Format *CertificateFormat `json:"format,omitempty"`
	// Certificate version
	Version string `json:"version,omitempty"`
	// Certificate object type to be downloaded - the certificate itself, private key or public key of the certificate
	CertType *CertificateType `json:"certType,omitempty"`
}

type CertificateType string

const (
	CertificateTypePrivateKey  CertificateType = "privatekey"
	CertificateTypePublicKey   CertificateType = "publickey"
	CertificateTypeCertificate CertificateType = "certificate"
)

type CertificateFormat string

const (
	CertificateFormatPEM CertificateFormat = "pem"
	CertificateFormatPFX CertificateFormat = "pfx"
)

// SecretObjectProperties represents the secret object for Volume.
type SecretObjectProperties struct {
	// The name of the secret
	Name string `json:"name"`
	// File name when written to disk.
	Alias string `json:"alias,omitempty"`
	// Encoding format. Default utf-8
	Encoding *SecretEncoding `json:"encoding,omitempty"`
	// Secret version
	Version string `json:"version,omitempty"`
}

// SecretEncoding is the encoding for the secret object.
type SecretEncoding string

const (
	SecretObjectPropertiesEncodingBase64 SecretEncoding = "base64"
	SecretObjectPropertiesEncodingHex    SecretEncoding = "hex"
	SecretObjectPropertiesEncodingUTF8   SecretEncoding = "utf-8"
)

// KeyObjectProperties represents Key object volume.
type KeyObjectProperties struct {
	// The name of the key
	Name string `json:"name"`
	// File name when written to disk.
	Alias string `json:"alias,omitempty"`
	// Key version
	Version string `json:"version,omitempty"`
}
