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

func Test_GroupDelete(t *testing.T) {
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	// Generate a unique resource group name to avoid conflicts with parallel tests
	uniqueGroupName := fmt.Sprintf("test-group-delete-%d", time.Now().Unix())
	envName := "group-delete-test-env"
	appName := "group-delete-test-app"
	containerA := "group-delete-container-a"
	containerB := "group-delete-container-b"

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
	templateFilePath := filepath.Join(cwd, "testdata/corerp-group-delete-test.bicep")

	// Deploy resources to the specific resource group
	t.Logf("Deploying resources to group: %s", uniqueGroupName)
	err = cli.DeployWithGroup(ctx, templateFilePath, "", "", uniqueGroupName, testutil.GetMagpieImage())
	require.NoError(t, err, "Failed to deploy resources to resource group")

	// Validate that resources were created successfully
	// Note: We need to wait a bit for Kubernetes resources to be created
	validation.ValidateObjectsRunning(ctx, t, options.K8sClient, options.DynamicClient, validation.K8sObjectSet{
		Namespaces: map[string][]validation.K8sObject{
			"default-group-delete-test-env-group-delete-test-app": {
				validation.NewK8sPodForResource(appName, containerA),
				validation.NewK8sPodForResource(appName, containerB),
			},
		},
	})

	// Delete the resource group with all its resources
	t.Logf("Deleting resource group: %s", uniqueGroupName)
	err = cli.GroupDelete(ctx, uniqueGroupName, radcli.DeleteOptions{Confirm: true})
	require.NoError(t, err, "Failed to delete resource group with resources")

	// Verify group is deleted
	t.Logf("Verifying resource group deletion: %s", uniqueGroupName)
	output, err := cli.GroupShow(ctx, uniqueGroupName)
	require.Error(t, err, "Group should be deleted")
	outputStr := strings.ToLower(output)
	require.Contains(t, outputStr, "not found", "Expected 'not found' in output but got: %s", output)

	// Verify all resources are deleted - checking in the specific group
	opts := radcli.ShowOptions{Group: uniqueGroupName}

	output, err = cli.ApplicationShow(ctx, appName, opts)
	require.Error(t, err, "Application should be deleted")
	outputStr = strings.ToLower(output)
	require.Contains(t, outputStr, "not found", "Expected 'not found' in output but got: %s", output)

	output, err = cli.EnvShow(ctx, envName, opts)
	require.Error(t, err, "Environment should be deleted")
	outputStr = strings.ToLower(output)
	require.Contains(t, outputStr, "not found", "Expected 'not found' in output but got: %s", output)

	output, err = cli.ResourceShow(ctx, "Applications.Core/containers", containerA, opts)
	require.Error(t, err, "Container A should be deleted")
	outputStr = strings.ToLower(output)
	require.Contains(t, outputStr, "not found", "Expected 'not found' in output but got: %s", output)

	output, err = cli.ResourceShow(ctx, "Applications.Core/containers", containerB, opts)
	require.Error(t, err, "Container B should be deleted")
	outputStr = strings.ToLower(output)
	require.Contains(t, outputStr, "not found", "Expected 'not found' in output but got: %s", output)

	t.Logf("Successfully verified deletion of resource group %s and all its resources", uniqueGroupName)
}
