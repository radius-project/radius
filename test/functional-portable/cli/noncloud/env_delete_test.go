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

package resource_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
)

func Test_EnvDelete(t *testing.T) {
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	// Generate a unique resource group name to avoid conflicts with parallel tests
	uniqueGroupName := fmt.Sprintf("test-env-delete-%d", time.Now().Unix())
	envName := "env-delete-test-env"
	appName := "env-delete-test-app"
	containerA := "env-delete-container-a"
	containerB := "env-delete-container-b"

	// Ensure cleanup even if test fails
	t.Cleanup(func() {
		// Try to delete the test group if it still exists
		// Ignore errors as the group might have been successfully deleted
		_ = cli.GroupDelete(context.Background(), uniqueGroupName, radcli.DeleteOptions{Confirm: true})
	})

	// Create the unique resource group
	t.Logf("Creating resource group: %s", uniqueGroupName)
	err := cli.GroupCreate(ctx, uniqueGroupName)
	require.NoError(t, err, "Failed to create resource group")

	// Get the template file path
	cwd, err := os.Getwd()
	require.NoError(t, err)
	templateFilePath := filepath.Join(cwd, "testdata/corerp-env-delete-test.bicep")

	// Deploy resources to the specific resource group
	t.Logf("Deploying resources to group: %s", uniqueGroupName)
	err = cli.DeployWithGroup(ctx, templateFilePath, "", "", uniqueGroupName, testutil.GetMagpieImage())
	require.NoError(t, err, "Failed to deploy resources to resource group")

	// Validate that resources were created successfully
	// Note: We need to wait a bit for Kubernetes resources to be created
	validation.ValidateObjectsRunning(ctx, t, options.K8sClient, options.DynamicClient, validation.K8sObjectSet{
		Namespaces: map[string][]validation.K8sObject{
			"default-env-delete-test-env-env-delete-test-app": {
				validation.NewK8sPodForResource(appName, containerA),
				validation.NewK8sPodForResource(appName, containerB),
			},
		},
	})

	// Set options for group-scoped operations
	showOpts := radcli.ShowOptions{Group: uniqueGroupName}
	deleteOpts := radcli.DeleteOptions{Group: uniqueGroupName, Confirm: true}

	// Verify environment exists
	t.Logf("Verifying environment exists: %s", envName)
	output, err := cli.EnvShow(ctx, envName, showOpts)
	require.NoError(t, err, "Failed to show environment")
	require.Contains(t, output, envName, "Environment should exist")

	// Verify application exists
	output, err = cli.ApplicationShow(ctx, appName, showOpts)
	require.NoError(t, err, "Failed to show application")
	require.Contains(t, output, appName, "Application should exist")

	// Delete the environment with all its resources
	t.Logf("Deleting environment: %s in group: %s", envName, uniqueGroupName)
	err = cli.EnvDelete(ctx, envName, deleteOpts)
	require.NoError(t, err, "Failed to delete environment with resources")

	// Verify environment is deleted - checking in the specific group
	t.Logf("Verifying environment deletion: %s", envName)
	output, err = cli.EnvShow(ctx, envName, showOpts)
	require.Error(t, err, "Environment should be deleted")
	outputStr := strings.ToLower(output)
	require.Contains(t, outputStr, "not found", "Expected 'not found' in output but got: %s", output)

	// Verify all associated resources are deleted
	output, err = cli.ApplicationShow(ctx, appName, showOpts)
	require.Error(t, err, "Application should be deleted")
	outputStr = strings.ToLower(output)
	require.Contains(t, outputStr, "not found", "Expected 'not found' in output but got: %s", output)

	output, err = cli.ResourceShow(ctx, "Applications.Core/containers", containerA, showOpts)
	require.Error(t, err, "Container A should be deleted")
	outputStr = strings.ToLower(output)
	require.Contains(t, outputStr, "not found", "Expected 'not found' in output but got: %s", output)

	output, err = cli.ResourceShow(ctx, "Applications.Core/containers", containerB, showOpts)
	require.Error(t, err, "Container B should be deleted")
	outputStr = strings.ToLower(output)
	require.Contains(t, outputStr, "not found", "Expected 'not found' in output but got: %s", output)

	t.Logf("Successfully verified deletion of environment %s and all its resources", envName)

	// Test 2: Delete empty environment
	t.Log("Testing deletion of empty environment")
	emptyEnvName := fmt.Sprintf("test-empty-env-%d", time.Now().Unix())
	emptyEnvNamespace := fmt.Sprintf("empty-env-ns-%d", time.Now().Unix())

	// Create an empty environment in the unique group
	t.Logf("Creating empty environment: %s in group: %s", emptyEnvName, uniqueGroupName)
	createOpts := radcli.CreateOptions{
		Namespace: emptyEnvNamespace,
		Group:     uniqueGroupName,
	}
	err = cli.EnvCreate(ctx, emptyEnvName, createOpts)
	require.NoError(t, err, "Failed to create empty environment")

	// Verify empty environment exists
	output, err = cli.EnvShow(ctx, emptyEnvName, showOpts)
	require.NoError(t, err, "Failed to show empty environment")
	require.Contains(t, output, emptyEnvName, "Empty environment should exist")

	// Delete the empty environment
	t.Logf("Deleting empty environment: %s", emptyEnvName)
	err = cli.EnvDelete(ctx, emptyEnvName, deleteOpts)
	require.NoError(t, err, "Failed to delete empty environment")

	// Verify empty environment is deleted
	output, err = cli.EnvShow(ctx, emptyEnvName, showOpts)
	require.Error(t, err, "Empty environment should be deleted")
	outputStr = strings.ToLower(output)
	require.Contains(t, outputStr, "not found", "Expected 'not found' in output but got: %s", output)

	t.Logf("Successfully tested deletion of empty environment %s", emptyEnvName)
}
