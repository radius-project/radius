// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

const (
	// AzureKeyVaultVolume represents the resource of azure keyvault volume.
	AzureKeyVaultVolume string = "azure.com.keyvault"
)

// VolumeResource represents VolumeResource resource.
type VolumeResource struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties VolumeResourceProperties `json:"properties"`
}

// ResourceTypeName returns the qualified name of the resource
func (h *VolumeResource) ResourceTypeName() string {
	return "Applications.Core/volumes"
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (h *VolumeResource) ApplyDeploymentOutput(do rp.DeploymentOutput) {
	h.Properties.Status.OutputResources = do.DeployedOutputResources
}

// OutputResources returns the output resources array.
func (h *VolumeResource) OutputResources() []outputresource.OutputResource {
	return h.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (h *VolumeResource) ResourceMetadata() *rp.BasicResourceProperties {
	return &h.Properties.BasicResourceProperties
}

// VolumeResourceProperties represents the properties of VolumeResource.
type VolumeResourceProperties struct {
	rp.BasicResourceProperties
	// Kind represents the type of Volume resource.
	Kind string `json:"kind,omitempty"`
	// AzureKeyVault represents Azure Keyvault volume properties
	AzureKeyVault *AzureKeyVaultVolumeProperties `json:"azureKeyVault,omitempty"`
}
type AzureIdentityKind string

const (
	AzureIdentityNone         AzureIdentityKind = "None"
	AzureIdentityWorkload     AzureIdentityKind = "Workload"
	AzureIdentityUserAssigned AzureIdentityKind = "UserAssigned"
)

// AzureIdentity represents the azure indentity info to access azure resource, such as Key vault.
type AzureIdentity struct {
	// Kind represents the type of authentication.
	Kind AzureIdentityKind `json:"kind"`
	// ClientID represents the client id of workload identity or user assigned managed identity.
	ClientID string `json:"clientId,omitempty"`
	// TenantID represents the tenant id for the resource.
	TenantID string `json:"tenantId,omitempty"`
}

// AzureKeyVaultVolumeProperties represents the volume for Azure Keyvault.
type AzureKeyVaultVolumeProperties struct {
	// The identity is to access keyvault
	Identity AzureIdentity `json:"identity"`
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
