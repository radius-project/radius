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

package reconciler

import (
	"context"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/upgrade/preflight"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_FluxHelmReleaseReconciler_isRadiusChart(t *testing.T) {
	reconciler := &FluxHelmReleaseReconciler{}

	testCases := []struct {
		name        string
		helmRelease *unstructured.Unstructured
		expected    bool
	}{
		{
			name:        "chart_with_upgrade_enabled_annotation",
			helmRelease: createTestHelmReleaseWithAnnotations(map[string]string{
				RadiusUpgradeEnabledAnnotation: "true",
			}),
			expected:    true,
		},
		{
			name:        "chart_with_upgrade_disabled_annotation",
			helmRelease: createTestHelmReleaseWithAnnotations(map[string]string{
				RadiusUpgradeEnabledAnnotation: "false",
			}),
			expected:    false,
		},
		{
			name:        "radius_chart_without_annotation",
			helmRelease: createTestHelmRelease("./deploy/Chart", "", "radius"),
			expected:    false, // No opt-in annotation
		},
		{
			name:        "non_radius_chart_with_annotation",
			helmRelease: createTestHelmReleaseWithAnnotations(map[string]string{
				RadiusUpgradeEnabledAnnotation: "true",
			}),
			expected:    true, // Any chart can opt-in via annotation
		},
		{
			name:        "empty_chart_info",
			helmRelease: createTestHelmRelease("", "", ""),
			expected:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := reconciler.isRadiusChart(tc.helmRelease)
			require.Equal(t, tc.expected, result)
		})
	}
}

func Test_FluxHelmReleaseReconciler_getChartVersion(t *testing.T) {
	reconciler := &FluxHelmReleaseReconciler{}

	testCases := []struct {
		name        string
		helmRelease *unstructured.Unstructured
		expected    string
	}{
		{
			name:        "version_specified",
			helmRelease: createTestHelmRelease("radius", "0.42.0", ""),
			expected:    "0.42.0",
		},
		{
			name:        "no_version",
			helmRelease: createTestHelmRelease("radius", "", ""),
			expected:    "",
		},
		{
			name:        "empty_helmrelease",
			helmRelease: &unstructured.Unstructured{},
			expected:    "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := reconciler.getChartVersion(tc.helmRelease)
			require.Equal(t, tc.expected, result)
		})
	}
}

func Test_FluxHelmReleaseReconciler_getAnnotation(t *testing.T) {
	reconciler := &FluxHelmReleaseReconciler{}

	testCases := []struct {
		name        string
		helmRelease *unstructured.Unstructured
		key         string
		expected    string
	}{
		{
			name:        "annotation_exists",
			helmRelease: createTestHelmReleaseWithAnnotations(map[string]string{"test-key": "test-value"}),
			key:         "test-key",
			expected:    "test-value",
		},
		{
			name:        "annotation_not_exists",
			helmRelease: createTestHelmReleaseWithAnnotations(map[string]string{"other-key": "other-value"}),
			key:         "test-key",
			expected:    "",
		},
		{
			name:        "no_annotations",
			helmRelease: createTestHelmReleaseWithAnnotations(nil),
			key:         "test-key",
			expected:    "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := reconciler.getAnnotation(tc.helmRelease, tc.key)
			require.Equal(t, tc.expected, result)
		})
	}
}

