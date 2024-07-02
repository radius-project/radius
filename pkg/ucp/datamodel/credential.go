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

import v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"

const (
	// InternalStorageKind represents ucp credential storage type for internal credential type
	InternalStorageKind = "Internal"
	// AzureServicePrincipalCredentialKind represents ucp credential kind for Azure service principal credentials.
	AzureServicePrincipalCredentialKind = "ServicePrincipal"
	// AzureWorkloadIdentityCredentialKind represents ucp credential kind for Azure workload identity credentials.
	AzureWorkloadIdentityCredentialKind = "WorkloadIdentity"
	// AWSAccessKeyCredentialKind represents ucp credential kind for aws access key credentials.
	AWSAccessKeyCredentialKind = "AccessKey"
	// AWSIRSACredentialKind represents ucp credential kind for aws irsa credentials.
	AWSIRSACredentialKind = "IRSA"
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
	// Kind is the kind of Azure credential resource.
	Kind string `json:"kind,omitempty"`
	// AzureCredential is the Azure credentials.
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

// AzureServicePrincipalCredentialProperties contains ucp Azure service principal credential properties.
type AzureServicePrincipalCredentialProperties struct {
	// TenantID represents the tenantId of azure service principal credential.
	TenantID string `json:"tenantId"`
	// ClientID represents the clientId of azure service principal credential.
	ClientID string `json:"clientId"`
	// ClientSecret represents the client secret of service principal credential.
	ClientSecret string `json:"clientSecret,omitempty"`
}

// AzureWorkloadIdentityCredentialProperties contains ucp Azure workload identity credential properties.
type AzureWorkloadIdentityCredentialProperties struct {
	// TenantID represents the tenantId of azure workload identity credential.
	TenantID string `json:"tenantId"`
	// ClientID represents the clientId of azure service principal credential.
	ClientID string `json:"clientId"`
}

type AzureCredentialProperties struct {
	// Kind is the kind of Azure credential.
	Kind string `json:"kind,omitempty"`
	// ServicePrincipal represents the service principal properties.
	ServicePrincipal *AzureServicePrincipalCredentialProperties `json:"servicePrincipal,omitempty"`
	// WorkloadIdentity represents the workload identity properties.
	WorkloadIdentity *AzureWorkloadIdentityCredentialProperties `json:"workloadIdentity,omitempty"`
}

// AWSAccessKeyCredentialProperties contains ucp AWS credential properties.
type AWSAccessKeyCredentialProperties struct {
	// AccessKeyID contains aws access key for iam.
	AccessKeyID string `json:"accessKeyId"`
	// SecretAccessKey contains secret access key for iam.
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
}

// AWSIRSACredentialProperties contains ucp AWS IRSA credential properties.
type AWSIRSACredentialProperties struct {
	// RoleARN contains aws role arn for irsa.
	RoleARN string `json:"roleARN"`
}

// AWSCredentialProperties contains ucp AWS credential properties.
type AWSCredentialProperties struct {
	// Kind is the kind of AWS credential.
	Kind string `json:"kind,omitempty"`
	// AccessKeyCredential represents the access key credential properties.
	AccessKeyCredential *AWSAccessKeyCredentialProperties `json:"accesskey,omitempty"`
	// IRSA represents the irsa credential properties.
	IRSACredential *AWSIRSACredentialProperties `json:"irsa,omitempty"`
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
