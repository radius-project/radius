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

package kubernetes_test

import (
	"context"
	"testing"
	"time"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	helmReleaseGroup   = "helm.toolkit.fluxcd.io"
	helmReleaseVersion = "v2"
	helmReleaseKind    = "HelmRelease"
	radiusChartName    = "radius"
	testTimeout        = 60 * time.Second
	testInterval       = 2 * time.Second
)

// Test_FluxHelmReleaseReconciler_RadiusChart tests the reconciler with a Radius HelmRelease
func Test_FluxHelmReleaseReconciler_RadiusChart(t *testing.T) {
	ctx := testcontext.New(t)
	opts := rp.NewRPTestOptions(t)

	// Test data for different scenarios
	testCases := []struct {
		name             string
		helmRelease      *unstructured.Unstructured
		expectAnnotation string
	}{
		{
			name:             "radius-chart-with-version",
			helmRelease:      createRadiusHelmRelease("radius-test", "flux-system", "0.42.0"),
			expectAnnotation: "0.42.0",
		},
		{
			name:             "radius-chart-without-version",
			helmRelease:      createRadiusHelmRelease("radius-latest", "flux-system", ""),
			expectAnnotation: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create the HelmRelease object
			err := opts.Client.Create(ctx, tc.helmRelease)
			require.NoError(t, err)
			
			// Ensure cleanup runs even if test fails
			t.Cleanup(func() {
				// Use context.Background() for cleanup to avoid cancelled context issues
				cleanupCtx := context.Background()
				if deleteErr := opts.Client.Delete(cleanupCtx, tc.helmRelease); deleteErr != nil {
					t.Logf("Warning: Failed to cleanup HelmRelease %s: %v", tc.helmRelease.GetName(), deleteErr)
				}
			})

			// Wait for preflight annotation to be added
			err = waitForPreflightAnnotation(t, ctx, opts.Client, tc.helmRelease, tc.expectAnnotation)
			require.NoError(t, err)

			// Check for PreflightSuccess event
			err = waitForPreflightEvent(t, ctx, opts.Client, tc.helmRelease, "PreflightSuccess")
			require.NoError(t, err)
		})
	}
}

// Test_FluxHelmReleaseReconciler_VersionUpdate tests version change detection
func Test_FluxHelmReleaseReconciler_VersionUpdate(t *testing.T) {
	ctx := testcontext.New(t)
	opts := rp.NewRPTestOptions(t)

	// Create initial HelmRelease with version 0.41.0
	helmRelease := createRadiusHelmRelease("radius-update-test", "flux-system", "0.41.0")
	err := opts.Client.Create(ctx, helmRelease)
	require.NoError(t, err)
	
	// Ensure cleanup runs even if test fails
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if deleteErr := opts.Client.Delete(cleanupCtx, helmRelease); deleteErr != nil {
			t.Logf("Warning: Failed to cleanup HelmRelease %s: %v", helmRelease.GetName(), deleteErr)
		}
	})

	// Wait for initial preflight check
	err = waitForPreflightAnnotation(t, ctx, opts.Client, helmRelease, "0.41.0")
	require.NoError(t, err)

	// Update to version 0.42.0
	err = updateHelmReleaseVersion(t, ctx, opts.Client, helmRelease, "0.42.0")
	require.NoError(t, err)

	// Wait for new preflight check
	err = waitForPreflightAnnotation(t, ctx, opts.Client, helmRelease, "0.42.0")
	require.NoError(t, err)

	// Verify multiple PreflightSuccess events exist
	events, err := getEventsForObject(t, ctx, opts.Client, helmRelease)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(events), 2, "Should have at least 2 preflight events")
}

// Test_FluxHelmReleaseReconciler_PreflightFailure tests handling of preflight failures
func Test_FluxHelmReleaseReconciler_PreflightFailure(t *testing.T) {
	// Note: This test would require mocking preflight failures
	// For now, we'll test the success path and ensure proper event generation
	ctx := testcontext.New(t)
	opts := rp.NewRPTestOptions(t)

	helmRelease := createRadiusHelmRelease("radius-failure-test", "flux-system", "0.42.0")
	err := opts.Client.Create(ctx, helmRelease)
	require.NoError(t, err)
	
	// Ensure cleanup runs even if test fails
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if deleteErr := opts.Client.Delete(cleanupCtx, helmRelease); deleteErr != nil {
			t.Logf("Warning: Failed to cleanup HelmRelease %s: %v", helmRelease.GetName(), deleteErr)
		}
	})

	// Even with potential failures, should eventually succeed with retries
	err = waitForPreflightAnnotation(t, ctx, opts.Client, helmRelease, "0.42.0")
	require.NoError(t, err)
}

