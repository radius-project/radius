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

package credential

// AWSEnvironmentConfig contains the configuration needed to create an AWS
// environment backed by OIDC (IRSA).
type AWSEnvironmentConfig struct {
	// EnvironmentName is the name of the GitHub Environment and Radius environment.
	EnvironmentName string

	// RoleARN is the IAM role ARN with an OIDC trust policy for GitHub Actions.
	RoleARN string

	// Region is the AWS region for deployments.
	Region string
}

// AzureEnvironmentConfig contains the configuration needed to create an Azure
// environment backed by either Workload Identity or a Service Principal.
type AzureEnvironmentConfig struct {
	// EnvironmentName is the name of the GitHub Environment and Radius environment.
	EnvironmentName string

	// TenantID is the Azure AD tenant ID.
	TenantID string

	// ClientID is the Azure AD application (client) ID.
	ClientID string

	// SubscriptionID is the Azure subscription ID.
	SubscriptionID string

	// ResourceGroup is the Azure resource group name.
	ResourceGroup string

	// AuthType is "WorkloadIdentity" or "ServicePrincipal".
	AuthType string

	// ClientSecret is the Azure AD application client secret.
	// Required only when AuthType is "ServicePrincipal".
	ClientSecret string
}

// EnvironmentResult describes the outcome of creating or querying an environment.
type EnvironmentResult struct {
	// EnvironmentName is the GitHub/Radius environment name.
	EnvironmentName string

	// Provider is "aws" or "azure".
	Provider string

	// GitHubEnvironmentCreated is true when the GitHub Environment was created or already existed.
	GitHubEnvironmentCreated bool

	// VariablesSet lists the GitHub Environment variable names that were set.
	VariablesSet []string

	// CredentialsVerified is true when cloud access has been verified via a
	// GitHub Actions workflow run.
	CredentialsVerified bool
}

// VerificationResult describes the outcome of a credential verification workflow.
type VerificationResult struct {
	// Provider is "aws" or "azure".
	Provider string

	// Status is "pending", "in_progress", "success", or "failure".
	Status string

	// Message contains human-readable details (e.g. error output on failure).
	Message string

	// WorkflowRunURL is the GitHub Actions URL for the verification run.
	WorkflowRunURL string
}

const (
	// AuthTypeWorkloadIdentity is the Azure Workload Identity (OIDC) credential type.
	AuthTypeWorkloadIdentity = "WorkloadIdentity"

	// AuthTypeServicePrincipal is the Azure Service Principal credential type.
	AuthTypeServicePrincipal = "ServicePrincipal"
)
