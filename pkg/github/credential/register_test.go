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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockGHClient implements environment.Client for testing.
type mockGHClient struct {
	createdEnvs  []string
	variables    map[string]map[string]string // envName -> key -> value
	secrets      map[string]map[string]string
	deletedEnvs  []string
	existingEnvs map[string]bool
	createEnvErr error
	setVarErr    error
	setSecretErr error
	deleteEnvErr error
}

func newMockGHClient() *mockGHClient {
	return &mockGHClient{
		variables:    make(map[string]map[string]string),
		secrets:      make(map[string]map[string]string),
		existingEnvs: make(map[string]bool),
	}
}

func (m *mockGHClient) CreateEnvironment(_ context.Context, _, _, envName string) error {
	if m.createEnvErr != nil {
		return m.createEnvErr
	}
	m.createdEnvs = append(m.createdEnvs, envName)
	m.existingEnvs[envName] = true
	return nil
}

func (m *mockGHClient) EnvironmentExists(_ context.Context, _, _, envName string) (bool, error) {
	return m.existingEnvs[envName], nil
}

func (m *mockGHClient) ListEnvironments(_ context.Context, _, _ string) ([]string, error) {
	names := make([]string, 0, len(m.existingEnvs))
	for k := range m.existingEnvs {
		names = append(names, k)
	}
	return names, nil
}

func (m *mockGHClient) SetVariable(_ context.Context, _, _, envName, key, value string) error {
	if m.setVarErr != nil {
		return m.setVarErr
	}
	if m.variables[envName] == nil {
		m.variables[envName] = make(map[string]string)
	}
	m.variables[envName][key] = value
	return nil
}

func (m *mockGHClient) GetVariables(_ context.Context, _, _, envName string) (map[string]string, error) {
	if m.variables[envName] == nil {
		return map[string]string{}, nil
	}
	return m.variables[envName], nil
}

func (m *mockGHClient) SetSecret(_ context.Context, _, _, envName, key, value string) error {
	if m.setSecretErr != nil {
		return m.setSecretErr
	}
	if m.secrets[envName] == nil {
		m.secrets[envName] = make(map[string]string)
	}
	m.secrets[envName][key] = value
	return nil
}

func (m *mockGHClient) DeleteEnvironment(_ context.Context, _, _, envName string) error {
	if m.deleteEnvErr != nil {
		return m.deleteEnvErr
	}
	m.deletedEnvs = append(m.deletedEnvs, envName)
	delete(m.existingEnvs, envName)
	return nil
}

func TestCreateAWSEnvironment(t *testing.T) {
	ghClient := newMockGHClient()
	svc := NewService(ghClient)

	result, err := svc.CreateAWSEnvironment(context.Background(), "owner", "repo", AWSEnvironmentConfig{
		EnvironmentName: "dev",
		RoleARN:         "arn:aws:iam::123456:role/radius-role",
		Region:          "us-east-1",
	})

	require.NoError(t, err)
	assert.Equal(t, "dev", result.EnvironmentName)
	assert.Equal(t, "aws", result.Provider)
	assert.True(t, result.GitHubEnvironmentCreated)
	assert.Contains(t, result.VariablesSet, "AWS_IAM_ROLE_ARN")
	assert.Contains(t, result.VariablesSet, "AWS_REGION")

	// Verify GitHub Environment was created.
	assert.Contains(t, ghClient.createdEnvs, "dev")
	assert.Equal(t, "arn:aws:iam::123456:role/radius-role", ghClient.variables["dev"]["AWS_IAM_ROLE_ARN"])
	assert.Equal(t, "us-east-1", ghClient.variables["dev"]["AWS_REGION"])
}

