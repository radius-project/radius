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
// The test is destructive: it uninstalls and reinstalls Radius (`--purge`) to simulate the
// ephemeral control plane that Repo Radius runs on. Because of that it runs on its own dedicated
// cluster in CI (the `statestore-noncloud` leg), never alongside other functional tests, and
// drives its own install/uninstall instead of relying on the shared "Install Radius" CI step.
package statestore

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/functional-portable/corerp"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/testutil"
)

const (
	stateNamespace = "radius-system"
	secretPrefix   = "tfstate-default-"

	// resourceGroup is the Radius resource group the test deploys into. It must match the group
	// segment of resourceID below so the Terraform state secret name resolves correctly.
	resourceGroup = "kind-radius"

	// relativeChartPath points at the in-repo Helm chart so the test installs the build under test.
	// The path is relative to this test package directory
	// (test/functional-portable/statestore/noncloud), which is four levels below the repo root.
	relativeChartPath = "../../../../deploy/Chart"

	// redisRecipeTemplate is the Terraform recipe fixture shared with the corerp recipe tests.
	redisRecipeTemplate = "../../corerp/noncloud/resources/testdata/corerp-resources-terraform-redis.bicep"

	// controlPlaneTimeout is how long to wait for the control plane API to become available after an
	// install. It is generous because the UCP aggregated APIService may briefly return 503 while the
	// pods roll. (Lesson from the flaky upgrade test, PR #12245.)
	controlPlaneTimeout      = 5 * time.Minute
	controlPlanePollInterval = 5 * time.Second

	// apiServiceDeregistrationTimeout bounds the wait for the Radius aggregated APIService to
	// deregister after an uninstall. Reinstalling while `api.ucp.dev/v1alpha3` is still registered
	// makes the API server return 503, which flakes the next install. (Lesson from PR #12245.)
	apiServiceDeregistrationTimeout  = 60 * time.Second
	apiServiceDeregistrationInterval = 2 * time.Second
	radiusAPIGroupVersion            = "api.ucp.dev/v1alpha3"

	// podTerminationTimeout bounds the wait for Radius pods to disappear after an uninstall.
	podTerminationTimeout = 2 * time.Minute
	podTerminationPoll    = 5 * time.Second
	radiusPodSelector     = "app.kubernetes.io/part-of=radius"
)

// installRadius installs Radius with the PostgreSQL state backend enabled, using the images and
// chart of the build under test. In CI the registry/tag come from DOCKER_REGISTRY/REL_VERSION and
// the secure local registry's CA is supplied via RADIUS_REGISTRY_CERT_FILE; locally it falls back
// to the public images.
func installRadius(ctx context.Context, t *testing.T, cli *radcli.CLI) {
	t.Helper()
	registry, tag := testutil.SetDefault()

	args := []string{
		"install", "kubernetes",
		"--chart", relativeChartPath,
		"--set", fmt.Sprintf("rp.image=%s/applications-rp,rp.tag=%s", registry, tag),
		"--set", fmt.Sprintf("dynamicrp.image=%s/dynamic-rp,dynamicrp.tag=%s", registry, tag),
		"--set", fmt.Sprintf("controller.image=%s/controller,controller.tag=%s", registry, tag),
		"--set", fmt.Sprintf("ucp.image=%s/ucpd,ucp.tag=%s", registry, tag),
		"--set", fmt.Sprintf("bicep.image=%s/bicep,bicep.tag=%s", registry, tag),
		"--set", fmt.Sprintf("preupgrade.image=%s/pre-upgrade,preupgrade.tag=%s", registry, tag),
		"--set", "database.enabled=true",
	}
	if deImage := os.Getenv("DE_IMAGE"); deImage != "" {
		args = append(args, "--set", fmt.Sprintf("de.image=%s,de.tag=%s", deImage, os.Getenv("DE_TAG")))
	}
	if cert := os.Getenv("RADIUS_REGISTRY_CERT_FILE"); cert != "" {
		args = append(args, "--set-file", "global.rootCA.cert="+cert)
	}

	out, err := cli.RunCommand(ctx, args)
	require.NoErrorf(t, err, "rad install failed: %s", out)
	waitForControlPlane(t, ctx)
}

// uninstallRadius removes Radius and its state so the next install starts from an empty control
// plane, simulating an ephemeral teardown. It then waits for the Radius pods to terminate and the
// aggregated APIService to deregister so a subsequent install does not race the teardown.
func uninstallRadius(ctx context.Context, t *testing.T, cli *radcli.CLI) {
	t.Helper()
	out, err := cli.RunCommand(ctx, []string{"uninstall", "kubernetes", "--purge"})
	require.NoErrorf(t, err, "rad uninstall failed: %s", out)
	waitForCleanTeardown(t, ctx)
}

