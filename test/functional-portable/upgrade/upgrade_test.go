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
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/testutil"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
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
	controlPlanePollInterval = 3 * time.Second
	cleanupPollInterval      = 3 * time.Second
	jobPollInterval          = 1 * time.Second
	jobPollAttempts          = 15

	// jobDeletionTimeout is the maximum time to wait for a deleted pre-upgrade Job to
	// disappear from the API server.
	jobDeletionTimeout = 60 * time.Second

	// controlPlaneTimeout is the maximum time to wait for the control plane API
	// to become available after Helm install/upgrade completes. This needs to be
	// generous because the UCP aggregated APIService may briefly return 503 while
	// pods are rolling.
	controlPlaneTimeout = 4 * time.Minute

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

	// ucpServiceName is the Service the kube-apiserver aggregation layer forwards
	// api.ucp.dev requests to.
	ucpServiceName = "ucp"

	// ucpPodSelector and ucpContainerName identify the workload behind that Service.
	ucpPodSelector   = "app.kubernetes.io/name=ucp"
	ucpContainerName = "ucp"

	// ucpAPIServicePath addresses the aggregated APIService object. It is fetched as
	// raw JSON because k8s.io/kube-aggregator is not a dependency of this module.
	ucpAPIServiceName = "v1alpha3.api.ucp.dev"
	ucpAPIServicePath = "/apis/apiregistration.k8s.io/v1/apiservices/" + ucpAPIServiceName

	// controlPlaneDiagnosticsTimeout bounds the diagnostics dump emitted when
	// readiness times out, so a wedged API server cannot hang the test run.
	controlPlaneDiagnosticsTimeout = 30 * time.Second

	// ucpLogTailLines is how many lines of UCP container logs to include in the
	// readiness failure diagnostics.
	ucpLogTailLines = 100

	// recentEventLimit is how many of the most recent namespace events to include in
	// the readiness failure diagnostics.
	recentEventLimit = 30
)

// Test_PreflightContainer exercises the chart's pre-upgrade Helm hook.
//
// The hook only runs on helm upgrade, so installing Radius is setup cost rather than the
// subject of the test. The parent installs once with the hook enabled, waits for the
// control plane once, and uninstalls once; the subtests then run in sequence against
// that single release, upgrading first with the hook enabled and then again with it
// disabled. They share a Helm release name and a Kubernetes namespace and so must not
// run in parallel.
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

	release := preflightRelease{options: waitForControlPlane(t, ctx, k8sClient), image: image, tag: tag}

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

// waitForControlPlane blocks until the Radius aggregated API is serving, then builds
// the test options.
//
// A transient 503 immediately after install is normal and unavoidable: the APIService
// Available condition is owned by the kube-apiserver aggregator, which re-checks the
// backend on its own schedule and can keep returning a stale unavailable result for
// several seconds after the UCP pod is Ready and serving.
//
// Readiness is polled with an error-returning probe rather than by calling
// rp.NewRPTestOptions in a retry callback. NewRPTestOptions asserts with require.*,
// which calls t.FailNow and therefore runtime.Goexit. Goexit is not a panic, so
// recover() cannot see it; a condition goroutine that exits that way never reports
// back to testify's Eventually loop, which only re-arms its retry ticker when the
// condition returns a value. The result was a single readiness attempt followed by an
// idle wait for the full timeout. rp.NewRPTestOptions is therefore called exactly
// once, on the test goroutine, after readiness has already succeeded.
func waitForControlPlane(t *testing.T, ctx context.Context, k8sClient kubernetes.Interface) rp.RPTestOptions {
	t.Helper()

	readyCtx, cancel := context.WithTimeout(ctx, controlPlaneTimeout)
	defer cancel()

	if err := testutil.WaitForControlPlaneReady(readyCtx, t, k8sClient.Discovery().RESTClient(), controlPlanePollInterval); err != nil {
		logControlPlaneDiagnostics(t, ctx, k8sClient)
		require.NoError(t, err, "Control plane did not become available within timeout")
	}

	return rp.NewRPTestOptions(t)
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

// logControlPlaneDiagnostics dumps the state of the Radius aggregated API and the UCP
// workload behind it, so a readiness timeout can be diagnosed from the CI log without
// a rerun.
//
// It runs on a context detached from ctx because the usual reason for calling it is an
// expired deadline, which would otherwise cancel every diagnostic request.
func logControlPlaneDiagnostics(t *testing.T, ctx context.Context, k8sClient kubernetes.Interface) {
	t.Helper()
	t.Log("Control plane readiness diagnostics")

	diagnosticsCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), controlPlaneDiagnosticsTimeout)
	defer cancel()

	logAPIServiceConditions(t, diagnosticsCtx, k8sClient)
	logUCPService(t, diagnosticsCtx, k8sClient)
	logUCPPods(t, diagnosticsCtx, k8sClient)
	logRecentEvents(t, diagnosticsCtx, k8sClient)
}

