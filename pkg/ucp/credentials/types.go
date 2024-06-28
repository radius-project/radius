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

package credentials

import (
	"context"

	ucp_dm "github.com/radius-project/radius/pkg/ucp/datamodel"
)

const (
	// AzureCloud represents the public cloud plane name for UCP.
	AzureCloud = "azurecloud"

	// AWSPublic represents the aws public cloud plane name for UCP.
	AWSPublic = "aws"

	// AzureServicePrincipalCredentialKind represents the kind of Azure service principal credential.
	AzureServicePrincipalCredentialKind = ucp_dm.AzureServicePrincipalCredentialKind

	// AzureWorkloadIdentityCredentialKind represents the kind of Azure workload identity credential.
	AzureWorkloadIdentityCredentialKind = ucp_dm.AzureWorkloadIdentityCredentialKind

	// AWSAccessKeyCredentialKind represents the kind of AWS access key credential.
	AWSAccessKeyCredentialKind = ucp_dm.AWSAccessKeyCredentialKind

	// AWSIRSACredentialKind represents the kind of AWS IRSA credential.
	AWSIRSACredentialKind = ucp_dm.AWSIRSACredentialKind
)

type (
	// AzureCredential represents a credential for Azure AD.
	AzureCredential = ucp_dm.AzureCredentialProperties
	// AzureServicePrincipalCredential represents a credential for Azure AD service principal.
	AzureServicePrincipalCredential = ucp_dm.AzureServicePrincipalCredentialProperties
	// AzureWorkloadIdentityCredential represents a credential for Azure AD workload identity.
	AzureWorkloadIdentityCredential = ucp_dm.AzureWorkloadIdentityCredentialProperties
	// AWSCredential represents a credential for AWS IAM.
	AWSCredential = ucp_dm.AWSCredentialProperties
	// AWSAccessKeyCredential represents a credential for AWS access key.
	AWSAccessKeyCredential = ucp_dm.AWSAccessKeyCredentialProperties
	// AWSIRSACredential represents a credential for AWS IRSA.
	AWSIRSACredential = ucp_dm.AWSIRSACredentialProperties
)

// CredentialProvider is an UCP credential provider interface.
type CredentialProvider[T any] interface {
	// Fetch gets the credentials from secret storage.
	Fetch(ctx context.Context, planeName, name string) (*T, error)
}
