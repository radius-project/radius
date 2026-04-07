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

	// AccountID is the AWS account ID (12-digit number).
	AccountID string
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

	// AzureAccessToken is an optional Microsoft Graph access token used to
	// automatically create the federated identity credential on the Azure AD
	// application. If empty, federation setup is skipped.
	AzureAccessToken string
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

	// FederatedCredentialCreated is true when a federated identity credential
	// was created on the Azure AD application.
	FederatedCredentialCreated bool
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

// DeploymentSummary is a condensed view of a deploy workflow run.
type DeploymentSummary struct {
	// ID is the workflow run ID.
	ID int64 `json:"id"`

	// Status is the run status (e.g. "completed", "in_progress", "queued").
	Status string `json:"status"`

	// Conclusion is the final result (e.g. "success", "failure"). Empty while in progress.
	Conclusion string `json:"conclusion"`

	// AppFile is the bicep file that was deployed (extracted from workflow inputs).
	AppFile string `json:"appFile,omitempty"`

	// Environment is the target environment name.
	Environment string `json:"environment,omitempty"`

	// HTMLURL is the link to the workflow run on GitHub.
	HTMLURL string `json:"htmlURL"`

	// CreatedAt is the run creation timestamp.
	CreatedAt string `json:"createdAt"`

	// HeadBranch is the branch the workflow ran on.
	HeadBranch string `json:"headBranch,omitempty"`
}

// DependenciesConfig contains the configuration for environment dependencies.
type DependenciesConfig struct {
	// KubernetesCluster is the name or identifier of the Kubernetes cluster.
	KubernetesCluster string

	// KubernetesNamespace is the target namespace for deployments.
	KubernetesNamespace string

	// OCIRegistry is the OCI container registry URL.
	OCIRegistry string

	// VPC is the VPC identifier (AWS-specific).
	VPC string

	// Subnets is a comma-separated list of subnet identifiers (AWS-specific).
	Subnets string

	// ResourceGroup is the Azure resource group (Azure-specific).
	ResourceGroup string
}

// DependenciesResult describes the outcome of saving environment dependencies.
type DependenciesResult struct {
	// VariablesSet lists the GitHub Environment variable names that were set.
	VariablesSet []string
}

const (
	// AuthTypeWorkloadIdentity is the Azure Workload Identity (OIDC) credential type.
	AuthTypeWorkloadIdentity = "WorkloadIdentity"

	// AuthTypeServicePrincipal is the Azure Service Principal credential type.
	AuthTypeServicePrincipal = "ServicePrincipal"
)