// logAPIServiceConditions reports the aggregator's own view of the Radius APIService.
// This distinguishes "the aggregator never contacted UCP" from "UCP answered badly".
func logAPIServiceConditions(t *testing.T, ctx context.Context, k8sClient kubernetes.Interface) {
	t.Helper()

	raw, err := k8sClient.Discovery().RESTClient().Get().AbsPath(ucpAPIServicePath).DoRaw(ctx)
	if err != nil {
		t.Logf("failed to get APIService %s: %v", ucpAPIServiceName, err)
		return
	}

	var apiService struct {
		Status struct {
			Conditions []struct {
				Type    string `json:"type"`
				Status  string `json:"status"`
				Reason  string `json:"reason"`
				Message string `json:"message"`
			} `json:"conditions"`
		} `json:"status"`
	}
	if err := json.Unmarshal(raw, &apiService); err != nil {
		t.Logf("failed to decode APIService %s (raw: %s): %v", ucpAPIServiceName, string(raw), err)
		return
	}

	if len(apiService.Status.Conditions) == 0 {
		t.Logf("APIService %s reports no status conditions", ucpAPIServiceName)
		return
	}
	for _, condition := range apiService.Status.Conditions {
		t.Logf("APIService %s condition %s=%s reason=%s message=%s",
			ucpAPIServiceName, condition.Type, condition.Status, condition.Reason, condition.Message)
	}
}

// logUCPService reports the Service the aggregator routes through and its
// EndpointSlices, which show whether a Ready backend was actually registered.
func logUCPService(t *testing.T, ctx context.Context, k8sClient kubernetes.Interface) {
	t.Helper()

	service, err := k8sClient.CoreV1().Services(radiusNamespace).Get(ctx, ucpServiceName, metav1.GetOptions{})
	if err != nil {
		t.Logf("failed to get Service %s/%s: %v", radiusNamespace, ucpServiceName, err)
	} else {
		ports := make([]string, 0, len(service.Spec.Ports))
		for _, port := range service.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%s:%d->%s/%s", port.Name, port.Port, port.TargetPort.String(), port.Protocol))
		}
		t.Logf("Service %s/%s: clusterIP=%s ports=%s selector=%v",
			service.Namespace, service.Name, service.Spec.ClusterIP, strings.Join(ports, ","), service.Spec.Selector)
	}

	endpointSlices, err := k8sClient.DiscoveryV1().EndpointSlices(radiusNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: discoveryv1.LabelServiceName + "=" + ucpServiceName,
	})
	if err != nil {
		t.Logf("failed to list EndpointSlices for Service %s/%s: %v", radiusNamespace, ucpServiceName, err)
		return
	}
	if len(endpointSlices.Items) == 0 {
		t.Logf("Service %s/%s has no EndpointSlices", radiusNamespace, ucpServiceName)
		return
	}

	for _, endpointSlice := range endpointSlices.Items {
		endpoints := make([]string, 0, len(endpointSlice.Endpoints))
		for _, endpoint := range endpointSlice.Endpoints {
			endpoints = append(endpoints, fmt.Sprintf("%s ready=%s serving=%s terminating=%s",
				strings.Join(endpoint.Addresses, ","),
				optionalBool(endpoint.Conditions.Ready),
				optionalBool(endpoint.Conditions.Serving),
				optionalBool(endpoint.Conditions.Terminating)))
		}
		t.Logf("EndpointSlice %s/%s: endpoints=[%s]", endpointSlice.Namespace, endpointSlice.Name, strings.Join(endpoints, "; "))
	}
}

