/*
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
*/

package datamodel

import v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"

const (
	// InternalStorageKind represents ucp credential storage type for internal credential type
	InternalStorageKind = "Internal"
	// AzureCredentialKind represents ucp credential kind for azure credentials.
	AzureCredentialKind = "ServicePrincipal"
	// AWSCredentialKind represents ucp credential kind for aws credentials.
	AWSCredentialKind = "AccessKey"
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
	// Kind is the kind of azure credential resource.
	Kind string `json:"kind,omitempty"`
	// AzureCredential is the azure service principal credentials.
	AzureCredential *AzureCredentialProperties `json:"azureCredential,omitempty"`
	// Storage contains the properties of the storage associated with the kind.
	Storage *CredentialStorageProperties `json:"storage,omitempty"`
}

// AWS Credential Properties represents UCP Credential Properties.
type AWSCredentialResourceProperties struct {
	// Kind is the kind of aws credential resource.
	Kind string `json:"kind,omitempty"`
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
