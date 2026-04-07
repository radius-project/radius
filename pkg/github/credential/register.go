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

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/github/azure"
	"github.com/radius-project/radius/pkg/github/environment"
)

// Service orchestrates creating GitHub Environments and storing cloud
// credentials as GitHub Environment variables/secrets. Credentials are
// consumed by GitHub Actions workflows at deploy time — Radius itself
// reads them from the workflow environment, not from its own store.
type Service interface {
	// CreateAWSEnvironment creates a GitHub Environment and stores the AWS
	// config as environment variables.
	CreateAWSEnvironment(ctx context.Context, owner, repo string, config AWSEnvironmentConfig) (*EnvironmentResult, error)

	// CreateAzureEnvironment creates a GitHub Environment and stores the
	// Azure config as environment variables (and optionally a secret for SP).
	CreateAzureEnvironment(ctx context.Context, owner, repo string, config AzureEnvironmentConfig) (*EnvironmentResult, error)

	// DeleteEnvironment removes a GitHub Environment.
	DeleteEnvironment(ctx context.Context, owner, repo, envName string) error

	// GetEnvironmentStatus returns the credential configuration state for
	// a GitHub Environment.
	GetEnvironmentStatus(ctx context.Context, owner, repo, envName string) (*EnvironmentResult, error)

	// SaveDependencies stores environment dependencies as GitHub Environment variables.
	SaveDependencies(ctx context.Context, owner, repo, envName string, config DependenciesConfig) (*DependenciesResult, error)

	// GetDependencies returns the current environment dependencies from GitHub Environment variables.
	GetDependencies(ctx context.Context, owner, repo, envName string) (map[string]string, error)
}

type service struct {
	ghClient        environment.Client
	federationClient *azure.FederationClient
}

// NewService creates a credential orchestration service that manages GitHub
// Environment variables for cloud credentials.
func NewService(ghClient environment.Client, federationClient *azure.FederationClient) Service {
	return &service{
		ghClient:        ghClient,
		federationClient: federationClient,
	}
}

func (s *service) CreateAWSEnvironment(ctx context.Context, owner, repo string, config AWSEnvironmentConfig) (*EnvironmentResult, error) {
	if config.EnvironmentName == "" {
		return nil, fmt.Errorf("environment name is required")
	}
	if config.RoleARN == "" {
		return nil, fmt.Errorf("IAM role ARN is required")
	}
	if config.Region == "" {
		return nil, fmt.Errorf("AWS region is required")
	}

	result := &EnvironmentResult{
		EnvironmentName: config.EnvironmentName,
		Provider:        "aws",
	}

	// 1. Create GitHub Environment.
	if err := s.ghClient.CreateEnvironment(ctx, owner, repo, config.EnvironmentName); err != nil {
		return nil, fmt.Errorf("failed to create GitHub Environment %q: %w", config.EnvironmentName, err)
	}
	result.GitHubEnvironmentCreated = true

	// 2. Set GitHub Environment variables.
	vars := map[string]string{
		"AWS_IAM_ROLE_ARN": config.RoleARN,
		"AWS_REGION":       config.Region,
	}
	if config.AccountID != "" {
		vars["AWS_ACCOUNT_ID"] = config.AccountID
	}
	for key, value := range vars {
		if err := s.ghClient.SetVariable(ctx, owner, repo, config.EnvironmentName, key, value); err != nil {
			return nil, fmt.Errorf("failed to set variable %q: %w", key, err)
		}
		result.VariablesSet = append(result.VariablesSet, key)
	}

	return result, nil
}

func (s *service) CreateAzureEnvironment(ctx context.Context, owner, repo string, config AzureEnvironmentConfig) (*EnvironmentResult, error) {
	if config.EnvironmentName == "" {
		return nil, fmt.Errorf("environment name is required")
	}
	if config.TenantID == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}
	if config.ClientID == "" {
		return nil, fmt.Errorf("client ID is required")
	}
	if config.SubscriptionID == "" {
		return nil, fmt.Errorf("subscription ID is required")
	}
	if config.AuthType != AuthTypeWorkloadIdentity && config.AuthType != AuthTypeServicePrincipal {
		return nil, fmt.Errorf("auth type must be %q or %q", AuthTypeWorkloadIdentity, AuthTypeServicePrincipal)
	}
	if config.AuthType == AuthTypeServicePrincipal && config.ClientSecret == "" {
		return nil, fmt.Errorf("client secret is required for ServicePrincipal auth type")
	}

	result := &EnvironmentResult{
		EnvironmentName: config.EnvironmentName,
		Provider:        "azure",
	}

	// 1. Create GitHub Environment.
	if err := s.ghClient.CreateEnvironment(ctx, owner, repo, config.EnvironmentName); err != nil {
		return nil, fmt.Errorf("failed to create GitHub Environment %q: %w", config.EnvironmentName, err)
	}
	result.GitHubEnvironmentCreated = true

	// 2. Set GitHub Environment variables (non-secret).
	vars := map[string]string{
		"AZURE_TENANT_ID":       config.TenantID,
		"AZURE_CLIENT_ID":       config.ClientID,
		"AZURE_SUBSCRIPTION_ID": config.SubscriptionID,
	}
	if config.ResourceGroup != "" {
		vars["AZURE_RESOURCE_GROUP"] = config.ResourceGroup
	}
	for key, value := range vars {
		if err := s.ghClient.SetVariable(ctx, owner, repo, config.EnvironmentName, key, value); err != nil {
			return nil, fmt.Errorf("failed to set variable %q: %w", key, err)
		}
		result.VariablesSet = append(result.VariablesSet, key)
	}

	// 3. For Service Principal, store the client secret as a GitHub Environment secret.
	if config.AuthType == AuthTypeServicePrincipal {
		if err := s.ghClient.SetSecret(ctx, owner, repo, config.EnvironmentName, "AZURE_CLIENT_SECRET", config.ClientSecret); err != nil {
			return nil, fmt.Errorf("failed to set client secret: %w", err)
		}
	}

	// 4. For Workload Identity with an Azure access token, create the federated
	// identity credential on the Azure AD application automatically.
	if config.AuthType == AuthTypeWorkloadIdentity && config.AzureAccessToken != "" && s.federationClient != nil {
		// Look up the Application Object ID from the Client ID.
		objectID, err := s.federationClient.LookupApplicationObjectID(ctx, config.AzureAccessToken, config.ClientID)
		if err != nil {
			return nil, fmt.Errorf("failed to look up Azure AD application: %w", err)
		}

		err = s.federationClient.EnsureFederatedCredential(ctx, config.AzureAccessToken, azure.FederatedCredentialConfig{
			ApplicationObjectID: objectID,
			Owner:               owner,
			Repo:                repo,
			EnvironmentName:     config.EnvironmentName,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create federated identity credential: %w", err)
		}
		result.FederatedCredentialCreated = true
	}

	return result, nil
}

