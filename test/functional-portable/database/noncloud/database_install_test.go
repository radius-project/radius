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

// Package database contains the functional tests that cover installing Radius with the
// PostgreSQL-backed control plane (`--set database.enabled=true`).
//
// The default functional suite installs with `database.enabled=false` (the Kubernetes API server
// store). Two blocking bugs reached review of PR #12214 as a result: the database StatefulSet
// pointed at a registry-mirror tag that did not exist (ImagePullBackOff), and the per-RP database
// users were never granted access to the `resources` table (`permission denied for table
// resources`). See issue #12227.
//
// This is one of three complementary layers, and deliberately the cheapest and most direct:
//
//   - deploy/Chart/tests/database_test.yaml guards the *rendering* of both fixes with helm-unittest.
//   - The statestore-noncloud leg also installs with database.enabled=true, but as part of a
//     40-minute destructive shutdown/startup lifecycle whose failures are ambiguous.
//   - This package is a plain install-and-deploy check, so a failure here means the
//     PostgreSQL-backed control plane itself is broken.
//
// Note that `rad install` waits on the Helm release, so both of the original bugs would already
// fail the install rather than reaching these tests. The lasting value here is
// Test_DatabaseEnabled_MinimalDeploy: a real deployment that round-trips through the PostgreSQL
// store using the resource providers' own database identities.
//
// These tests assume Radius is ALREADY installed with the database enabled:
//
//	rad install kubernetes --set database.enabled=true
//
// Every test asserts that precondition via requireDatabaseInstalled, so the suite cannot quietly
// pass against an apiserver-backed control plane regardless of which test runs first. In CI the
// `database-noncloud` matrix leg of .github/workflows/functional-test-noncloud.yaml performs that
// install.
package database

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/radius-project/radius/test"
)

const (
	// radiusNamespace is the namespace the control plane is installed into.
	radiusNamespace = "radius-system"

	// databaseStatefulSetName is the name of the PostgreSQL StatefulSet rendered by
	// deploy/Chart/templates/database/statefulset.yaml when database.enabled=true. Its pods carry
	// the label app.kubernetes.io/name=database.
	databaseStatefulSetName = "database"

	// permissionDeniedMarker is the PostgreSQL error emitted when a per-RP user was never granted
	// access to the shared `resources` table. It crash-loops the resource provider on startup.
	permissionDeniedMarker = "permission denied for table"

	// databaseReadyTimeout bounds the wait for the PostgreSQL StatefulSet to report a ready
	// replica. `rad install` already waits for the release, so this is a short confirmation rather
	// than the primary wait.
	databaseReadyTimeout = 3 * time.Minute

	// controlPlaneReadyTimeout bounds the wait for the resource provider Deployments to report the
	// Available condition. Like databaseReadyTimeout this is a backstop: `rad install` already
	// waited on the Helm release, so in a healthy run both checks pass on the first poll.
	controlPlaneReadyTimeout = 3 * time.Minute

	// pollInterval is the cadence shared by every wait in this package. Both waits are backstops
	// against an install that has already been waited on, so they poll at the same rate and differ
	// only in what they are waiting for.
	pollInterval = 5 * time.Second

	// testTimeout bounds the whole test. It must exceed databaseReadyTimeout plus
	// controlPlaneReadyTimeout so that a genuine wait fails with its own specific message rather
	// than an opaque context deadline.
	testTimeout = 7 * time.Minute

	// logTailLines bounds how much of each control-plane log is fetched for the permission scan.
	// The failure this guards against happens during startup, so a tail of the log is enough and
	// keeps the request cheap on a chatty control plane.
	logTailLines = int64(2000)
)

// controlPlaneComponents are the resource providers that talk to PostgreSQL. For each of them the
// Deployment name and the name of the main container in its pod are identical, so this one list
// drives both the availability check and the log scan. Pods can carry sidecars, so the container
// must be requested by name when reading logs.
var controlPlaneComponents = []string{"ucp", "applications-rp", "dynamic-rp"}

// apiServerStoreGVR is the CRD the apiserver database store persists into. It is the store the
// control plane uses by default, and the one it must NOT be using when database.enabled=true.
// The apiserver *queue* provider stays enabled in both modes, but it writes the separate
// queuemessages.ucp.dev CRD, so it does not interfere with this assertion.
var apiServerStoreGVR = schema.GroupVersionResource{
	Group:    "ucp.dev",
	Version:  "v1alpha1",
	Resource: "resources",
}

