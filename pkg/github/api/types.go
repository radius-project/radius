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

package api

// CreateAWSEnvironmentRequest is the request body for creating an AWS environment.
type CreateAWSEnvironmentRequest struct {
	// Name is the environment name (e.g., "dev", "staging", "prod").
	Name string `json:"name"`

	// RoleARN is the IAM role ARN with an OIDC trust policy for GitHub Actions.
	RoleARN string `json:"roleARN"`

	// Region is the AWS region for deployments.
	Region string `json:"region"`

	// AccountID is the AWS account ID (12-digit number).
	AccountID string `json:"accountID"`
}

// CreateAzureEnvironmentRequest is the request body for creating an Azure environment.
type CreateAzureEnvironmentRequest struct {
	// Name is the environment name (e.g., "dev", "staging", "prod").
	Name string `json:"name"`

	// TenantID is the Azure AD tenant ID.
	TenantID string `json:"tenantID"`

	// ClientID is the Azure AD application (client) ID.
	ClientID string `json:"clientID"`

	// SubscriptionID is the Azure subscription ID.
	SubscriptionID string `json:"subscriptionID"`

	// ResourceGroup is the Azure resource group name.
	ResourceGroup string `json:"resourceGroup,omitempty"`

	// AuthType is "WorkloadIdentity" or "ServicePrincipal".
	AuthType string `json:"authType"`

	// ClientSecret is the client secret for Service Principal auth.
	// Required only when AuthType is "ServicePrincipal".
	ClientSecret string `json:"clientSecret,omitempty"`

	// AzureAccessToken is an optional Microsoft Graph access token used to
	// automatically create the federated identity credential. If omitted,
	// the user must create the federated credential manually.
	AzureAccessToken string `json:"azureAccessToken,omitempty"`
}

// EnvironmentResponse is the response body for environment operations.
type EnvironmentResponse struct {
	// Name is the environment name.
	Name string `json:"name"`

	// Provider is "aws" or "azure".
	Provider string `json:"provider"`

	// GitHubEnvironmentCreated is true when the GitHub Environment exists.
	GitHubEnvironmentCreated bool `json:"githubEnvironmentCreated"`

	// VariablesSet lists the GitHub Environment variable names that were set.
	VariablesSet []string `json:"variablesSet"`

	// CredentialsVerified is true when cloud access has been verified.
	CredentialsVerified bool `json:"credentialsVerified"`

	// FederatedCredentialCreated is true when a federated identity credential
	// was automatically created on the Azure AD application.
	FederatedCredentialCreated bool `json:"federatedCredentialCreated,omitempty"`
}

// VerificationResponse is the response body for verification status.
type VerificationResponse struct {
	// Provider is "aws" or "azure".
	Provider string `json:"provider"`

	// Status is "pending", "in_progress", "success", or "failure".
	Status string `json:"status"`

	// Message contains human-readable details.
	Message string `json:"message"`

	// WorkflowRunURL is the GitHub Actions URL for the verification run.
	WorkflowRunURL string `json:"workflowRunURL,omitempty"`
}

// SaveDependenciesRequest is the request body for saving environment dependencies.
type SaveDependenciesRequest struct {
	// KubernetesCluster is the name or identifier of the Kubernetes cluster.
	KubernetesCluster string `json:"kubernetesCluster"`

	// KubernetesNamespace is the target namespace for deployments.
	KubernetesNamespace string `json:"kubernetesNamespace"`

	// OCIRegistry is the OCI container registry URL.
	OCIRegistry string `json:"ociRegistry,omitempty"`

	// VPC is the VPC identifier (AWS-specific).
	VPC string `json:"vpc,omitempty"`

	// Subnets is a comma-separated list of subnet identifiers (AWS-specific).
	Subnets string `json:"subnets,omitempty"`

	// ResourceGroup is the Azure resource group (Azure-specific).
	ResourceGroup string `json:"resourceGroup,omitempty"`
}

// DependenciesResponse is the response body for environment dependencies.
type DependenciesResponse struct {
	// VariablesSet lists the GitHub Environment variable names that were set.
	VariablesSet []string `json:"variablesSet"`
}

// ErrorResponse is a standard error response.
type ErrorResponse struct {
	Error string `json:"error"`
}

// DeployAppRequest is the request body for deploying a Radius application.
type DeployAppRequest struct {
	// AppFile is the Bicep file to deploy (e.g., "app.bicep").
	AppFile string `json:"appFile"`
}

// CreateAppFileRequest is the request body for creating an application Bicep file.
type CreateAppFileRequest struct {
	// Filename is the Bicep file name (e.g., "app.bicep").
	Filename string `json:"filename"`
}

// CreateAppFileResponse is the response body for creating an application Bicep file.
type CreateAppFileResponse struct {
	// Filename is the Bicep file name.
	Filename string `json:"filename"`

	// Created is true if the file was newly created, false if it already existed.
	Created bool `json:"created"`
}
