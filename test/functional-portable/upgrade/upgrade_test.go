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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/testutil"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	radiusNamespace   = "radius-system"
	helmReleaseName   = "radius"
	preUpgradeJobName = "pre-upgrade"
	helmTimeout       = "5m"

	relativeChartPath = "../../../deploy/Chart"

	// helmStatusDeployed is the Helm release status that means the last operation
	// succeeded and the release is ready to be upgraded again.
	helmStatusDeployed = "deployed"

	// preflightJobTTLSeconds is the ttlSecondsAfterFinished the test configures on the
	// pre-upgrade Job and then asserts was applied.
	preflightJobTTLSeconds = 60

	// Polling intervals for waiting on Kubernetes state changes.
	cleanupPollInterval = 3 * time.Second
	jobPollInterval     = 1 * time.Second
	jobPollAttempts     = 15

	// jobDeletionTimeout is the maximum time to wait for a deleted pre-upgrade Job to
	// disappear from the API server.
	jobDeletionTimeout = 60 * time.Second

	// cleanupTimeout is the maximum time to wait for Radius pods to terminate
	// after uninstalling the Helm release.
	cleanupTimeout = 2 * time.Minute

	// apiServiceDeregistrationTimeout is the maximum time to wait for the
	// Kubernetes aggregated API service to deregister after Radius pods terminate.
	apiServiceDeregistrationTimeout = 30 * time.Second

	// apiServiceDeregistrationInterval is the polling interval for checking
	// API service deregistration.
	apiServiceDeregistrationInterval = 2 * time.Second

	// radiusPodSelector selects only pods belonging to the Radius Helm release.
	// Contour is deployed as a separate Helm release in the same namespace and
	// must be excluded from cleanup checks — its pods will remain running.
	radiusPodSelector = "app.kubernetes.io/part-of=radius"
)

// Test_PreflightContainer exercises the chart's pre-upgrade Helm hook.
//
// The hook only runs on helm upgrade, so installing Radius is setup cost rather than the
// subject of the test. The parent installs once with the hook enabled and uninstalls
// once; the chart's post-install hook guarantees the aggregated API is serving before
// the test client is constructed. The subtests then run in sequence against that single
// release, upgrading first with the hook enabled and then again with it disabled. They
// share a Helm release name and a Kubernetes namespace and so must not run in parallel.
func Test_PreflightContainer(t *testing.T) {
	ctx := t.Context()
	image, tag := getPreUpgradeImage()

	k8sClient, err := newKubernetesClient()
	require.NoError(t, err, "Failed to create Kubernetes client")

	cleanupAndWait(t, ctx, k8sClient)

	t.Log("Installing Radius with preflight enabled and custom configuration")
	require.NoError(t, helmInstall(ctx, image, tag, preflightEnabledValues()), "Failed to install Radius")

	// t.Context is canceled before cleanup functions run, so the uninstall needs a
	// context that outlives it.
	t.Cleanup(func() { helmUninstall(t, context.WithoutCancel(ctx)) })

	// The chart's post-install hook verifies the aggregated API through
	// kube-apiserver before Helm returns, so no test-specific readiness wait is
	// needed here.
	release := preflightRelease{options: rp.NewRPTestOptions(t), image: image, tag: tag}

	// The subtests are given the parent's context rather than their own so the shared
	// release outlives any single subtest.
	t.Run("Enabled", func(t *testing.T) { testPreflightEnabled(t, ctx, release) })
	t.Run("Disabled", func(t *testing.T) { testPreflightDisabled(t, ctx, release) })
}

// preflightRelease is the state the preflight subtests share: the single Helm release
// they upgrade in sequence and the clients used to observe it.
type preflightRelease struct {
	options rp.RPTestOptions
	image   string
	tag     string
}

// preflightEnabledValues returns the Helm values that enable the pre-upgrade hook with
// the Job configuration the Enabled subtest asserts on.
func preflightEnabledValues() map[string]string {
	return map[string]string{
		"preupgrade.enabled":                 "true",
		"preupgrade.ttlSecondsAfterFinished": strconv.Itoa(preflightJobTTLSeconds),
		"preupgrade.checks.version":          "true",
	}
}

// testPreflightEnabled verifies that when preflight is enabled:
//   - The pre-upgrade Helm hook creates a job during upgrade
//   - Custom job configuration (TTL, version check) is applied correctly
//   - Job logs and status are accessible
func testPreflightEnabled(t *testing.T, ctx context.Context, release preflightRelease) {
	t.Log("Upgrading to trigger pre-upgrade hook")
	// The upgrade itself is allowed to fail: the preflight checks may legitimately
	// reject it. The assertion is that the hook created the job.
	if err := helmUpgrade(ctx, release.image, release.tag, preflightEnabledValues()); err != nil {
		t.Logf("Upgrade with preflight enabled failed, which is expected when the preflight checks reject it: %v", err)
	}

	t.Log("Verifying preflight job was created and configured correctly")
	job := findPreflightJob(t, ctx, release.options)
	require.NotNil(t, job, "Preflight job not found - upgrade likely failed before hooks triggered")

	logJobDetails(t, ctx, release.options, job)

	require.NotNil(t, job.Spec.TTLSecondsAfterFinished, "TTLSecondsAfterFinished should be set")
	require.Equal(t, int32(preflightJobTTLSeconds), *job.Spec.TTLSecondsAfterFinished)
	t.Log("Job configuration verified")
}

