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
	"time"

	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
)

const (
	// groupDeleteTimeout bounds the deploy and delete steps. The bug this test guards against
	// previously manifested as a hang, so every step needs a deadline; without one a regression
	// would stall the suite instead of failing it.
	groupDeleteTimeout = 20 * time.Minute

	// groupDeleteCleanupTimeout bounds the best-effort cleanup. It is deliberately independent of
	// the test context, which is already cancelled by the time cleanup runs.
	groupDeleteCleanupTimeout = 10 * time.Minute
)

// Test_GroupDelete_RecipeResources is the regression test for radius-project/radius#12469.
//
// A resource group containing recipe-driven resources alongside the environment and recipe pack
// those recipes resolve must delete cleanly. Deleting the group's contents in one unordered wave
// removes the recipe pack before the containers that need it to tear themselves down, which leaves
// the containers in a failed state and the group undeleted.
func Test_GroupDelete_RecipeResources(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), groupDeleteTimeout)
	t.Cleanup(cancel)

	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	// Both the group and the namespace must be unique: namespaces are unique across the plane, and
	// the suite runs tests in parallel.
	unique := time.Now().Unix()
	groupName := fmt.Sprintf("test-group-delete-recipe-%d", unique)
	namespace := fmt.Sprintf("group-delete-recipe-%d", unique)

	envName := "group-delete-recipe-env"
	appName := "group-delete-recipe-app"
	packName := "group-delete-recipe-pack"
	containerA := "group-delete-recipe-container-a"
	containerB := "group-delete-recipe-container-b"

	t.Cleanup(func() {
		// Best-effort: the group is expected to be gone already. Errors are ignored because a
		// successful test leaves nothing to delete.
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), groupDeleteCleanupTimeout) //nolint:usetesting
		defer cleanupCancel()

		_ = cli.GroupDelete(cleanupCtx, groupName, radcli.DeleteOptions{Confirm: true})
	})

	t.Logf("Creating resource group %s and namespace %s", groupName, namespace)
	require.NoError(t, cli.GroupCreate(ctx, groupName), "failed to create resource group")
	createKubernetesNamespace(ctx, t, options, namespace)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	templateFilePath := filepath.Join(cwd, "testdata/corerp-group-delete-recipe-resources.bicep")

	t.Logf("Deploying recipe-driven resources into group %s", groupName)
	err = cli.DeployWithGroup(ctx, templateFilePath, "", "", groupName,
		testutil.GetMagpieImage(),
		"recipeTag=edge",
		fmt.Sprintf("namespace=%s", namespace),
	)
	require.NoError(t, err, "failed to deploy recipe-driven resources")

	validation.ValidateObjectsRunning(ctx, t, options.K8sClient, options.DynamicClient, validation.K8sObjectSet{
		Namespaces: map[string][]validation.K8sObject{
			namespace: {
				validation.NewK8sPodForResource(appName, containerA),
				validation.NewK8sPodForResource(appName, containerB),
			},
		},
	})

	// Positive control. The post-delete assertions below are only meaningful if this same query
	// can see the resources while they exist.
	listed, err := cli.ResourceListInResourceGroup(ctx, groupName)
	require.NoError(t, err, "failed to list resources in group %s", groupName)
	for _, name := range []string{envName, appName, packName, containerA, containerB} {
		require.Containsf(t, listed, name,
			"expected %s to be listed in group %s before deletion, got: %s", name, groupName, listed)
	}

	t.Logf("Deleting resource group %s", groupName)
	err = cli.GroupDelete(ctx, groupName, radcli.DeleteOptions{Confirm: true})
	require.NoError(t, err, "failed to delete resource group containing recipe-driven resources")

	output, err := cli.GroupShow(ctx, groupName)
	require.Errorf(t, err, "resource group %s should have been deleted, but it was found: %s", groupName, output)

	// The group record being gone is not sufficient: the defect this guards against deleted the
	// record while leaving resources behind.
	//
	// Those leftovers cannot be observed by showing them directly, because UCP resolves the
	// resource group before the resource and answers "not found" for anything under a group that
	// does not exist -- so every such assertion would pass whether or not the resource survived.
	// Recreating the group at the same scope makes any surviving record addressable again, which
	// is the only way to tell an emptied group apart from a silently orphaned one.
	require.NoError(t, cli.GroupCreate(ctx, groupName), "failed to recreate resource group for the orphan check")

	remaining, err := cli.ResourceListInResourceGroup(ctx, groupName)
	require.NoError(t, err, "failed to list resources in recreated group %s", groupName)
	for _, name := range []string{envName, appName, packName, containerA, containerB} {
		require.NotContainsf(t, remaining, name,
			"%s survived the deletion of group %s and was orphaned, got: %s", name, groupName, remaining)
	}

	validation.ValidateNoPodsInApplication(ctx, t, options.K8sClient, namespace, appName)

	t.Logf("Verified deletion of resource group %s and all of its resources", groupName)
}
