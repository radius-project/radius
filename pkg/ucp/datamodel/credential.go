// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"

const (
	// InternalStorageKind represents ucp credential storage type for internal credential type
	InternalStorageKind = CredentialStorageKind("Internal")
	// AzureCredentialKind represents ucp credential kind for azure credentials.
	AzureCredentialKind = "azure.com.serviceprincipal"
	// AWSCredentialKind represents ucp credential kind for aws credentials.
	AWSCredentialKind = "aws.com.iam"
)

// Credential represents UCP Credential.
type Credential struct {
	v1.BaseResource

	Properties *CredentialResourceProperties `json:"properties,omitempty"`
}

// ResourceTypeName gives the type of ucp resource.
func (c *Credential) ResourceTypeName() string {
	return c.Type
}

// Credential Properties represents UCP Credential Properties.
type CredentialResourceProperties struct {
	// Kind is the kind of credential resource.
	Kind string `json:"kind,omitempty"`
	// AzureCredential is the azure service principal credentials.
	AzureCredential *AzureCredentialProperties `json:"azureCredential,omitempty"`
	// AWSCredential is the aws iam credentials.
	AWSCredential *AWSCredentialProperties `json:"awsCredential,omitempty"`
	// Storage contains the properties of the storage associated with the kind.
	Storage *CredentialStorageProperties `json:"storage,omitempty"`
}

// AzureCredentialProperties contains ucp Azure credential properties.
type AzureCredentialProperties struct {
	// TenantID represents the tenantId of azure service principal.
	TenantID *string `json:"tenantId,omitempty"`
	// ClientID represents the clientId of azure service principal.
	ClientID *string `json:"clientId,omitempty"`
}

// AWSCredentialProperties contains ucp AWS credential properties.
type AWSCredentialProperties struct {
	// AccessKeyID contains aws access key for iam.
	AccessKeyID *string `json:"accessKeyId,omitempty"`
	// SecretAccessKey contains secret access key for iam.
	SecretAccessKey *string `json:"secretAccessKey,omitempty"`
}

// CredentialStorageKind represents ucp credential storage kind.
type CredentialStorageKind string

// CredentialStorageProperties contains ucp credential storage properties.
type CredentialStorageProperties struct {
	// Kind represents ucp credential storage kind.
	Kind *CredentialStorageKind `json:"kind,omitempty"`
	// InternalCredential represents ucp internal credential storage properties.
	InternalCredential *InternalCredentialStorageProperties `json:"internalCredential,omitempty"`
}

// InternalCredentialStorageProperties contains ucp internal credential storage properties.
type InternalCredentialStorageProperties struct {
	// SecretName is the name of secret stored in ucp for the crendentials.
	SecretName *string `json:"secretName"`
}
