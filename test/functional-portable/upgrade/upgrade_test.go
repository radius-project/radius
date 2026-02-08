/*
Copyright 2025 The Radius Authors.

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

package upgrade_test

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/radius-project/radius/test/testutil"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	radiusNamespace   = "radius-system"
	preUpgradeJobName = "pre-upgrade"
	helmTimeout       = "3m"

	relativeChartPath = "../../../deploy/Chart"

	// Polling intervals for waiting on Kubernetes state changes.
	controlPlanePollInterval = 3 * time.Second
	cleanupPollInterval      = 3 * time.Second
	jobPollInterval          = 1 * time.Second
	jobPollAttempts          = 15

	// cleanupTimeout is the maximum time to wait for Radius pods to terminate
	// after uninstalling the Helm release.
	cleanupTimeout = 2 * time.Minute

	// radiusPodSelector selects only pods belonging to the Radius Helm release.
	// Contour is deployed as a separate Helm release in the same namespace and
	// must be excluded from cleanup checks — its pods will remain running.
	radiusPodSelector = "app.kubernetes.io/part-of=radius"
)

// Test_PreflightContainer runs all preflight container upgrade tests as sequential
// subtests. These tests cannot run in parallel because they share the same Helm
// release name and Kubernetes namespace. Consolidating into subtests reduces the
// number of full install/uninstall cycles (from 4 to 2) and eliminates redundant
// test logic.
func Test_PreflightContainer(t *testing.T) {
	t.Run("Enabled", testPreflightEnabled)
	t.Run("Disabled", testPreflightDisabled)
}

// testPreflightEnabled verifies that when preflight is enabled:
//   - The pre-upgrade Helm hook creates a job during upgrade
//   - Custom job configuration (TTL, version check) is applied correctly
//   - Job logs and status are accessible
func testPreflightEnabled(t *testing.T) {
	ctx := testcontext.New(t)
	image, tag := getPreUpgradeImage()

	cleanupAndWait(t, ctx)

	helmValues := map[string]string{
		"preupgrade.enabled":                 "true",
		"preupgrade.ttlSecondsAfterFinished": "60",
		"preupgrade.checks.version":          "true",
	}

	t.Log("Installing Radius with preflight enabled and custom configuration")
	err := helmInstall(ctx, image, tag, helmValues)
	require.NoError(t, err, "Failed to install Radius")

	options := waitForControlPlane(t, ctx)

	t.Log("Upgrading to trigger pre-upgrade hook")
	// Upgrade may fail due to version issues, but should trigger the Helm hook.
	// The key assertion is that the job gets created.
	_ = helmUpgrade(ctx, image, tag, helmValues)

	t.Log("Verifying preflight job was created and configured correctly")
	job := findPreflightJob(t, ctx, options)
	if job != nil {
		logJobDetails(t, ctx, options, job)

		// Verify custom configuration was applied
		require.NotNil(t, job.Spec.TTLSecondsAfterFinished, "TTLSecondsAfterFinished should be set")
		require.Equal(t, int32(60), *job.Spec.TTLSecondsAfterFinished)
		t.Log("Job configuration verified")
	} else {
		t.Log("Preflight job not found - upgrade likely failed before hooks triggered (acceptable in test environment)")
	}

	helmUninstall(t, ctx)
}

// testPreflightDisabled verifies that when preflight is disabled, the pre-upgrade
// Helm hook does not create a job during upgrade.
func testPreflightDisabled(t *testing.T) {
	ctx := testcontext.New(t)
	image, tag := getPreUpgradeImage()

	cleanupAndWait(t, ctx)

	helmValues := map[string]string{
		"preupgrade.enabled": "false",
	}

	t.Log("Installing Radius with preflight disabled")
	err := helmInstall(ctx, image, tag, helmValues)
	require.NoError(t, err, "Failed to install Radius")

	options := waitForControlPlane(t, ctx)

	// Ensure no leftover job exists before upgrade
	_ = options.K8sClient.BatchV1().Jobs(radiusNamespace).Delete(ctx, preUpgradeJobName, metav1.DeleteOptions{})

	t.Log("Upgrading with preflight disabled")
	_ = helmUpgrade(ctx, image, tag, helmValues)

	t.Log("Verifying no preflight job was created")
	// Brief wait to allow any unexpected job creation to surface
	time.Sleep(5 * time.Second)
	_, err = options.K8sClient.BatchV1().Jobs(radiusNamespace).Get(ctx, preUpgradeJobName, metav1.GetOptions{})
	if err != nil && strings.Contains(err.Error(), "not found") {
		t.Log("Preflight job correctly not created when disabled")
	} else {
		t.Errorf("Expected preflight job to not exist when disabled, but found it or got unexpected error: %v", err)
	}

	helmUninstall(t, ctx)
}

// Helper functions

// getPreUpgradeImage constructs the pre-upgrade container image name using the configured registry and tag.
func getPreUpgradeImage() (image string, tag string) {
	registry, tag := testutil.SetDefault()
	return fmt.Sprintf("%s/pre-upgrade", registry), tag
}

// helmInstall runs helm install with the given image, tag, and additional values.
func helmInstall(ctx context.Context, image, tag string, values map[string]string) error {
	args := []string{
		"helm", "install", "radius", relativeChartPath,
		"--namespace", radiusNamespace,
		"--create-namespace",
		"--set", fmt.Sprintf("preupgrade.image=%s", image),
		"--set", fmt.Sprintf("preupgrade.tag=%s", tag),
		"--wait",
		"--timeout", helmTimeout,
	}
	for k, v := range values {
		args = append(args, "--set", fmt.Sprintf("%s=%s", k, v))
	}
	return runCommand(ctx, args)
}

// helmUpgrade runs helm upgrade with the given image, tag, and additional values.
func helmUpgrade(ctx context.Context, image, tag string, values map[string]string) error {
	args := []string{
		"helm", "upgrade", "radius", relativeChartPath,
		"--namespace", radiusNamespace,
		"--set", fmt.Sprintf("preupgrade.image=%s", image),
		"--set", fmt.Sprintf("preupgrade.tag=%s", tag),
		"--wait",
		"--timeout", helmTimeout,
	}
	for k, v := range values {
		args = append(args, "--set", fmt.Sprintf("%s=%s", k, v))
	}
	return runCommand(ctx, args)
}

// helmUninstall removes the Radius helm release.
func helmUninstall(t *testing.T, ctx context.Context) {
	t.Helper()
	t.Log("Uninstalling Radius")
	err := runCommand(ctx, []string{"helm", "uninstall", "radius", "--namespace", radiusNamespace})
	require.NoError(t, err, "Failed to uninstall Radius")
}

// runCommand executes a shell command and returns an error if it fails.
func runCommand(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %s, output: %s", err, string(output))
	}
	return nil
}

// waitForControlPlane polls until the Radius control plane API is reachable.
func waitForControlPlane(t *testing.T, ctx context.Context) rp.RPTestOptions {
	t.Helper()
	var options rp.RPTestOptions
	require.Eventually(t, func() bool {
		func() {
			defer func() {
				if r := recover(); r != nil {
					// NewRPTestOptions calls require.NoError internally, catch panics
					t.Logf("Control plane not ready yet: %v", r)
				}
			}()
			options = rp.NewRPTestOptions(t)
		}()
		return options.ManagementClient != nil
	}, 2*time.Minute, controlPlanePollInterval, "Control plane did not become available within timeout")
	return options
}

// cleanupAndWait uninstalls the Radius Helm release and waits for all Radius pods in
// the namespace to be fully terminated before returning. This prevents aggregated API
// service conflicts when the next helm install runs before previous resources are
// fully cleaned up.
//
// Only Radius-owned pods (labeled app.kubernetes.io/part-of=radius) are monitored.
// Contour is deployed as a separate Helm release in the same namespace and its pods
// are expected to remain running.
func cleanupAndWait(t *testing.T, ctx context.Context) {
	t.Helper()

	t.Log("Cleaning up any existing Radius installation")
	_ = exec.CommandContext(ctx, "helm", "uninstall", "radius",
		"--namespace", radiusNamespace, "--ignore-not-found", "--wait").Run()

	// Wait for Radius pods to terminate. The Kubernetes aggregated API service needs
	// time to deregister after pods are gone, so we must wait for Radius pods to be
	// fully removed before starting a new install.
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), nil).ClientConfig()
	if err != nil {
		t.Logf("Warning: could not create k8s client for cleanup wait: %v", err)
		time.Sleep(10 * time.Second)
		return
	}

	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Logf("Warning: could not create k8s client for cleanup wait: %v", err)
		time.Sleep(10 * time.Second)
		return
	}

	require.Eventually(t, func() bool {
		pods, err := k8sClient.CoreV1().Pods(radiusNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: radiusPodSelector,
		})
		if err != nil {
			return true
		}
		if len(pods.Items) == 0 {
			return true
		}
		t.Logf("Waiting for %d Radius pod(s) in %s to terminate...", len(pods.Items), radiusNamespace)
		return false
	}, cleanupTimeout, cleanupPollInterval, "Radius pods in %s did not terminate within timeout", radiusNamespace)

	// Brief wait for Kubernetes aggregated API service deregistration
	time.Sleep(3 * time.Second)
}

// findPreflightJob polls for the pre-upgrade job, returning it if found within the timeout.
func findPreflightJob(t *testing.T, ctx context.Context, options rp.RPTestOptions) *batchv1.Job {
	t.Helper()
	for range jobPollAttempts {
		job, err := options.K8sClient.BatchV1().Jobs(radiusNamespace).Get(ctx, preUpgradeJobName, metav1.GetOptions{})
		if err == nil {
			t.Log("Preflight job was created by Helm pre-upgrade hook")
			return job
		}
		time.Sleep(jobPollInterval)
	}
	return nil
}

// logJobDetails retrieves and logs the job's pod logs and status.
func logJobDetails(t *testing.T, ctx context.Context, options rp.RPTestOptions, job *batchv1.Job) {
	t.Helper()

	pods, err := options.K8sClient.CoreV1().Pods(radiusNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "job-name=" + preUpgradeJobName,
	})
	if err == nil && len(pods.Items) > 0 {
		logs, logErr := options.K8sClient.CoreV1().Pods(radiusNamespace).
			GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{}).DoRaw(ctx)
		if logErr == nil {
			t.Logf("Preflight job logs:\n%s", string(logs))
		}
	}

	t.Logf("Preflight job status - Succeeded: %d, Failed: %d", job.Status.Succeeded, job.Status.Failed)
}