// testPreflightDisabled verifies that upgrading with preflight disabled does not create a
// pre-upgrade job.
//
// It upgrades the release the Enabled subtest left behind, so it first restores that
// release to a state Helm will upgrade and removes the job from the previous subtest.
func testPreflightDisabled(t *testing.T, ctx context.Context, release preflightRelease) {
	requireDeployedRelease(t, ctx)
	deletePreflightJob(t, ctx, release.options)

	t.Log("Upgrading with preflight disabled")
	// With the hook disabled there is no job to reject the upgrade, so it must succeed.
	// Otherwise the assertion below would pass simply because no upgrade ran.
	err := helmUpgrade(ctx, release.image, release.tag, map[string]string{"preupgrade.enabled": "false"})
	require.NoError(t, err, "Upgrade with preflight disabled should succeed")

	t.Log("Verifying no preflight job was created")
	// Poll several times to confirm no job was created. The helm upgrade --wait
	// flag ensures the upgrade is fully complete before returning, so if a job
	// was going to be created it would exist by now. We poll briefly to be safe.
	for i := range jobPollAttempts {
		_, err := release.options.K8sClient.BatchV1().Jobs(radiusNamespace).Get(ctx, preUpgradeJobName, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			if i < jobPollAttempts-1 {
				time.Sleep(jobPollInterval)
			}
			continue
		case err == nil:
			t.Fatal("Expected preflight job to not exist when disabled, but it was found")
		default:
			t.Fatalf("Unexpected error checking for preflight job: %v", err)
		}
	}
	t.Log("Preflight job correctly not created when disabled")
}

// requireDeployedRelease makes sure the Radius Helm release will accept another upgrade.
//
// The Enabled subtest tolerates a failed upgrade because the preflight checks may reject
// it, which leaves the release in "failed" or, if Helm was interrupted partway, in
// "pending-upgrade". Helm refuses to start a new operation on a pending release
// ("another operation (install/upgrade/rollback) is in progress"), so recover by rolling
// back to the last deployed revision rather than letting the next upgrade fail with that
// error.
func requireDeployedRelease(t *testing.T, ctx context.Context) {
	t.Helper()

	info, err := helmReleaseStatus(ctx)
	require.NoError(t, err, "Failed to read the status of Helm release %q", helmReleaseName)
	if info.Status == helmStatusDeployed {
		return
	}

	t.Logf("Helm release %q is %q (%s); rolling back to the last deployed revision",
		helmReleaseName, info.Status, info.Description)
	err = runCommand(ctx, []string{
		"helm", "rollback", helmReleaseName,
		"--namespace", radiusNamespace,
		"--wait",
		"--timeout", helmTimeout,
	})
	require.NoError(t, err, "Failed to roll back Helm release %q from status %q (%s)",
		helmReleaseName, info.Status, info.Description)

	info, err = helmReleaseStatus(ctx)
	require.NoError(t, err, "Failed to read the status of Helm release %q after rollback", helmReleaseName)
	require.Equal(t, helmStatusDeployed, info.Status,
		"Helm release %q is still not deployed after rollback: %s", helmReleaseName, info.Description)
}

