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
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/stretchr/testify/require"
)

// Test_EnvDelete covers group-scoped env create, show, and delete. Cascade is covered by
// Test_Environment_CascadeDelete.
func Test_EnvDelete(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	// Generate unique names to avoid conflicts with parallel tests
	uniqueGroupName := fmt.Sprintf("test-env-delete-%d", time.Now().Unix())
	envName := fmt.Sprintf("test-empty-env-%d", time.Now().Unix())
	envNamespace := fmt.Sprintf("empty-env-ns-%d", time.Now().Unix())

	// Ensure cleanup even if test fails
	t.Cleanup(func() {
		// Try to delete the test group if it still exists
		// Ignore errors as the group might have been successfully deleted.
		// Use a fresh context because t.Context() is cancelled before cleanup runs.
		_ = cli.GroupDelete(context.Background(), uniqueGroupName, radcli.DeleteOptions{Confirm: true}) //nolint:usetesting
		deleteKubernetesNamespace(context.Background(), t, options, envNamespace)                       //nolint:usetesting
	})

	// Radius.Core requires the environment's namespace to exist.
	createKubernetesNamespace(ctx, t, options, envNamespace)

	t.Logf("Creating resource group: %s", uniqueGroupName)
	err := cli.GroupCreate(ctx, uniqueGroupName)
	require.NoError(t, err, "Failed to create resource group")

	showOpts := radcli.ShowOptions{Group: uniqueGroupName}
	deleteOpts := radcli.DeleteOptions{Group: uniqueGroupName, Confirm: true}
	createOpts := radcli.CreateOptions{
		Namespace: envNamespace,
		Group:     uniqueGroupName,
	}

	t.Logf("Creating environment: %s in group: %s", envName, uniqueGroupName)
	err = cli.EnvCreate(ctx, envName, createOpts)
	require.NoError(t, err, "Failed to create environment")

	output, err := cli.EnvShow(ctx, envName, showOpts)
	require.NoError(t, err, "Failed to show environment")
	require.Contains(t, output, envName, "Environment should exist")

	t.Logf("Deleting environment: %s", envName)
	err = cli.EnvDelete(ctx, envName, deleteOpts)
	require.NoError(t, err, "Failed to delete environment")

	output, err = cli.EnvShow(ctx, envName, showOpts)
	require.Error(t, err, "Environment should be deleted")
	require.Contains(t, strings.ToLower(output), "does not exist", "Expected 'does not exist' in output but got: %s", output)

	t.Logf("Successfully tested deletion of environment %s", envName)
}