// logUCPPods reports pod and container status plus recent UCP logs.
func logUCPPods(t *testing.T, ctx context.Context, k8sClient kubernetes.Interface) {
	t.Helper()

	pods, err := k8sClient.CoreV1().Pods(radiusNamespace).List(ctx, metav1.ListOptions{LabelSelector: ucpPodSelector})
	if err != nil {
		t.Logf("failed to list pods matching %q in %s: %v", ucpPodSelector, radiusNamespace, err)
		return
	}
	if len(pods.Items) == 0 {
		t.Logf("no pods matching %q in %s", ucpPodSelector, radiusNamespace)
		return
	}

	tailLines := int64(ucpLogTailLines)
	for _, pod := range pods.Items {
		t.Logf("Pod %s/%s: phase=%s podIP=%s", pod.Namespace, pod.Name, pod.Status.Phase, pod.Status.PodIP)
		for _, condition := range pod.Status.Conditions {
			t.Logf("  condition %s=%s reason=%s message=%s", condition.Type, condition.Status, condition.Reason, condition.Message)
		}
		for _, containerStatus := range pod.Status.ContainerStatuses {
			t.Logf("  container %s: ready=%t restarts=%d state=%s",
				containerStatus.Name, containerStatus.Ready, containerStatus.RestartCount, containerState(containerStatus.State))
		}

		logs, err := k8sClient.CoreV1().Pods(pod.Namespace).
			GetLogs(pod.Name, &corev1.PodLogOptions{Container: ucpContainerName, TailLines: &tailLines}).
			DoRaw(ctx)
		if err != nil {
			t.Logf("  failed to get logs for %s/%s: %v", pod.Namespace, pod.Name, err)
			continue
		}
		t.Logf("  last %d log lines for %s/%s:\n%s", ucpLogTailLines, pod.Namespace, pod.Name, string(logs))
	}
}

// logRecentEvents reports the most recent events in the Radius namespace.
func logRecentEvents(t *testing.T, ctx context.Context, k8sClient kubernetes.Interface) {
	t.Helper()

	events, err := k8sClient.CoreV1().Events(radiusNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Logf("failed to list events in %s: %v", radiusNamespace, err)
		return
	}
	if len(events.Items) == 0 {
		t.Logf("no events in %s", radiusNamespace)
		return
	}

	items := events.Items
	sort.Slice(items, func(i, j int) bool { return eventTime(items[i]).Before(eventTime(items[j])) })
	if len(items) > recentEventLimit {
		items = items[len(items)-recentEventLimit:]
	}

	for _, event := range items {
		t.Logf("Event %s %s/%s: type=%s reason=%s message=%s",
			eventTime(event).Format(time.RFC3339), event.InvolvedObject.Kind, event.InvolvedObject.Name,
			event.Type, event.Reason, event.Message)
	}
}

// eventTime returns the most meaningful timestamp available on an event. Events written
// through the newer events.k8s.io API leave LastTimestamp empty.
func eventTime(event corev1.Event) time.Time {
	switch {
	case !event.LastTimestamp.IsZero():
		return event.LastTimestamp.Time
	case !event.EventTime.IsZero():
		return event.EventTime.Time
	default:
		return event.CreationTimestamp.Time
	}
}

func containerState(state corev1.ContainerState) string {
	switch {
	case state.Running != nil:
		return fmt.Sprintf("running since %s", state.Running.StartedAt.Format(time.RFC3339))
	case state.Waiting != nil:
		return fmt.Sprintf("waiting reason=%s message=%s", state.Waiting.Reason, state.Waiting.Message)
	case state.Terminated != nil:
		return fmt.Sprintf("terminated reason=%s exitCode=%d message=%s",
			state.Terminated.Reason, state.Terminated.ExitCode, state.Terminated.Message)
	default:
		return "unknown"
	}
}

func optionalBool(value *bool) string {
	if value == nil {
		return "unknown"
	}
	return strconv.FormatBool(*value)
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