func Test_FluxHelmReleaseReconciler_runPreflightChecks(t *testing.T) {
	testCases := []struct {
		name           string
		registry       *preflight.Registry
		currentVersion string
		targetVersion  string
		expectError    bool
		errorMsg       string
	}{
		{
			name:           "nil_registry",
			registry:       nil,
			currentVersion: "0.42.0",
			targetVersion:  "0.43.0",
			expectError:    false,
		},
		{
			name:           "empty_registry",
			registry:       preflight.NewRegistry(&testNullWriter{}),
			currentVersion: "0.42.0",
			targetVersion:  "0.43.0",
			expectError:    false,
		},
		{
			name:           "valid_minor_upgrade",
			registry:       createTestPreflightRegistry(),
			currentVersion: "0.42.0",
			targetVersion:  "0.43.0",
			expectError:    false,
		},
		{
			name:           "invalid_patch_upgrade",
			registry:       createTestPreflightRegistry(),
			currentVersion: "0.42.0",
			targetVersion:  "0.42.1",
			expectError:    true,
			errorMsg:       "Only incremental version upgrades are supported",
		},
		{
			name:           "valid_major_upgrade",
			registry:       createTestPreflightRegistry(),
			currentVersion: "0.42.0",
			targetVersion:  "1.0.0",
			expectError:    false,
		},
		{
			name:           "invalid_downgrade",
			registry:       createTestPreflightRegistry(),
			currentVersion: "0.43.0",
			targetVersion:  "0.42.0",
			expectError:    true,
			errorMsg:       "Downgrading is not supported",
		},
		{
			name:           "same_version_graceful_handling",
			registry:       createTestPreflightRegistry(),
			currentVersion: "0.42.0",
			targetVersion:  "0.42.0",
			expectError:    false, // Should pass gracefully without running version check
		},
		{
			name:           "invalid_major_skip",
			registry:       createTestPreflightRegistry(),
			currentVersion: "0.42.0",
			targetVersion:  "2.0.0",
			expectError:    true,
			errorMsg:       "Skipping multiple major versions not supported",
		},
		{
			name:           "invalid_minor_skip",
			registry:       createTestPreflightRegistry(),
			currentVersion: "0.42.0",
			targetVersion:  "0.44.0",
			expectError:    true,
			errorMsg:       "Only incremental version upgrades are supported",
		},
		{
			name:           "empty_versions",
			registry:       createTestPreflightRegistry(),
			currentVersion: "",
			targetVersion:  "",
			expectError:    false, // Should not add version check and pass
		},
		{
			name:           "empty_current_version",
			registry:       createTestPreflightRegistry(),
			currentVersion: "",
			targetVersion:  "0.43.0",
			expectError:    false, // Should not add version check and pass
		},
		{
			name:           "empty_target_version",
			registry:       createTestPreflightRegistry(),
			currentVersion: "0.42.0",
			targetVersion:  "",
			expectError:    false, // Should not add version check and pass
		},
		{
			name:           "invalid_version_format",
			registry:       createTestPreflightRegistry(),
			currentVersion: "invalid",
			targetVersion:  "0.43.0",
			expectError:    true,
			errorMsg:       "Invalid Semantic Version",
		},
		{
			name:           "dev_version_to_release",
			registry:       createTestPreflightRegistry(),
			currentVersion: "0.42.42-dev",
			targetVersion:  "0.43.0",
			expectError:    false,
		},
		{
			name:           "rc_version_to_release",
			registry:       createTestPreflightRegistry(),
			currentVersion: "0.43.0-rc1",
			targetVersion:  "0.43.0",
			expectError:    true,
			errorMsg:       "Only incremental version upgrades are supported",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reconciler := &FluxHelmReleaseReconciler{
				PreflightRegistry: tc.registry,
			}

			err := reconciler.runPreflightChecks(context.Background(), tc.currentVersion, tc.targetVersion)
			if tc.expectError {
				require.Error(t, err)
				if tc.errorMsg != "" {
					require.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_FluxHelmReleaseReconciler_getCurrentDeployedVersion(t *testing.T) {
	reconciler := &FluxHelmReleaseReconciler{}

	testCases := []struct {
		name            string
		helmRelease     *unstructured.Unstructured
		expectedVersion string
	}{
		{
			name:            "version_from_history",
			helmRelease:     createTestHelmReleaseWithHistory("0.42.0"),
			expectedVersion: "0.42.0",
		},
		{
			name:            "version_from_lastAttemptedRevision",
			helmRelease:     createTestHelmReleaseWithLastAttempted("0.41.0"),
			expectedVersion: "0.41.0",
		},
		{
			name:            "no_version_info",
			helmRelease:     createTestHelmRelease("radius", "", ""),
			expectedVersion: "",
		},
		{
			name:            "history_takes_precedence",
			helmRelease:     createTestHelmReleaseWithBothVersions("0.42.0", "0.41.0"),
			expectedVersion: "0.42.0", // History should take precedence
		},
		{
			name:            "empty_history_fallback",
			helmRelease:     createTestHelmReleaseWithEmptyHistory("0.41.0"),
			expectedVersion: "0.41.0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			version := reconciler.getCurrentDeployedVersion(tc.helmRelease)
			require.Equal(t, tc.expectedVersion, version)
		})
	}
}

func Test_FluxHelmReleaseReconciler_Reconcile_NonRadiusChart(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	reconciler := &FluxHelmReleaseReconciler{
		Client:            client,
		Scheme:            scheme,
		EventRecorder:     &record.FakeRecorder{},
		PreflightRegistry: nil,
	}

	// Create a non-Radius HelmRelease
	helmRelease := createTestHelmRelease("nginx", "1.0.0", "nginx")
	err := client.Create(context.Background(), helmRelease)
	require.NoError(t, err)

	// Reconcile should return without error and not process the chart
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      helmRelease.GetName(),
			Namespace: helmRelease.GetNamespace(),
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)

	// Verify no annotation was added
	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(helmRelease.GroupVersionKind())
	err = client.Get(context.Background(), req.NamespacedName, current)
	require.NoError(t, err)

	annotations := current.GetAnnotations()
	if annotations != nil {
		_, exists := annotations[RadiusUpgradeCheckedAnnotation]
		require.False(t, exists, "Should not have preflight annotation for non-Radius chart")
	}
}

func Test_FluxHelmReleaseReconciler_Reconcile_RadiusChart(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	reconciler := &FluxHelmReleaseReconciler{
		Client:            client,
		Scheme:            scheme,
		EventRecorder:     &record.FakeRecorder{},
		PreflightRegistry: createTestPreflightRegistry(),
	}

	// Create a Radius HelmRelease with upgrade enabled
	helmRelease := createTestHelmRelease("./deploy/Chart", "0.42.0", "radius")
	helmRelease.SetAnnotations(map[string]string{
		RadiusUpgradeEnabledAnnotation: "true",
	})
	err := client.Create(context.Background(), helmRelease)
	require.NoError(t, err)

	// Reconcile should process the chart
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      helmRelease.GetName(),
			Namespace: helmRelease.GetNamespace(),
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)

	// Verify annotation was added
	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(helmRelease.GroupVersionKind())
	err = client.Get(context.Background(), req.NamespacedName, current)
	require.NoError(t, err)

	annotations := current.GetAnnotations()
	require.NotNil(t, annotations)

	version, exists := annotations[RadiusUpgradeCheckedAnnotation]
	require.True(t, exists, "Should have preflight annotation for Radius chart")
	require.Equal(t, "0.42.0", version)
}

func Test_FluxHelmReleaseReconciler_Reconcile_PreflightFailure(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	reconciler := &FluxHelmReleaseReconciler{
		Client:            client,
		Scheme:            scheme,
		EventRecorder:     &record.FakeRecorder{},
		PreflightRegistry: createTestPreflightRegistry(),
	}

	// Create a Radius HelmRelease with invalid version change (downgrade)
	helmRelease := createTestHelmReleaseWithBothVersions("0.43.0", "0.42.0")
	helmRelease.Object["spec"] = map[string]any{
		"chart": map[string]any{
			"spec": map[string]any{
				"chart":   "./deploy/Chart",
				"version": "0.42.0", // Target version (downgrade)
			},
		},
		"releaseName": "radius",
	}
	// Add upgrade enabled annotation
	helmRelease.SetAnnotations(map[string]string{
		RadiusUpgradeEnabledAnnotation: "true",
	})
	err := client.Create(context.Background(), helmRelease)
	require.NoError(t, err)

	// Reconcile should fail preflight checks
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      helmRelease.GetName(),
			Namespace: helmRelease.GetNamespace(),
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Downgrading is not supported")
	require.Equal(t, time.Minute*5, result.RequeueAfter)

	// Verify HelmRelease was put on hold
	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(helmRelease.GroupVersionKind())
	err = client.Get(context.Background(), req.NamespacedName, current)
	require.NoError(t, err)

	// Check hold annotation
	annotations := current.GetAnnotations()
	require.NotNil(t, annotations)
	holdReason, exists := annotations[RadiusUpgradeHoldAnnotation]
	require.True(t, exists, "Should have hold annotation when preflight fails")
	require.Contains(t, holdReason, "Downgrading is not supported")

	// Check suspend field
	suspend, found, err := unstructured.NestedBool(current.Object, "spec", "suspend")
	require.NoError(t, err)
	require.True(t, found, "Should have suspend field")
	require.True(t, suspend, "HelmRelease should be suspended")
}

func Test_FluxHelmReleaseReconciler_holdHelmRelease(t *testing.T) {
	reconciler := &FluxHelmReleaseReconciler{}
	client := fake.NewClientBuilder().Build()
	reconciler.Client = client

	helmRelease := createTestHelmRelease("./deploy/Chart", "0.42.0", "radius")
	err := client.Create(context.Background(), helmRelease)
	require.NoError(t, err)

	// Put HelmRelease on hold
	reason := "Test preflight failure"
	err = reconciler.holdHelmRelease(context.Background(), helmRelease, reason)
	require.NoError(t, err)

	// Verify the changes
	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(helmRelease.GroupVersionKind())
	err = client.Get(context.Background(), types.NamespacedName{
		Name:      helmRelease.GetName(),
		Namespace: helmRelease.GetNamespace(),
	}, current)
	require.NoError(t, err)

	// Check annotations
	annotations := current.GetAnnotations()
	require.NotNil(t, annotations)
	holdReason, exists := annotations[RadiusUpgradeHoldAnnotation]
	require.True(t, exists)
	require.Equal(t, reason, holdReason)

	// Check suspend field
	suspend, found, err := unstructured.NestedBool(current.Object, "spec", "suspend")
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, suspend)
}

func Test_FluxHelmReleaseReconciler_clearHoldAndMarkComplete(t *testing.T) {
	reconciler := &FluxHelmReleaseReconciler{}
	client := fake.NewClientBuilder().Build()
	reconciler.Client = client

	// Create HelmRelease with hold
	helmRelease := createTestHelmReleaseWithAnnotations(map[string]string{
		RadiusUpgradeHoldAnnotation: "Test hold reason",
	})
	helmRelease.Object["spec"] = map[string]any{
		"suspend": true,
		"chart": map[string]any{
			"spec": map[string]any{
				"chart": "./deploy/Chart",
			},
		},
	}
	err := client.Create(context.Background(), helmRelease)
	require.NoError(t, err)

	// Clear hold and mark complete
	version := "0.43.0"
	err = reconciler.clearHoldAndMarkComplete(context.Background(), helmRelease, version)
	require.NoError(t, err)

	// Verify the changes
	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(helmRelease.GroupVersionKind())
	err = client.Get(context.Background(), types.NamespacedName{
		Name:      helmRelease.GetName(),
		Namespace: helmRelease.GetNamespace(),
	}, current)
	require.NoError(t, err)

	// Check annotations
	annotations := current.GetAnnotations()
	require.NotNil(t, annotations)

	// Hold annotation should be removed
	_, exists := annotations[RadiusUpgradeHoldAnnotation]
	require.False(t, exists, "Hold annotation should be removed")

	// Preflight annotation should be set
	preflightVersion, exists := annotations[RadiusUpgradeCheckedAnnotation]
	require.True(t, exists)
	require.Equal(t, version, preflightVersion)

	// Suspend field should be removed
	_, found, err := unstructured.NestedBool(current.Object, "spec", "suspend")
	require.NoError(t, err)
	require.False(t, found, "Suspend field should be removed")
}

// Helper functions

func createTestHelmRelease(chartName, version, releaseName string) *unstructured.Unstructured {
	helmRelease := &unstructured.Unstructured{}
	helmRelease.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "helm.toolkit.fluxcd.io",
		Version: "v2",
		Kind:    "HelmRelease",
	})
	helmRelease.SetName("test-release")
	helmRelease.SetNamespace("test-namespace")

	spec := map[string]any{
		"chart": map[string]any{
			"spec": map[string]any{
				"chart": chartName,
			},
		},
	}

	if version != "" {
		chartSpec := spec["chart"].(map[string]any)["spec"].(map[string]any)
		chartSpec["version"] = version
	}

	if releaseName != "" {
		spec["releaseName"] = releaseName
	}

	helmRelease.Object["spec"] = spec
	return helmRelease
}