// waitForControlPlane polls until the Radius control plane API is reachable, treating transient
// errors (including 503 from the aggregated APIService while pods roll) as retryable.
func waitForControlPlane(t *testing.T, _ context.Context) {
	t.Helper()
	require.Eventually(t, func() bool {
		ready := false
		func() {
			// NewRPTestOptions calls require/panic internally on a not-yet-ready control plane;
			// catch that and treat it as "retry".
			defer func() { _ = recover() }()
			opts := rp.NewRPTestOptions(t)
			ready = opts.ManagementClient != nil
		}()
		return ready
	}, controlPlaneTimeout, controlPlanePollInterval, "control plane did not become available within timeout")
}

// waitForCleanTeardown waits for Radius pods to terminate and the aggregated APIService to
// deregister after an uninstall, so the next install does not race a half-torn-down control plane.
func waitForCleanTeardown(t *testing.T, ctx context.Context) {
	t.Helper()
	k8s := test.NewTestOptions(t).K8sClient

	require.Eventually(t, func() bool {
		pods, err := k8s.CoreV1().Pods(stateNamespace).List(ctx, metav1.ListOptions{LabelSelector: radiusPodSelector})
		if err != nil {
			t.Logf("waiting to list pods: %v", err)
			return false
		}
		if len(pods.Items) == 0 {
			return true
		}
		t.Logf("waiting for %d Radius pod(s) to terminate...", len(pods.Items))
		return false
	}, podTerminationTimeout, podTerminationPoll, "Radius pods did not terminate within timeout")

	// A 503 from the aggregated APIService means it is still registered but its backend is gone;
	// poll discovery until the Radius API group is no longer served.
	require.Eventually(t, func() bool {
		_, resources, err := k8s.Discovery().ServerGroupsAndResources()
		if err != nil {
			// Partial results are expected mid-deregistration; inspect what we got.
			t.Logf("discovery returned partial results (expected during deregistration): %v", err)
		}
		for _, rl := range resources {
			if rl != nil && rl.GroupVersion == radiusAPIGroupVersion {
				t.Log("Radius aggregated APIService still registered, waiting...")
				return false
			}
		}
		return true
	}, apiServiceDeregistrationTimeout, apiServiceDeregistrationInterval, "aggregated APIService did not deregister within timeout")
}

// Test_StateStore_ShutdownStartup_TerraformCrossDeploy exercises every state path:
// install, deploy a Terraform resource, shut down (backup), tear down, start up (restore), then
// deploy an update to the same resource.
func Test_StateStore_ShutdownStartup_TerraformCrossDeploy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	cli := radcli.NewCLI(t, "")

	appName := "statestore-tf-redis-app"
	envName := "statestore-tf-redis-env"
	resourceName := "statestore-tf-redis"
	redisCacheName := "statestore-redis"

	resourceID := "/planes/radius/local/resourcegroups/" + resourceGroup + "/providers/Applications.Core/extenders/" + resourceName
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

	// 1. Fresh install with the PostgreSQL state backend.
	installRadius(ctx, t, cli)
	t.Cleanup(func() { uninstallRadius(context.Background(), t, cli) })

	// Create the workspace and resource group the test deploys into (the shared CI "Install Radius"
	// step is skipped for this leg, so the test owns this setup).
	out, err := cli.RunCommand(ctx, []string{"workspace", "create", "kubernetes", "--force"})
	require.NoErrorf(t, err, "rad workspace create failed: %s", out)
	out, err = cli.RunCommand(ctx, []string{"group", "create", resourceGroup})
	require.NoErrorf(t, err, "rad group create failed: %s", out)
	out, err = cli.RunCommand(ctx, []string{"group", "switch", resourceGroup})
	require.NoErrorf(t, err, "rad group switch failed: %s", out)

	// 2. Deploy the Terraform-backed resource. This creates control-plane state and a Terraform
	//    state Secret.
	deploy()
	require.True(t, secretExists(), "Terraform state secret should exist after the first deploy")

	// 3. Back up all durable state.
	out, err = cli.RunCommand(ctx, []string{"shutdown"})
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

	// At least one Terraform state Secret must still back the resource. The backend may shard large
	// state across multiple tfstate-* Secrets, so assert presence rather than an exact count.
	secrets, err := k8s.CoreV1().Secrets(stateNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "tfstate=true",
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(secrets.Items), 1, "at least one Terraform state secret should exist after the cross-deploy")
}