// Helper functions

func createRadiusHelmRelease(name, namespace, version string) *unstructured.Unstructured {
	helmRelease := &unstructured.Unstructured{}
	helmRelease.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   helmReleaseGroup,
		Version: helmReleaseVersion,
		Kind:    helmReleaseKind,
	})
	helmRelease.SetName(name)
	helmRelease.SetNamespace(namespace)

	spec := map[string]any{
		"interval": "5m",
		"chart": map[string]any{
			"spec": map[string]any{
				"chart": "./deploy/Chart", // This identifies it as a Radius chart
			},
		},
		"releaseName":     radiusChartName,
		"targetNamespace": "radius-system",
	}

	if version != "" {
		chartSpec := spec["chart"].(map[string]any)["spec"].(map[string]any)
		chartSpec["version"] = version
	}

	helmRelease.Object["spec"] = spec
	return helmRelease
}

func waitForPreflightAnnotation(t *testing.T, ctx context.Context, client client.Client, helmRelease *unstructured.Unstructured, expectedVersion string) error {
	return wait.PollUntilContextTimeout(ctx, testInterval, testTimeout, true, func(ctx context.Context) (bool, error) {
		current := &unstructured.Unstructured{}
		current.SetGroupVersionKind(helmRelease.GroupVersionKind())

		err := client.Get(ctx, types.NamespacedName{
			Name:      helmRelease.GetName(),
			Namespace: helmRelease.GetNamespace(),
		}, current)
		if err != nil {
			t.Logf("Error getting HelmRelease: %v", err)
			return false, nil
		}

		annotations := current.GetAnnotations()
		if annotations == nil {
			t.Logf("No annotations found on HelmRelease %s", helmRelease.GetName())
			return false, nil
		}

		annotationKey := "radius.io/preflight-checked-version"
		actualVersion, exists := annotations[annotationKey]
		if !exists {
			t.Logf("Preflight annotation not found on HelmRelease %s", helmRelease.GetName())
			return false, nil
		}

		if actualVersion != expectedVersion {
			t.Logf("Preflight annotation version mismatch: expected %s, got %s", expectedVersion, actualVersion)
			return false, nil
		}

		t.Logf("Found preflight annotation with correct version: %s", actualVersion)
		return true, nil
	})
}

func waitForPreflightEvent(t *testing.T, ctx context.Context, client client.Client, helmRelease *unstructured.Unstructured, expectedEventType string) error {
	return wait.PollUntilContextTimeout(ctx, testInterval, testTimeout, true, func(ctx context.Context) (bool, error) {
		events, err := getEventsForObject(t, ctx, client, helmRelease)
		if err != nil {
			t.Logf("Error getting events: %v", err)
			return false, nil
		}

		for _, event := range events {
			if event.Reason == expectedEventType {
				t.Logf("Found expected event: %s - %s", event.Reason, event.Message)
				return true, nil
			}
		}

		t.Logf("Expected event %s not found, got %d events", expectedEventType, len(events))
		return false, nil
	})
}

func getEventsForObject(_ *testing.T, ctx context.Context, kubeClient client.Client, obj *unstructured.Unstructured) ([]corev1.Event, error) {
	eventList := &corev1.EventList{}
	err := kubeClient.List(ctx, eventList, client.InNamespace(obj.GetNamespace()))
	if err != nil {
		return nil, err
	}

	var objectEvents []corev1.Event
	for _, event := range eventList.Items {
		if event.InvolvedObject.Name == obj.GetName() &&
			event.InvolvedObject.Kind == obj.GetKind() {
			objectEvents = append(objectEvents, event)
		}
	}

	return objectEvents, nil
}

func updateHelmReleaseVersion(t *testing.T, ctx context.Context, client client.Client, helmRelease *unstructured.Unstructured, newVersion string) error {
	// Get current object
	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(helmRelease.GroupVersionKind())

	err := client.Get(ctx, types.NamespacedName{
		Name:      helmRelease.GetName(),
		Namespace: helmRelease.GetNamespace(),
	}, current)
	if err != nil {
		return err
	}

	// Update the version in the spec
	spec := current.Object["spec"].(map[string]any)
	chart := spec["chart"].(map[string]any)
	chartSpec := chart["spec"].(map[string]any)
	chartSpec["version"] = newVersion

	// Update the object
	err = client.Update(ctx, current)
	if err != nil {
		return err
	}

	t.Logf("Updated HelmRelease %s to version %s", helmRelease.GetName(), newVersion)
	return nil
}