// databaseInstalledOnce guards the one-time lookup behind requireDatabaseInstalled. The error is
// cached rather than the assertion so that every caller re-asserts on its own *testing.T. Calling
// require inside Once.Do would fail only whichever test won the race and let the other pass
// silently, which is precisely the ordering assumption this helper exists to remove.
var (
	databaseInstalledOnce sync.Once
	databaseInstalledErr  error
)

// requireDatabaseInstalled asserts that Radius was installed with database.enabled=true, by
// requiring the PostgreSQL StatefulSet the chart renders in that mode. Every test in this package
// calls it, so a run against a default apiserver-backed control plane fails immediately with an
// actionable message rather than passing and proving nothing.
//
// This is deliberately not left to test ordering. Test_DatabaseEnabled_MinimalDeploy runs in
// parallel by way of the RPTest harness, so which test observes the cluster first depends on file
// names and on RunSerial staying false; making both tests assert the precondition removes that
// dependency.
//
// Note that a transient API error on the first caller is cached and fails both tests. That is the
// intended trade-off: it is no worse than a single un-retried Get, and this precondition is meant
// to fail fast rather than paper over a cluster that cannot be read.
func requireDatabaseInstalled(ctx context.Context, t *testing.T, k8s kubernetes.Interface) {
	t.Helper()

	databaseInstalledOnce.Do(func() {
		_, databaseInstalledErr = k8s.AppsV1().StatefulSets(radiusNamespace).Get(ctx, databaseStatefulSetName, metav1.GetOptions{})
	})

	require.NoErrorf(t, databaseInstalledErr,
		"the %q StatefulSet was not found in %s: install Radius with --set database.enabled=true before running this test",
		databaseStatefulSetName, radiusNamespace)
}

// requireAPIServerStoreUnused asserts that the apiserver database store holds nothing, which is the
// evidence that the control plane is actually backed by PostgreSQL rather than merely running a
// PostgreSQL StatefulSet alongside an apiserver-backed control plane.
//
// This is the assertion that distinguishes a real database.enabled=true install from a hybrid one.
// Asserting the StatefulSet only proves PostgreSQL is running; ucp, applications-rp and dynamic-rp
// each select their database provider independently, so any of them could still be writing here.
//
// The check is not vacuous. By the time any test in this package runs, the CI workflow has already
// waited for UCP to log "Successfully registered manifests" and then run `rad group create` and
// `rad env create` (see .github/workflows/functional-test-noncloud.yaml). Those are real writes
// through UCP and applications-rp, so an apiserver-backed control plane is guaranteed to have rows
// here and an empty list is a genuine signal. Deployment availability is deliberately not relied on
// for this: the UCP Deployment has no readiness probe and serves traffic while initialization is
// still in flight, so "Available" would prove nothing about whether the bootstrap writes landed.
//
// The list must succeed. A missing CRD or an unresolvable kind is a failure, not a pass: the CRD
// ships unconditionally in deploy/Chart/crds/ucpd/, so its absence means a broken install or a
// mistyped GVR, and treating that as success would turn this into an assertion about nothing.
//
// This assumes a cluster that was not previously installed without the flag. Switching providers
// does not migrate or delete rows already written to the apiserver store, so a long-lived local
// cluster can legitimately hold stale objects. CI creates a fresh KinD cluster, so this holds there.
func requireAPIServerStoreUnused(ctx context.Context, t *testing.T, dynamicClient dynamic.Interface) {
	t.Helper()

	stored, err := dynamicClient.Resource(apiServerStoreGVR).Namespace(radiusNamespace).List(ctx, metav1.ListOptions{})
	require.NoErrorf(t, err,
		"failed to list %s in %s. This assertion cannot pass without reading the apiserver store: the CRD ships unconditionally with the chart, so a failure here means a broken install rather than an unused store.",
		apiServerStoreGVR.Resource, radiusNamespace)

	names := []string{}
	for _, item := range stored.Items {
		names = append(names, item.GetName())
	}

	require.Emptyf(t, names,
		"the apiserver database store holds %d object(s) in %s (%s), so the control plane is still using the apiserver store instead of PostgreSQL. Check the databaseProvider settings in the ucp, applications-rp and dynamic-rp ConfigMaps. If this is a long-lived cluster that was previously installed without database.enabled=true, these may be stale objects from that install — re-run against a fresh cluster.",
		len(names), radiusNamespace, strings.Join(names, ", "))
}

