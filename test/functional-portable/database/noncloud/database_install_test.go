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
// store), so nothing exercised the PostgreSQL path end to end. Two blocking bugs reached review of
// PR #12214 as a result: the database StatefulSet pointed at a registry-mirror tag that did not
// exist (ImagePullBackOff), and the per-RP database users were never granted access to the
// `resources` table (`permission denied for table resources`). See issue #12227.
//
// The chart's helm-unittest suite (deploy/Chart/tests/database_test.yaml) guards the *rendering* of
// both fixes. This package guards the *runtime*: the control plane must actually come up against
// PostgreSQL, and a real deployment must round-trip through the PostgreSQL store.
//
// These tests assume Radius is ALREADY installed with the database enabled:
//
//	rad install kubernetes --set database.enabled=true
//
// Test_DatabaseEnabled_ControlPlaneHealthy fails immediately if it is not, so the suite cannot
// quietly pass against an apiserver-backed control plane. In CI the `database-noncloud` matrix leg
// of .github/workflows/functional-test-noncloud.yaml performs that install.
package database

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	databaseReadyPoll    = 5 * time.Second

	// logTailLines bounds how much of each control-plane log is fetched for the permission scan.
	// The failure this guards against happens during startup, so a tail of the log is enough and
	// keeps the request cheap on a chatty control plane.
	logTailLines = int64(2000)
)

// controlPlaneContainers are the main containers of the resource providers that talk to PostgreSQL.
// Their Deployments are named the same as the container in each pod, and the pods can carry
// sidecars, so the container must be requested by name when reading logs.
var controlPlaneContainers = []string{"ucp", "applications-rp", "dynamic-rp"}

// Test_DatabaseEnabled_ControlPlaneHealthy asserts that the PostgreSQL-backed control plane came up
// cleanly. It is deliberately narrow: `rad install` already waits on the Helm release, so this
// test's job is to turn the two known failure modes into named, immediately readable failures
// instead of an opaque timeout somewhere later in the suite.
func Test_DatabaseEnabled_ControlPlaneHealthy(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	k8s := test.NewTestOptions(t).K8sClient

	// The database StatefulSet must exist at all. If it does not, Radius was installed without
	// database.enabled=true and the rest of this package proves nothing — fail loudly rather than
	// pass a meaningless run.
	statefulSet, err := k8s.AppsV1().StatefulSets(radiusNamespace).Get(ctx, databaseStatefulSetName, metav1.GetOptions{})
	require.NoErrorf(t, err, "the %q StatefulSet was not found in %s: install Radius with --set database.enabled=true before running this test", databaseStatefulSetName, radiusNamespace)
	require.NotNil(t, statefulSet)

	requireDatabaseReady(ctx, t, k8s)
	requireNoPermissionDeniedInLogs(ctx, t, k8s)
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
		case <-time.After(databaseReadyPoll):
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
func requireNoPermissionDeniedInLogs(ctx context.Context, t *testing.T, k8s kubernetes.Interface) {
	t.Helper()

	pods, err := k8s.CoreV1().Pods(radiusNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/part-of=radius",
	})
	require.NoError(t, err, "failed to list control-plane pods")

	for _, pod := range pods.Items {
		for _, container := range containersToScan(pod) {
			for _, previous := range []bool{false, true} {
				logs, err := readContainerLogs(ctx, k8s, pod.Name, container, previous)
				if err != nil {
					// A missing previous instance is the normal, healthy case and the API returns
					// an error for it. Never fail the test on a log-read error; the assertion is
					// about what the logs contain, not about whether they could be read.
					t.Logf("skipping logs for pod %s container %s (previous=%t): %v", pod.Name, container, previous, err)
					continue
				}

				if line, found := findLine(logs, permissionDeniedMarker); found {
					require.Failf(t, "a resource provider was denied access to a PostgreSQL table",
						"pod %s container %s (previous=%t) logged: %s. The per-RP database users must be granted privileges on the tables created by the superuser (see deploy/Chart/templates/database/configmap-initdb.yaml).",
						pod.Name, container, previous, line)
				}
			}
		}
	}
}

// containersToScan returns the control-plane containers present in the pod. Pods can carry
// sidecars, so containers are matched by name rather than assuming the pod has exactly one.
func containersToScan(pod corev1.Pod) []string {
	matched := []string{}
	for _, container := range pod.Spec.Containers {
		for _, wanted := range controlPlaneContainers {
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
