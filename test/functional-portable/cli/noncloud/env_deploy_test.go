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
	"testing"

	"github.com/google/uuid"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/stretchr/testify/require"
)

// Test_DeployEnvironmentTemplate verifies that an environment can be created
// by deploying a Bicep template that defines an environment resource, without
// pre-creating an environment or specifying the environment name via the
// --environment flag.
func Test_DeployEnvironmentTemplate(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	options := rp.NewRPTestOptions(t)
	cli := newCLIWithoutDefaultEnvironment(t, options)

	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Generate a unique suffix so the resource group and Kubernetes namespace do
	// not collide with parallel, repeated, or stale runs against the same cluster.
	uniqueSuffix := uuid.New().String()
	uniqueGroupName := fmt.Sprintf("test-deploy-env-group-%s", uniqueSuffix)
	envName := "test-deploy-env"
	envNamespace := fmt.Sprintf("default-test-deploy-env-%s", uniqueSuffix)

	// Ensure cleanup even if test fails
	t.Cleanup(func() {
		deleteKubernetesNamespace(context.Background(), t, options.K8sClient, envNamespace)
	})
	t.Cleanup(func() {
		// Try to delete the test group if it still exists
		// Ignore errors as the group might have been successfully deleted
		_ = cli.GroupDelete(context.Background(), uniqueGroupName, radcli.DeleteOptions{Confirm: true})
	})

	// Create the unique resource group
	t.Logf("Creating resource group: %s", uniqueGroupName)
	err = cli.GroupCreate(ctx, uniqueGroupName)
	require.NoError(t, err, "Failed to create resource group")

	createKubernetesNamespace(ctx, t, options.K8sClient, envNamespace)

	// Get the template file path
	templateFilePath := filepath.Join(cwd, "testdata/corerp-env-deploy-test.bicep")

	// Deploy the environment template without specifying --environment flag
	t.Logf("Deploying environment template to group: %s without --environment flag", uniqueGroupName)
	err = cli.DeployWithGroup(ctx, templateFilePath, "", "", uniqueGroupName, "envNamespace="+envNamespace)
	require.NoError(t, err, "Failed to deploy environment template")

	// Verify environment was created successfully
	t.Logf("Verifying environment was created: %s", envName)
	showOpts := radcli.ShowOptions{Group: uniqueGroupName}
	output, err := cli.ResourceShow(ctx, "Radius.Core/environments", envName, showOpts)
	require.NoError(t, err, "Failed to show environment %s", envName)
	require.Contains(t, output, envName, "Environment %s should exist", envName)

	t.Logf("Successfully verified environment %s was created from template without --environment flag", envName)

	// Clean up
	t.Logf("Cleaning up: deleting group %s", uniqueGroupName)
	deleteOpts := radcli.DeleteOptions{Group: uniqueGroupName, Confirm: true}
	err = cli.GroupDelete(ctx, uniqueGroupName, deleteOpts)
	require.NoError(t, err, "Failed to delete resource group")
}
