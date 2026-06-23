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

// Package statestore contains the end-to-end lifecycle test for `rad shutdown` / `rad startup`.
//
// Unlike the standard functional tests (which assume an already-running Radius install), this test
// installs Radius with a PostgreSQL state backend, deploys a Terraform-backed resource, backs up
// and restores all durable state across a full uninstall/reinstall cycle, and then deploys an
// UPDATE to the same resource. The update is the path that fails when Terraform state is lost: it
// proves that both the control-plane databases and the Terraform state Secrets survived the
// teardown.
//
// The test is destructive (it uninstalls and reinstalls Radius on the target cluster) and requires
// a real cluster plus the Terraform recipe module server, so it does not run as part of the normal
// functional suite. It is skipped unless RADIUS_STATE_E2E is set to a truthy value.
package statestore

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/functional-portable/corerp"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/testutil"
)

const (
	stateNamespace = "radius-system"
	secretPrefix   = "tfstate-default-"

	// redisRecipeTemplate is the Terraform recipe fixture shared with the corerp recipe tests.
	redisRecipeTemplate = "../../corerp/noncloud/resources/testdata/corerp-resources-terraform-redis.bicep"
)

// shouldRun reports whether the destructive lifecycle test has been opted into.
func shouldRun(t *testing.T) {
	t.Helper()
	v, _ := strconv.ParseBool(os.Getenv("RADIUS_STATE_E2E"))
	if !v {
		t.Skip("set RADIUS_STATE_E2E=1 to run the destructive rad startup/shutdown lifecycle test")
	}
}

// installRadius installs Radius with the PostgreSQL state backend enabled.
func installRadius(ctx context.Context, t *testing.T, cli *radcli.CLI) {
	t.Helper()
	out, err := cli.RunCommand(ctx, []string{"install", "kubernetes", "--set", "database.enabled=true"})
	require.NoErrorf(t, err, "rad install failed: %s", out)
}

// uninstallRadius removes Radius and its state so the next install starts from an empty control
// plane, simulating an ephemeral teardown.
func uninstallRadius(ctx context.Context, t *testing.T, cli *radcli.CLI) {
	t.Helper()
	out, err := cli.RunCommand(ctx, []string{"uninstall", "kubernetes", "--purge"})
	require.NoErrorf(t, err, "rad uninstall failed: %s", out)
}

// Test_StateStore_ShutdownStartup_TerraformCrossDeploy exercises every state path:
// install, deploy a Terraform resource, shut down (backup), tear down, start up (restore), then
// deploy an update to the same resource.
func Test_StateStore_ShutdownStartup_TerraformCrossDeploy(t *testing.T) {
	shouldRun(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	cli := radcli.NewCLI(t, "")

	appName := "statestore-tf-redis-app"
	envName := "statestore-tf-redis-env"
	resourceName := "statestore-tf-redis"
	redisCacheName := "statestore-redis"

	resourceID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/" + resourceName
	secretSuffix, err := corerp.GetSecretSuffix(resourceID, envName, appName)
	require.NoError(t, err)
	secretName := secretPrefix + secretSuffix

	k8s := test.NewTestOptions(t).K8sClient

	deploy := func() {
		out, derr := cli.RunCommand(ctx, []string{
			"deploy", redisRecipeTemplate,
			"--parameters", testutil.GetTerraformRecipeModuleServerURL(),
			"--parameters", "appName=" + appName,
			"--parameters", "envName=" + envName,
			"--parameters", "resourceName=" + resourceName,
			"--parameters", "redisCacheName=" + redisCacheName,
		})
		require.NoErrorf(t, derr, "rad deploy failed: %s", out)
	}

	secretExists := func() bool {
		_, getErr := k8s.CoreV1().Secrets(stateNamespace).Get(ctx, secretName, metav1.GetOptions{})
		return getErr == nil
	}

	// 1. Fresh install with PostgreSQL state backend.
	installRadius(ctx, t, cli)
	t.Cleanup(func() { uninstallRadius(context.Background(), t, cli) })

	// 2. Deploy the Terraform-backed resource. This creates control-plane state and a Terraform
	//    state Secret.
	deploy()
	require.True(t, secretExists(), "Terraform state secret should exist after the first deploy")

	// 3. Back up all durable state.
	out, err := cli.RunCommand(ctx, []string{"shutdown"})
	require.NoErrorf(t, err, "rad shutdown failed: %s", out)

	// 4. Tear the control plane down completely (ephemeral teardown).
	uninstallRadius(ctx, t, cli)

	// 5. Reinstall onto a fresh, empty control plane.
	installRadius(ctx, t, cli)
	require.False(t, secretExists(), "Terraform state secret must be gone after reinstall (teardown was real)")

	// 6. Restore the saved state.
	out, err = cli.RunCommand(ctx, []string{"startup"})
	require.NoErrorf(t, err, "rad startup failed: %s", out)

	// 7. Both stores must be restored: the Terraform state Secret is back, and the control-plane
	//    resource is queryable again.
	require.True(t, secretExists(), "Terraform state secret should be restored by rad startup")
	resourceShow, err := cli.RunCommand(ctx, []string{"resource", "show", "Applications.Core/extenders", resourceName})
	require.NoErrorf(t, err, "resource should be restored into the control plane: %s", resourceShow)

	// 8. Cross-deploy: deploy an update to the same resource. With Terraform state restored this
	//    plans incrementally and succeeds; without it, Terraform would plan from an empty backend
	//    and either error or orphan cloud resources.
	deploy()

	// The same single Terraform state Secret must still back the resource (no duplicate created).
	secrets, err := k8s.CoreV1().Secrets(stateNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "tfstate=true",
	})
	require.NoError(t, err)
	require.Len(t, secrets.Items, 1, "exactly one Terraform state secret should exist after the cross-deploy")
}