func TestCreateAWSEnvironment_MissingFields(t *testing.T) {
	svc := NewService(newMockGHClient())

	_, err := svc.CreateAWSEnvironment(context.Background(), "owner", "repo", AWSEnvironmentConfig{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "environment name is required")

	_, err = svc.CreateAWSEnvironment(context.Background(), "owner", "repo", AWSEnvironmentConfig{
		EnvironmentName: "dev",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "IAM role ARN is required")
}

func TestCreateAzureEnvironment_WorkloadIdentity(t *testing.T) {
	ghClient := newMockGHClient()
	svc := NewService(ghClient)

	result, err := svc.CreateAzureEnvironment(context.Background(), "owner", "repo", AzureEnvironmentConfig{
		EnvironmentName: "dev",
		TenantID:        "tenant-123",
		ClientID:        "client-456",
		SubscriptionID:  "sub-789",
		ResourceGroup:   "my-rg",
		AuthType:        AuthTypeWorkloadIdentity,
	})

	require.NoError(t, err)
	assert.Equal(t, "azure", result.Provider)
	assert.True(t, result.GitHubEnvironmentCreated)
	assert.Contains(t, result.VariablesSet, "AZURE_TENANT_ID")
	assert.Contains(t, result.VariablesSet, "AZURE_CLIENT_ID")
	assert.Contains(t, result.VariablesSet, "AZURE_SUBSCRIPTION_ID")
	assert.Contains(t, result.VariablesSet, "AZURE_RESOURCE_GROUP")

	// No secret should be set for WI.
	assert.Empty(t, ghClient.secrets)
}

func TestCreateAzureEnvironment_ServicePrincipal(t *testing.T) {
	ghClient := newMockGHClient()
	svc := NewService(ghClient)

	result, err := svc.CreateAzureEnvironment(context.Background(), "owner", "repo", AzureEnvironmentConfig{
		EnvironmentName: "staging",
		TenantID:        "tenant-123",
		ClientID:        "client-456",
		SubscriptionID:  "sub-789",
		AuthType:        AuthTypeServicePrincipal,
		ClientSecret:    "super-secret",
	})

	require.NoError(t, err)
	assert.True(t, result.GitHubEnvironmentCreated)

	// Verify client secret was stored as a GitHub secret.
	assert.Equal(t, "super-secret", ghClient.secrets["staging"]["AZURE_CLIENT_SECRET"])
}

func TestCreateAzureEnvironment_SPMissingSecret(t *testing.T) {
	svc := NewService(newMockGHClient())

	_, err := svc.CreateAzureEnvironment(context.Background(), "owner", "repo", AzureEnvironmentConfig{
		EnvironmentName: "dev",
		TenantID:        "t",
		ClientID:        "c",
		SubscriptionID:  "s",
		AuthType:        AuthTypeServicePrincipal,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client secret is required")
}

func TestCreateAzureEnvironment_InvalidAuthType(t *testing.T) {
	svc := NewService(newMockGHClient())

	_, err := svc.CreateAzureEnvironment(context.Background(), "owner", "repo", AzureEnvironmentConfig{
		EnvironmentName: "dev",
		TenantID:        "t",
		ClientID:        "c",
		SubscriptionID:  "s",
		AuthType:        "InvalidType",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "auth type must be")
}

func TestDeleteEnvironment(t *testing.T) {
	ghClient := newMockGHClient()
	ghClient.existingEnvs["dev"] = true
	svc := NewService(ghClient)

	err := svc.DeleteEnvironment(context.Background(), "owner", "repo", "dev")
	require.NoError(t, err)
	assert.Contains(t, ghClient.deletedEnvs, "dev")
}

func TestGetEnvironmentStatus(t *testing.T) {
	ghClient := newMockGHClient()
	ghClient.existingEnvs["dev"] = true
	ghClient.variables["dev"] = map[string]string{
		"AWS_IAM_ROLE_ARN": "arn:aws:iam::123:role/test",
		"AWS_REGION":       "us-east-1",
	}
	svc := NewService(ghClient)

	result, err := svc.GetEnvironmentStatus(context.Background(), "owner", "repo", "dev")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "aws", result.Provider)
	assert.True(t, result.GitHubEnvironmentCreated)
}

func TestGetEnvironmentStatus_NotFound(t *testing.T) {
	svc := NewService(newMockGHClient())

	result, err := svc.GetEnvironmentStatus(context.Background(), "owner", "repo", "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGenerateVerificationWorkflow_AWS(t *testing.T) {
	content, err := generateVerificationWorkflow("aws", "dev")
	require.NoError(t, err)
	assert.Contains(t, string(content), "aws sts get-caller-identity")
	assert.Contains(t, string(content), "vars.AWS_IAM_ROLE_ARN")
}

func TestGenerateVerificationWorkflow_Azure(t *testing.T) {
	content, err := generateVerificationWorkflow("azure", "staging")
	require.NoError(t, err)
	assert.Contains(t, string(content), "az account show")
	assert.Contains(t, string(content), "vars.AZURE_CLIENT_ID")
}
