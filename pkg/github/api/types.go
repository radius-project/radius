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

// ErrorResponse is a standard error response.
type ErrorResponse struct {
	Error string `json:"error"`
}
