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

package reconciler

import (
	"context"
	"testing"

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
		name       string
		helmRelease *unstructured.Unstructured
		expected   bool
	}{
		{
			name:       "radius_chart_by_standard_path",
			helmRelease: createTestHelmRelease("./deploy/Chart", "", ""),
			expected:   true, // This is the standard Radius chart path
		},
		{
			name:       "radius_chart_by_name",
			helmRelease: createTestHelmRelease("radius", "", ""),
			expected:   true,
		},
		{
			name:       "radius_chart_by_path_with_radius",
			helmRelease: createTestHelmRelease("./charts/radius", "", ""),
			expected:   true,
		},
		{
			name:       "radius_chart_by_release_name",
			helmRelease: createTestHelmRelease("nginx", "", "radius"),
			expected:   true,
		},
		{
			name:       "non_radius_chart",
			helmRelease: createTestHelmRelease("nginx", "", "nginx"),
			expected:   false,
		},
		{
			name:       "empty_chart_info",
			helmRelease: createTestHelmRelease("", "", ""),
			expected:   false,
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
		name       string
		helmRelease *unstructured.Unstructured
		expected   string
	}{
		{
			name:       "version_specified",
			helmRelease: createTestHelmRelease("radius", "0.42.0", ""),
			expected:   "0.42.0",
		},
		{
			name:       "no_version",
			helmRelease: createTestHelmRelease("radius", "", ""),
			expected:   "",
		},
		{
			name:       "empty_helmrelease",
			helmRelease: &unstructured.Unstructured{},
			expected:   "",
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
		name        string
		registry    *preflight.Registry
		expectError bool
	}{
		{
			name:        "nil_registry",
			registry:    nil,
			expectError: false,
		},
		{
			name:        "empty_registry",
			registry:    preflight.NewRegistry(&testNullWriter{}),
			expectError: false,
		},
		{
			name:        "registry_with_checks",
			registry:    createTestPreflightRegistry(),
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reconciler := &FluxHelmReleaseReconciler{
				PreflightRegistry: tc.registry,
			}

			err := reconciler.runPreflightChecks(context.Background())
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
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
		_, exists := annotations[RadiusPreflightAnnotation]
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

	// Create a Radius HelmRelease
	helmRelease := createTestHelmRelease("./deploy/Chart", "0.42.0", "radius")
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
	
	version, exists := annotations[RadiusPreflightAnnotation]
	require.True(t, exists, "Should have preflight annotation for Radius chart")
	require.Equal(t, "0.42.0", version)
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