// Test_DatabaseEnabled_ControlPlaneHealthy asserts that the PostgreSQL-backed control plane came up
// cleanly: the database is serving, every resource provider reached Available, no provider was
// denied access to its tables, and nothing is still being written to the apiserver store.
func Test_DatabaseEnabled_ControlPlaneHealthy(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), testTimeout)
	defer cancel()

	options := test.NewTestOptions(t)
	k8s := options.K8sClient

	requireDatabaseInstalled(ctx, t, k8s)
	requireDatabaseReady(ctx, t, k8s)
	requireResourceProvidersAvailable(ctx, t, k8s)
	requireNoPermissionDeniedInLogs(ctx, t, k8s)
	requireAPIServerStoreUnused(ctx, t, options.DynamicClient)
}

// requireResourceProvidersAvailable waits for each resource provider Deployment to report the
// Available condition. This is the assertion that actually establishes control-plane health: a
// ready PostgreSQL StatefulSet proves the database came up, not that the providers can use it.
//
// Transient crashes are tolerated on purpose — the database StatefulSet has no readiness probe, so
// a provider can legitimately crash and recover while init-db is still running. What is not
// tolerated is a provider that never reaches Available, which is how the missing per-RP GRANTs
// manifest.
func requireResourceProvidersAvailable(ctx context.Context, t *testing.T, k8s kubernetes.Interface) {
	t.Helper()

	deadline := time.Now().Add(controlPlaneReadyTimeout)
	for {
		pending := []string{}
		for _, name := range controlPlaneComponents {
			deployment, err := k8s.AppsV1().Deployments(radiusNamespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				pending = append(pending, fmt.Sprintf("%s (read failed: %v)", name, err))
				continue
			}
			if !deploymentAvailable(deployment) {
				pending = append(pending, fmt.Sprintf("%s (%d/%d ready)", name, deployment.Status.ReadyReplicas, deployment.Status.Replicas))
			}
		}

		if len(pending) == 0 {
			return
		}

		if time.Now().After(deadline) {
			require.Failf(t, "the resource providers never became available",
				"still not Available after %s: %s. A resource provider that cannot reach its PostgreSQL tables crash-loops on startup.",
				controlPlaneReadyTimeout, strings.Join(pending, ", "))
		}

		t.Logf("waiting for control-plane deployments: %s", strings.Join(pending, ", "))
		select {
		case <-ctx.Done():
			require.Failf(t, "the resource providers never became available", "context ended while waiting: %v", ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

func deploymentAvailable(deployment *appsv1.Deployment) bool {
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// requireDatabaseReady waits for the PostgreSQL StatefulSet to report a ready replica, giving up
// immediately (rather than after the full timeout) if its container cannot pull its image. A bad
// image reference is terminal, so there is nothing to wait for. This is the regression guard for
// the ImagePullBackOff bug.
//
// The poll runs on the test goroutine rather than inside require.Eventually: Eventually evaluates
// its condition in a worker goroutine, where a failed assertion only stops that goroutine and the
// test would still burn the whole timeout and then report a misleading "never became ready".
func requireDatabaseReady(ctx context.Context, t *testing.T, k8s kubernetes.Interface) {
	t.Helper()

	deadline := time.Now().Add(databaseReadyTimeout)
	for {
		if failure := databaseImagePullFailure(ctx, t, k8s); failure != "" {
			require.Failf(t, "the PostgreSQL image cannot be pulled",
				"%s. The chart's database image reference must point at a tag that actually exists.", failure)
		}

		statefulSet, err := k8s.AppsV1().StatefulSets(radiusNamespace).Get(ctx, databaseStatefulSetName, metav1.GetOptions{})
		switch {
		case err != nil:
			t.Logf("waiting to read the %q StatefulSet: %v", databaseStatefulSetName, err)
		case statefulSet.Status.ReadyReplicas >= 1:
			return
		default:
			t.Logf("waiting for the %q StatefulSet to report a ready replica (currently %d)...", databaseStatefulSetName, statefulSet.Status.ReadyReplicas)
		}

		if time.Now().After(deadline) {
			require.Failf(t, "the PostgreSQL StatefulSet never became ready",
				"%q did not report a ready replica within %s", databaseStatefulSetName, databaseReadyTimeout)
		}

		select {
		case <-ctx.Done():
			require.Failf(t, "the PostgreSQL StatefulSet never became ready", "context ended while waiting: %v", ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// databaseImagePullFailure returns a description of an image pull failure on the PostgreSQL
// container, or an empty string when there is none. Only the database container is checked this
// way: it is the one whose image the chart pins, and an image pull failure is unambiguous. The
// resource providers are deliberately not checked for CrashLoopBackOff, because the database
// StatefulSet has no readiness probe and an RP can legitimately crash and recover while init-db is
// still running.
func databaseImagePullFailure(ctx context.Context, t *testing.T, k8s kubernetes.Interface) string {
	t.Helper()

	pods, err := k8s.CoreV1().Pods(radiusNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=" + databaseStatefulSetName,
	})
	if err != nil {
		t.Logf("waiting to list database pods: %v", err)
		return ""
	}

	for _, pod := range pods.Items {
		for _, status := range pod.Status.ContainerStatuses {
			waiting := status.State.Waiting
			if waiting == nil {
				continue
			}
			if waiting.Reason == "ImagePullBackOff" || waiting.Reason == "ErrImagePull" {
				return fmt.Sprintf("pod %s container %s is %s for image %q: %s",
					pod.Name, status.Name, waiting.Reason, status.Image, waiting.Message)
			}
		}
	}
	return ""
}

// requireNoPermissionDeniedInLogs scans the control-plane logs for the PostgreSQL error raised when
// a per-RP user was never granted access to the shared `resources` table. This is the regression
// guard for the "permission denied for table resources" crash. Both the current and the previous
// container instance are scanned: the error crash-loops the RP, so by the time the test runs the
// evidence may only survive in the previous instance's log.
//
// The scan must not pass vacuously. A log read that fails, a pod that never appears, or a container
// name that stops matching would all silently turn this into an assertion about nothing, so the
// function requires that every expected provider's current log was actually read.
func requireNoPermissionDeniedInLogs(ctx context.Context, t *testing.T, k8s kubernetes.Interface) {
	t.Helper()

	pods, err := k8s.CoreV1().Pods(radiusNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/part-of=radius",
	})
	require.NoError(t, err, "failed to list control-plane pods")

	// scanned records the containers whose current log was read successfully, so the assertion
	// below can prove the scan actually covered every resource provider.
	scanned := map[string]bool{}

	for _, pod := range pods.Items {
		for _, container := range containersToScan(pod) {
			for _, previous := range []bool{false, true} {
				logs, err := readContainerLogs(ctx, k8s, pod.Name, container, previous)
				if err != nil {
					// Not fatal on its own: a pod may be terminating or replaced, and a missing
					// previous instance is the normal healthy case that the API reports as an
					// error. The `scanned` check below is what prevents this from failing open.
					t.Logf("skipping logs for pod %s container %s (previous=%t): %v", pod.Name, container, previous, err)
					continue
				}

				if !previous {
					scanned[container] = true
				}

				if line, found := findLine(logs, permissionDeniedMarker); found {
					require.Failf(t, "a resource provider was denied access to a PostgreSQL table",
						"pod %s container %s (previous=%t) logged: %s. The per-RP database users must be granted privileges on the tables created by the superuser (see deploy/Chart/templates/database/configmap-initdb.yaml).",
						pod.Name, container, previous, line)
				}
			}
		}
	}

	missing := []string{}
	for _, container := range controlPlaneComponents {
		if !scanned[container] {
			missing = append(missing, container)
		}
	}
	require.Emptyf(t, missing, "could not read current logs for %s, so the permission scan proved nothing", strings.Join(missing, ", "))
}

// containersToScan returns the control-plane containers present in the pod. Pods can carry
// sidecars, so containers are matched by name rather than assuming the pod has exactly one.
func containersToScan(pod corev1.Pod) []string {
	matched := []string{}
	for _, container := range pod.Spec.Containers {
		for _, wanted := range controlPlaneComponents {
			if container.Name == wanted {
				matched = append(matched, container.Name)
				break
			}
		}
	}
	return matched
}

func readContainerLogs(ctx context.Context, k8s kubernetes.Interface, podName string, container string, previous bool) (string, error) {
	tail := logTailLines
	stream, err := k8s.CoreV1().Pods(radiusNamespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: container,
		Previous:  previous,
		TailLines: &tail,
	}).Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	contents, err := io.ReadAll(stream)
	if err != nil {
		return "", err
	}
	return string(contents), nil
}

// findLine returns the first line containing marker, so failures report the offending log line
// instead of an entire container log.
func findLine(logs string, marker string) (string, bool) {
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, marker) {
			return strings.TrimSpace(line), true
		}
	}
	return "", false
}