// deletePreflightJob removes the pre-upgrade job left behind by the previous subtest and
// blocks until the API server reports it gone.
//
// Helm does not remove the job itself: its helm.sh/hook-delete-policy is
// before-hook-creation, which only deletes the job when a later upgrade recreates it, and
// the upgrade with preflight disabled never renders one. Without this the assertion that
// no job was created would observe the job from the enabled run.
func deletePreflightJob(t *testing.T, ctx context.Context, options rp.RPTestOptions) {
	t.Helper()
	t.Log("Deleting the preflight job left by the previous subtest")

	jobs := options.K8sClient.BatchV1().Jobs(radiusNamespace)
	propagation := metav1.DeletePropagationForeground
	err := jobs.Delete(ctx, preUpgradeJobName, metav1.DeleteOptions{PropagationPolicy: &propagation})
	if !apierrors.IsNotFound(err) {
		require.NoError(t, err, "Failed to delete job %s/%s", radiusNamespace, preUpgradeJobName)
	}

	require.Eventually(t, func() bool {
		_, err := jobs.Get(ctx, preUpgradeJobName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true
		}
		if err != nil {
			t.Logf("Warning: failed to get job %s/%s: %v", radiusNamespace, preUpgradeJobName, err)
		}
		return false
	}, jobDeletionTimeout, jobPollInterval, "Job %s/%s was not deleted within timeout", radiusNamespace, preUpgradeJobName)
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
		"helm", "install", helmReleaseName, relativeChartPath,
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
//
// --cleanup-on-fail is set because a caller may tolerate a failed upgrade and then
// upgrade again: it removes resources the failed attempt created rather than leaving
// them for the next attempt to reconcile. It does not remove the pre-upgrade job, which
// Helm tracks as a hook rather than as a resource created from the release manifest.
func helmUpgrade(ctx context.Context, image, tag string, values map[string]string) error {
	args := []string{
		"helm", "upgrade", helmReleaseName, relativeChartPath,
		"--namespace", radiusNamespace,
		"--set", fmt.Sprintf("preupgrade.image=%s", image),
		"--set", fmt.Sprintf("preupgrade.tag=%s", tag),
		"--wait",
		"--cleanup-on-fail",
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
	err := runCommand(ctx, []string{"helm", "uninstall", helmReleaseName, "--namespace", radiusNamespace})
	require.NoError(t, err, "Failed to uninstall Radius")
}

// helmReleaseInfo is the subset of "helm status --output json" this test reads.
type helmReleaseInfo struct {
	// Status is the Helm release status, for example "deployed", "failed" or
	// "pending-upgrade".
	Status string `json:"status"`

	// Description is Helm's summary of the last operation and carries the reason a
	// release failed.
	Description string `json:"description"`
}

// helmReleaseStatus reports the current state of the Radius Helm release.
//
// Only stdout is parsed. Helm writes diagnostics such as the insecure kubeconfig
// permissions warning to stderr, and mixing those into the JSON would make a successful
// "helm status --output json" undecodable.
func helmReleaseStatus(ctx context.Context) (helmReleaseInfo, error) {
	cmd := exec.CommandContext(ctx, "helm", "status", helmReleaseName,
		"--namespace", radiusNamespace, "--output", "json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.Output()
	if err != nil {
		return helmReleaseInfo{}, fmt.Errorf("helm status failed: %w, stderr: %s", err, stderr.String())
	}

	var release struct {
		Info helmReleaseInfo `json:"info"`
	}
	if err := json.Unmarshal(stdout, &release); err != nil {
		return helmReleaseInfo{}, fmt.Errorf("failed to decode helm status output %q (stderr: %s): %w",
			string(stdout), stderr.String(), err)
	}
	return release.Info, nil
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

// newKubernetesClient builds a Kubernetes client from the ambient kubeconfig.
func newKubernetesClient() (*kubernetes.Clientset, error) {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), nil).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}
	return kubernetes.NewForConfig(config)
}

// cleanupAndWait uninstalls the Radius Helm release and waits for all Radius pods in
// the namespace to be fully terminated before returning. This prevents aggregated API
// service conflicts when the next helm install runs before previous resources are
// fully cleaned up.
//
// Only Radius-owned pods (labeled app.kubernetes.io/part-of=radius) are monitored.
// Contour is deployed as a separate Helm release in the same namespace and its pods
// are expected to remain running.
func cleanupAndWait(t *testing.T, ctx context.Context, k8sClient kubernetes.Interface) {
	t.Helper()

	t.Log("Cleaning up any existing Radius installation")
	_ = exec.CommandContext(ctx, "helm", "uninstall", helmReleaseName,
		"--namespace", radiusNamespace, "--ignore-not-found", "--wait").Run()

	// Wait for Radius pods to terminate. The Kubernetes aggregated API service needs
	// time to deregister after pods are gone, so we must wait for Radius pods to be
	// fully removed before starting a new install.
	require.Eventually(t, func() bool {
		pods, err := k8sClient.CoreV1().Pods(radiusNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: radiusPodSelector,
		})
		if err != nil {
			t.Logf("Warning: failed to list pods: %v", err)
			return false
		}
		if len(pods.Items) == 0 {
			return true
		}
		t.Logf("Waiting for %d Radius pod(s) in %s to terminate...", len(pods.Items), radiusNamespace)
		return false
	}, cleanupTimeout, cleanupPollInterval, "Radius pods in %s did not terminate within timeout", radiusNamespace)

	// Poll until the aggregated API service is fully deregistered. After pods
	// terminate, the APIService may still briefly return 503 to the API server.
	// We verify deregistration by confirming the Radius API group is no longer
	// served (returns 404 or connection refused rather than 503).
	t.Log("Waiting for aggregated API service deregistration...")
	require.Eventually(t, func() bool {
		// Use server-side discovery to check if the Radius API group is still registered.
		// A 503 means the APIService is registered but the backend is gone (not yet deregistered).
		err := func() error {
			// Query the specific group/version. During deregistration this will typically
			// return 503 while the APIService is still registered, and 404 once it is gone.
			_, err := k8sClient.Discovery().ServerResourcesForGroupVersion("api.ucp.dev/v1alpha3")
			return err
		}()
		if err == nil {
			t.Log("Radius aggregated API service still registered, waiting...")
			return false
		}
		if apierrors.IsNotFound(err) {
			return true
		}
		t.Logf("Discovery for api.ucp.dev/v1alpha3 failed (expected during deregistration): %v", err)
		return false
	}, apiServiceDeregistrationTimeout, apiServiceDeregistrationInterval, "Aggregated API service did not deregister within timeout")
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
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(jobPollInterval):
		}
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
