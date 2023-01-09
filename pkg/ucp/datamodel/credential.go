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
type AzureCredential struct {
	v1.BaseResource

	Properties *AzureCredentialResourceProperties `json:"properties,omitempty"`
}

// ResourceTypeName gives the type of ucp resource.
func (c *AzureCredential) ResourceTypeName() string {
	return c.Type
}

// Credential represents UCP Credential.
type AWSCredential struct {
	v1.BaseResource

	Properties *AWSCredentialResourceProperties `json:"properties,omitempty"`
}

// ResourceTypeName gives the type of ucp resource.
func (c *AWSCredential) ResourceTypeName() string {
	return c.Type
}

// Azure Credential Properties represents UCP Credential Properties.
type AzureCredentialResourceProperties struct {
	// AzureCredential is the azure service principal credentials.
	AzureCredential *AzureCredentialProperties `json:"azureCredential,omitempty"`
	// Storage contains the properties of the storage associated with the kind.
	Storage *CredentialStorageProperties `json:"storage,omitempty"`
}

// AWS Credential Properties represents UCP Credential Properties.
type AWSCredentialResourceProperties struct {
	// AWSCredential is the aws iam credentials.
	AWSCredential *AWSCredentialProperties `json:"awsCredential,omitempty"`
	// Storage contains the properties of the storage associated with the kind.
	Storage *CredentialStorageProperties `json:"storage,omitempty"`
}

// AzureCredentialProperties contains ucp Azure credential properties.
type AzureCredentialProperties struct {
	// TenantID represents the tenantId of azure service principal.
	TenantID string `json:"tenantId"`
	// ClientID represents the clientId of azure service principal.
	ClientID string `json:"clientId"`
	// ClientSecret represents the client secret of service principal.
	ClientSecret string `json:"clientSecret,omitempty"`
}

// AWSCredentialProperties contains ucp AWS credential properties.
type AWSCredentialProperties struct {
	// AccessKeyID contains aws access key for iam.
	AccessKeyID string `json:"accessKeyId"`
	// SecretAccessKey contains secret access key for iam.
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
}

// CredentialStorageProperties contains ucp credential storage properties.
type CredentialStorageProperties struct {
	// Kind represents ucp credential storage kind.
	Kind string `json:"kind"`
	// InternalCredential represents ucp internal credential storage properties.
	InternalCredential *InternalCredentialStorageProperties `json:"internalCredential,omitempty"`
}

// InternalCredentialStorageProperties contains ucp internal credential storage properties.
type InternalCredentialStorageProperties struct {
	// SecretName is the name of secret stored in ucp for the crendentials.
	SecretName string `json:"secretName"`
}