func createTestHelmReleaseWithAnnotations(annotations map[string]string) *unstructured.Unstructured {
	helmRelease := createTestHelmRelease("test-chart", "", "")
	if annotations != nil {
		helmRelease.SetAnnotations(annotations)
	}
	return helmRelease
}

func createTestPreflightRegistry() *preflight.Registry {
	return preflight.NewRegistry(&testNullWriter{})
}

// testNullWriter implements output.Interface for testing
type testNullWriter struct{}

func (w *testNullWriter) LogInfo(format string, v ...any) {}
func (w *testNullWriter) WriteFormatted(format string, obj any, options output.FormatterOptions) error {
	return nil
}
func (w *testNullWriter) BeginStep(format string, v ...any) output.Step {
	return output.Step{}
}
func (w *testNullWriter) CompleteStep(step output.Step) {}

// Additional helper functions for version testing

func createTestHelmReleaseWithHistory(chartVersion string) *unstructured.Unstructured {
	hr := createTestHelmRelease("radius", "", "")
	history := []any{
		map[string]any{
			"chartVersion": chartVersion,
			"appVersion":   "latest",
			"digest":       "sha256:abc123",
		},
	}
	hr.Object["status"] = map[string]any{
		"history": history,
	}
	return hr
}

func createTestHelmReleaseWithLastAttempted(lastAttempted string) *unstructured.Unstructured {
	hr := createTestHelmRelease("radius", "", "")
	hr.Object["status"] = map[string]any{
		"lastAttemptedRevision": lastAttempted,
	}
	return hr
}

func createTestHelmReleaseWithBothVersions(historyVersion, lastAttempted string) *unstructured.Unstructured {
	hr := createTestHelmRelease("radius", "", "")
	history := []any{
		map[string]any{
			"chartVersion": historyVersion,
			"appVersion":   "latest",
		},
	}
	hr.Object["status"] = map[string]any{
		"history":               history,
		"lastAttemptedRevision": lastAttempted,
	}
	return hr
}

func createTestHelmReleaseWithEmptyHistory(lastAttempted string) *unstructured.Unstructured {
	hr := createTestHelmRelease("radius", "", "")
	hr.Object["status"] = map[string]any{
		"history":               []any{}, // Empty history
		"lastAttemptedRevision": lastAttempted,
	}
	return hr
}