func (s *service) DeleteEnvironment(ctx context.Context, owner, repo, envName string) error {
	if err := s.ghClient.DeleteEnvironment(ctx, owner, repo, envName); err != nil {
		return fmt.Errorf("failed to delete GitHub Environment %q: %w", envName, err)
	}
	return nil
}

func (s *service) GetEnvironmentStatus(ctx context.Context, owner, repo, envName string) (*EnvironmentResult, error) {
	exists, err := s.ghClient.EnvironmentExists(ctx, owner, repo, envName)
	if err != nil {
		return nil, fmt.Errorf("failed to check GitHub Environment %q: %w", envName, err)
	}
	if !exists {
		return nil, nil
	}

	vars, err := s.ghClient.GetVariables(ctx, owner, repo, envName)
	if err != nil {
		return nil, fmt.Errorf("failed to get variables for %q: %w", envName, err)
	}

	result := &EnvironmentResult{
		EnvironmentName:          envName,
		GitHubEnvironmentCreated: true,
	}

	varNames := make([]string, 0, len(vars))
	for k := range vars {
		varNames = append(varNames, k)
	}
	result.VariablesSet = varNames

	// Determine provider from variables.
	if _, ok := vars["AWS_IAM_ROLE_ARN"]; ok {
		result.Provider = "aws"
	} else if _, ok := vars["AZURE_TENANT_ID"]; ok {
		result.Provider = "azure"
	}

	return result, nil
}

func (s *service) SaveDependencies(ctx context.Context, owner, repo, envName string, config DependenciesConfig) (*DependenciesResult, error) {
	if config.KubernetesNamespace == "" {
		config.KubernetesNamespace = "default"
	}

	// Build the variable map — only set non-empty values.
	// KubernetesCluster and KubernetesNamespace are optional since the
	// ephemeral k3d model does not require the user's own cluster.
	vars := make(map[string]string)
	if config.KubernetesCluster != "" {
		vars["RADIUS_K8S_CLUSTER"] = config.KubernetesCluster
	}
	if config.KubernetesNamespace != "" {
		vars["RADIUS_K8S_NAMESPACE"] = config.KubernetesNamespace
	}
	if config.OCIRegistry != "" {
		vars["RADIUS_OCI_REGISTRY"] = config.OCIRegistry
	}
	if config.VPC != "" {
		vars["RADIUS_VPC"] = config.VPC
	}
	if config.Subnets != "" {
		vars["RADIUS_SUBNETS"] = config.Subnets
	}
	if config.ResourceGroup != "" {
		vars["RADIUS_RESOURCE_GROUP"] = config.ResourceGroup
	}

	result := &DependenciesResult{}
	for key, value := range vars {
		if err := s.ghClient.SetVariable(ctx, owner, repo, envName, key, value); err != nil {
			return nil, fmt.Errorf("failed to set variable %q: %w", key, err)
		}
		result.VariablesSet = append(result.VariablesSet, key)
	}

	return result, nil
}

func (s *service) GetDependencies(ctx context.Context, owner, repo, envName string) (map[string]string, error) {
	allVars, err := s.ghClient.GetVariables(ctx, owner, repo, envName)
	if err != nil {
		return nil, fmt.Errorf("failed to get variables for %q: %w", envName, err)
	}

	// Filter to RADIUS_ dependency variables only.
	depKeys := []string{
		"RADIUS_K8S_CLUSTER",
		"RADIUS_K8S_NAMESPACE",
		"RADIUS_OCI_REGISTRY",
		"RADIUS_VPC",
		"RADIUS_SUBNETS",
		"RADIUS_RESOURCE_GROUP",
	}
	deps := make(map[string]string)
	for _, key := range depKeys {
		if val, ok := allVars[key]; ok {
			deps[key] = val
		}
	}

	return deps, nil
}
