//go:build integration

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
	"os"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/github/environment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_AWSEnvironmentConfig tests creating an environment with
// AWS credentials using the credential.Service.
func TestIntegration_AWSEnvironmentConfig(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	owner := os.Getenv("TEST_GITHUB_OWNER")
	repo := os.Getenv("TEST_GITHUB_REPO")

	if token == "" || owner == "" || repo == "" {
		t.Skip("Skipping integration test: GITHUB_TOKEN, TEST_GITHUB_OWNER, and TEST_GITHUB_REPO must be set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ghClient := environment.NewClient(environment.StaticTokenSource(token))
	credService := NewService(ghClient)
	envName := fmt.Sprintf("radius-aws-test-%d", time.Now().Unix())

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = credService.DeleteEnvironment(cleanupCtx, owner, repo, envName)
	})

	// Use credential.Service to create the AWS environment.
	config := AWSEnvironmentConfig{
		EnvironmentName: envName,
		RoleARN:         "arn:aws:iam::123456789012:role/test-role",
		Region:          "us-east-1",
	}
	result, err := credService.CreateAWSEnvironment(ctx, owner, repo, config)
	require.NoError(t, err)
	assert.True(t, result.GitHubEnvironmentCreated, "GitHub environment should be created")
	assert.Equal(t, "aws", result.Provider)
	assert.Contains(t, result.VariablesSet, "AWS_IAM_ROLE_ARN")
	assert.Contains(t, result.VariablesSet, "AWS_REGION")

	// Verify via GetEnvironmentStatus.
	status, err := credService.GetEnvironmentStatus(ctx, owner, repo, envName)
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, "aws", status.Provider)
	assert.Contains(t, status.VariablesSet, "AWS_IAM_ROLE_ARN")
	assert.Contains(t, status.VariablesSet, "AWS_REGION")

	// Read back raw values and verify.
	vars, err := ghClient.GetVariables(ctx, owner, repo, envName)
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:iam::123456789012:role/test-role", vars["AWS_IAM_ROLE_ARN"])
	assert.Equal(t, "us-east-1", vars["AWS_REGION"])

	t.Logf("AWS environment %q created with variables: %v", envName, result.VariablesSet)
}

// TestIntegration_AzureEnvironmentConfig tests creating an environment with
// Azure credentials using the credential.Service. It creates the environment
// and verifies the variables are set correctly. Azure access verification
// is done by the GitHub Actions workflow which references the created
// environment and uses the runner's OIDC token for federated login.
//
// When TEST_AZURE_ENV_NAME is set, the test uses that name and skips deletion
// so the workflow can verify Azure access from the runner. Otherwise it
// generates a name and cleans up after itself.
func TestIntegration_AzureEnvironmentConfig(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	owner := os.Getenv("TEST_GITHUB_OWNER")
	repo := os.Getenv("TEST_GITHUB_REPO")
	azureClientID := os.Getenv("TEST_AZURE_APP_ID")
	azureSubID := os.Getenv("TEST_AZURE_SUB_ID")
	azureTenantID := os.Getenv("TEST_AZURE_TENANT_ID")

	if token == "" || owner == "" || repo == "" {
		t.Skip("Skipping: GITHUB_TOKEN, TEST_GITHUB_OWNER, and TEST_GITHUB_REPO must be set")
	}
	if azureClientID == "" || azureSubID == "" || azureTenantID == "" {
		t.Skip("Skipping: TEST_AZURE_APP_ID, TEST_AZURE_SUB_ID, and TEST_AZURE_TENANT_ID must be set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ghClient := environment.NewClient(environment.StaticTokenSource(token))
	credService := NewService(ghClient)

	// If TEST_AZURE_ENV_NAME is set, use it and skip deletion (workflow handles cleanup).
	// Otherwise generate a unique name and clean up after ourselves.
	envName := os.Getenv("TEST_AZURE_ENV_NAME")
	skipDeletion := envName != ""
	if envName == "" {
		envName = fmt.Sprintf("radius-azure-test-%d", time.Now().Unix())
	}

	if !skipDeletion {
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanupCancel()
			_ = credService.DeleteEnvironment(cleanupCtx, owner, repo, envName)
			t.Logf("Cleaned up Azure test environment: %s", envName)
		})
	}

	// Step 1: Use credential.Service to create the Azure environment with credentials.
	t.Run("CreateAzureEnvironment", func(t *testing.T) {
		config := AzureEnvironmentConfig{
			EnvironmentName: envName,
			TenantID:        azureTenantID,
			ClientID:        azureClientID,
			SubscriptionID:  azureSubID,
			AuthType:        AuthTypeWorkloadIdentity,
		}
		result, err := credService.CreateAzureEnvironment(ctx, owner, repo, config)
		require.NoError(t, err, "Failed to create Azure environment via credential service")
		assert.True(t, result.GitHubEnvironmentCreated, "GitHub environment should be created")
		assert.Equal(t, "azure", result.Provider)
		assert.Contains(t, result.VariablesSet, "AZURE_TENANT_ID")
		assert.Contains(t, result.VariablesSet, "AZURE_CLIENT_ID")
		assert.Contains(t, result.VariablesSet, "AZURE_SUBSCRIPTION_ID")
		t.Logf("Created Azure environment with variables: %v", result.VariablesSet)
	})

	// Step 2: Use GetEnvironmentStatus to verify the credential service can read back the state.
	t.Run("VerifyEnvironmentStatus", func(t *testing.T) {
		status, err := credService.GetEnvironmentStatus(ctx, owner, repo, envName)
		require.NoError(t, err, "Failed to get environment status")
		require.NotNil(t, status, "Environment status should not be nil")
		assert.True(t, status.GitHubEnvironmentCreated)
		assert.Equal(t, "azure", status.Provider)
		assert.Contains(t, status.VariablesSet, "AZURE_TENANT_ID")
		assert.Contains(t, status.VariablesSet, "AZURE_CLIENT_ID")
		assert.Contains(t, status.VariablesSet, "AZURE_SUBSCRIPTION_ID")
		t.Logf("Environment status verified: provider=%s, vars=%v", status.Provider, status.VariablesSet)
	})

	// Step 3: Read back the raw variables and verify values match what was provided.
	t.Run("VerifyCredentialValues", func(t *testing.T) {
		vars, err := ghClient.GetVariables(ctx, owner, repo, envName)
		require.NoError(t, err, "Failed to get variables")

		require.NotEmpty(t, vars["AZURE_CLIENT_ID"], "AZURE_CLIENT_ID should be set")
		require.NotEmpty(t, vars["AZURE_SUBSCRIPTION_ID"], "AZURE_SUBSCRIPTION_ID should be set")
		require.NotEmpty(t, vars["AZURE_TENANT_ID"], "AZURE_TENANT_ID should be set")
		assert.True(t, vars["AZURE_CLIENT_ID"] == azureClientID, "AZURE_CLIENT_ID mismatch")
		assert.True(t, vars["AZURE_SUBSCRIPTION_ID"] == azureSubID, "AZURE_SUBSCRIPTION_ID mismatch")
		assert.True(t, vars["AZURE_TENANT_ID"] == azureTenantID, "AZURE_TENANT_ID mismatch")
		t.Log("Azure credential values verified on environment")
	})
}
