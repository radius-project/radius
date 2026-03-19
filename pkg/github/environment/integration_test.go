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

package environment

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_EnvironmentLifecycle tests the full GitHub Environment
// lifecycle against the real GitHub API. It requires:
//
//   - GITHUB_TOKEN: a token with repo permissions
//   - TEST_GITHUB_OWNER: the repository owner (e.g., "radius-project")
//   - TEST_GITHUB_REPO: the repository name (e.g., "radius")
//
// Run with: go test -tags=integration -run TestIntegration ./pkg/github/environment/...
func TestIntegration_EnvironmentLifecycle(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	owner := os.Getenv("TEST_GITHUB_OWNER")
	repo := os.Getenv("TEST_GITHUB_REPO")

	if token == "" || owner == "" || repo == "" {
		t.Skip("Skipping integration test: GITHUB_TOKEN, TEST_GITHUB_OWNER, and TEST_GITHUB_REPO must be set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := NewClient(StaticTokenSource(token))
	envName := fmt.Sprintf("radius-test-%d", time.Now().Unix())

	// Cleanup on test completion (regardless of pass/fail).
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = client.DeleteEnvironment(cleanupCtx, owner, repo, envName)
		t.Logf("Cleaned up test environment: %s", envName)
	})

	// Step 1: Create the environment.
	t.Run("CreateEnvironment", func(t *testing.T) {
		err := client.CreateEnvironment(ctx, owner, repo, envName)
		require.NoError(t, err, "Failed to create environment %q", envName)
		t.Logf("Created environment: %s", envName)
	})

	// Step 2: Verify the environment exists.
	t.Run("EnvironmentExists", func(t *testing.T) {
		exists, err := client.EnvironmentExists(ctx, owner, repo, envName)
		require.NoError(t, err)
		assert.True(t, exists, "Environment %q should exist", envName)
	})

	// Step 3: Set variables.
	t.Run("SetVariables", func(t *testing.T) {
		err := client.SetVariable(ctx, owner, repo, envName, "TEST_VAR_1", "value-1")
		require.NoError(t, err, "Failed to set TEST_VAR_1")

		err = client.SetVariable(ctx, owner, repo, envName, "TEST_VAR_2", "value-2")
		require.NoError(t, err, "Failed to set TEST_VAR_2")

		t.Log("Set TEST_VAR_1=value-1, TEST_VAR_2=value-2")
	})

	// Step 4: Read variables back and verify.
	t.Run("GetVariables", func(t *testing.T) {
		vars, err := client.GetVariables(ctx, owner, repo, envName)
		require.NoError(t, err, "Failed to get variables")

		assert.Equal(t, "value-1", vars["TEST_VAR_1"], "TEST_VAR_1 mismatch")
		assert.Equal(t, "value-2", vars["TEST_VAR_2"], "TEST_VAR_2 mismatch")
		t.Logf("Variables verified: %v", vars)
	})

	// Step 5: Update a variable and verify.
	t.Run("UpdateVariable", func(t *testing.T) {
		err := client.SetVariable(ctx, owner, repo, envName, "TEST_VAR_1", "updated-value")
		require.NoError(t, err, "Failed to update TEST_VAR_1")

		vars, err := client.GetVariables(ctx, owner, repo, envName)
		require.NoError(t, err)
		assert.Equal(t, "updated-value", vars["TEST_VAR_1"], "TEST_VAR_1 should be updated")
		t.Log("Variable update verified")
	})

	// Step 6: List environments and verify ours is present.
	t.Run("ListEnvironments", func(t *testing.T) {
		names, err := client.ListEnvironments(ctx, owner, repo)
		require.NoError(t, err)
		assert.Contains(t, names, envName, "Environment list should contain %q", envName)
		t.Logf("Environment %q found in list of %d environments", envName, len(names))
	})

	// Step 7: Delete the environment.
	t.Run("DeleteEnvironment", func(t *testing.T) {
		err := client.DeleteEnvironment(ctx, owner, repo, envName)
		require.NoError(t, err, "Failed to delete environment")

		exists, err := client.EnvironmentExists(ctx, owner, repo, envName)
		require.NoError(t, err)
		assert.False(t, exists, "Environment should not exist after deletion")
		t.Log("Environment deleted and verified gone")
	})
}

// TestIntegration_AWSEnvironmentConfig tests creating an environment with
// AWS-style variables, simulating what the credential service does.
func TestIntegration_AWSEnvironmentConfig(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	owner := os.Getenv("TEST_GITHUB_OWNER")
	repo := os.Getenv("TEST_GITHUB_REPO")

	if token == "" || owner == "" || repo == "" {
		t.Skip("Skipping integration test: GITHUB_TOKEN, TEST_GITHUB_OWNER, and TEST_GITHUB_REPO must be set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := NewClient(StaticTokenSource(token))
	envName := fmt.Sprintf("radius-aws-test-%d", time.Now().Unix())

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = client.DeleteEnvironment(cleanupCtx, owner, repo, envName)
	})

	// Create environment with AWS-style variables.
	err := client.CreateEnvironment(ctx, owner, repo, envName)
	require.NoError(t, err)

	awsVars := map[string]string{
		"AWS_IAM_ROLE_ARN": "arn:aws:iam::123456789012:role/test-role",
		"AWS_REGION":       "us-east-1",
	}

	for key, value := range awsVars {
		err := client.SetVariable(ctx, owner, repo, envName, key, value)
		require.NoError(t, err, "Failed to set %s", key)
	}

	// Read back and verify.
	vars, err := client.GetVariables(ctx, owner, repo, envName)
	require.NoError(t, err)

	for key, expected := range awsVars {
		assert.Equal(t, expected, vars[key], "%s mismatch", key)
	}

	t.Logf("✅ AWS environment %q created with %d variables", envName, len(awsVars))
}
