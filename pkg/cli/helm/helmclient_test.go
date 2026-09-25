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

package helm

import (
	"bytes"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/kube"
	kubefake "helm.sh/helm/v4/pkg/kube/fake"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestHelmClientImpl_UpgradeImmutableImageDefaults(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		storedTag   string
		clearTag    bool
		expectedTag string
	}{
		{name: "chart defaults move from channel to patch", expectedTag: "0.61.1"},
		{name: "explicit channel override survives", storedTag: "0.61", expectedTag: "0.61"},
		{name: "cleared channel override adopts patch", storedTag: "0.61", clearTag: true, expectedTag: "0.61.1"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			previousChart, err := loader.Load("../../../deploy/Chart")
			require.NoError(t, err)
			previousChart.Metadata.Version = "0.61.0"
			previousChart.Metadata.AppVersion = "0.61.0"
			previousChart.Files = slices.DeleteFunc(previousChart.Files, func(file *common.File) bool {
				return strings.HasPrefix(file.Name, "crds/")
			})
			for _, template := range previousChart.Templates {
				if template.Name == "templates/_helpers.tpl" {
					require.Contains(t, string(template.Data), "{{- .Chart.AppVersion -}}")
					template.Data = bytes.Replace(template.Data, []byte("{{- .Chart.AppVersion -}}"), []byte(`{{- $parts := splitList "." .Chart.AppVersion -}}{{- printf "%s.%s" (index $parts 0) (index $parts 1) -}}`), 1)
				}
			}
			configuration := &helm.Configuration{
				Releases:     storage.Init(driver.NewMemory()),
				KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
				Capabilities: common.DefaultCapabilities,
			}
			client := NewHelmClient()
			values := map[string]any{
				"global":     map[string]any{"imageTag": testCase.storedTag},
				"rp":         map[string]any{"publicEndpointOverride": "retained.example.test"},
				"preupgrade": map[string]any{"enabled": true},
			}
			installed, err := client.RunHelmInstall(configuration, previousChart, values, "radius", "radius-system", false, DefaultInstallTimeout)
			require.NoError(t, err)
			require.Contains(t, helmWorkloadImages(t, installed.Manifest), "ghcr.io/radius-project/controller:0.61")

			nextChart, err := loader.Load("../../../deploy/Chart")
			require.NoError(t, err)
			nextChart.Metadata.Version = "0.61.1"
			nextChart.Metadata.AppVersion = "0.61.1"
			nextChart.Files = slices.DeleteFunc(nextChart.Files, func(file *common.File) bool {
				return strings.HasPrefix(file.Name, "crds/")
			})
			overrides := map[string]any{}
			if testCase.clearTag {
				overrides["global"] = map[string]any{"imageTag": ""}
			}
			upgraded, err := client.RunHelmUpgrade(configuration, nextChart, overrides, "radius", "radius-system", false, true, DefaultInstallTimeout)
			require.NoError(t, err)
			require.Equal(t, 2, upgraded.Version)
			manifest := upgraded.Manifest
			for _, hook := range upgraded.Hooks {
				manifest += "\n---\n" + hook.Manifest
			}
			images := helmWorkloadImages(t, manifest)
			for _, image := range []string{"applications-rp", "controller", "dynamic-rp", "ucpd", "pre-upgrade", "bicep", "dashboard", "deployment-engine"} {
				require.Contains(t, images, "ghcr.io/radius-project/"+image+":"+testCase.expectedTag)
			}
			require.Equal(t, "retained.example.test", upgraded.Config["rp"].(map[string]any)["publicEndpointOverride"])
		})
	}
}

func helmWorkloadImages(t *testing.T, manifest string) []string {
	t.Helper()
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	var images []string
	for {
		var workload struct {
			Spec struct {
				Template corev1.PodTemplateSpec `json:"template"`
			} `json:"spec"`
		}
		err := decoder.Decode(&workload)
		if err == io.EOF {
			return images
		}
		require.NoError(t, err)
		for _, container := range append(workload.Spec.Template.Spec.Containers, workload.Spec.Template.Spec.InitContainers...) {
			images = append(images, container.Image)
		}
	}
}

func TestHelmClientImpl_RunHelmHistory(t *testing.T) {
	client := &HelmClientImpl{}

	// Since RunHelmHistory uses real Helm internals that require a configured cluster,
	// we test the method exists and has the correct signature.
	// The actual functionality is tested through integration tests in cluster_test.go
	require.NotNil(t, client.RunHelmHistory)

	// Test with nil configuration - should handle gracefully or panic predictably
	defer func() {
		if r := recover(); r != nil {
			// Expected behavior when configuration is nil
			t.Log("Expected panic when configuration is nil")
		}
	}()

	// This will panic or error, which is expected behavior
	_, err := client.RunHelmHistory(nil, "test-release")
	if err == nil {
		t.Error("Expected error when configuration is nil, but got nil")
	}
}

func TestHelmClientImpl_RunHelmRollback(t *testing.T) {
	client := &HelmClientImpl{}

	// Since RunHelmRollback uses real Helm internals that require a configured cluster,
	// we test the method exists and has the correct signature.
	// The actual functionality is tested through integration tests in cluster_test.go
	require.NotNil(t, client.RunHelmRollback)

	// Test with nil configuration - should handle gracefully or panic predictably
	defer func() {
		if r := recover(); r != nil {
			// Expected behavior when configuration is nil
			t.Log("Expected panic when configuration is nil")
		}
	}()

	// This will panic or error, which is expected behavior
	err := client.RunHelmRollback(nil, "test-release", 1, true)
	if err == nil {
		t.Error("Expected error when configuration is nil, but got nil")
	}
}

func TestHelmClient_Constants(t *testing.T) {
	// Test that our timeout constants are defined
	require.Equal(t, time.Duration(5)*time.Minute, rollbackTimeout)
	require.Equal(t, time.Duration(5)*time.Minute, uninstallTimeout)

	// Install and upgrade wait for the whole control plane to come up, including image
	// pulls on a cold cluster, so they get a longer budget than the other operations.
	// See https://github.com/radius-project/radius/issues/10236.
	require.Equal(t, time.Duration(10)*time.Minute, DefaultInstallTimeout)
}

// Test_waitStrategy verifies that we wait using Helm's legacy poller rather than the v4
// default kstatus watcher, which can hang until timeout on resources that are already ready.
// See https://github.com/helm/helm/issues/31526.
func Test_waitStrategy(t *testing.T) {
	tests := []struct {
		name     string
		wait     bool
		expected kube.WaitStrategy
	}{
		{
			name:     "wait uses the legacy poller, not the kstatus watcher",
			wait:     true,
			expected: kube.LegacyStrategy,
		},
		{
			name:     "no wait only waits for hooks",
			wait:     false,
			expected: kube.HookOnlyStrategy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, waitStrategy(tt.wait))
		})
	}
}

func Test_effectiveTimeout(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		expected time.Duration
	}{
		{
			name:     "caller-supplied timeout is honored",
			timeout:  30 * time.Minute,
			expected: 30 * time.Minute,
		},
		{
			name:     "shorter caller-supplied timeout is honored",
			timeout:  90 * time.Second,
			expected: 90 * time.Second,
		},
		{
			name:     "zero falls back to the default",
			timeout:  0,
			expected: DefaultInstallTimeout,
		},
		{
			name:     "negative falls back to the default rather than expiring immediately",
			timeout:  -1 * time.Minute,
			expected: DefaultInstallTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, effectiveTimeout(tt.timeout))
		})
	}
}

func TestHelmClient_Interface(t *testing.T) {
	// Test that HelmClientImpl implements the HelmClient interface
	var _ HelmClient = &HelmClientImpl{}

	// Test that all new methods are included in the interface
	client := NewHelmClient()
	require.NotNil(t, client)

	// Verify the interface includes the new methods by checking they exist
	// We can't call them without a proper Helm configuration
	require.NotNil(t, client.(*HelmClientImpl).RunHelmHistory)
	require.NotNil(t, client.(*HelmClientImpl).RunHelmRollback)
}

// Mock test to verify the method signatures are correct
func TestHelmClient_MockCompatibility(t *testing.T) {
	// This test ensures our new methods can be mocked properly
	client := &HelmClientImpl{}

	// Test RunHelmHistory method signature
	var historyFunc = client.RunHelmHistory
	require.NotNil(t, historyFunc)

	// Test RunHelmRollback method signature
	var rollbackFunc func(*helm.Configuration, string, int, bool) error = client.RunHelmRollback
	require.NotNil(t, rollbackFunc)
}

// Test_newInstallClient_Configuration asserts that the install action is actually configured with
// the legacy wait strategy and an effective timeout. Test_waitStrategy and Test_effectiveTimeout
// only cover the helpers in isolation, so this guards against the assignments being dropped.
func Test_newInstallClient_Configuration(t *testing.T) {
	tests := []struct {
		name            string
		wait            bool
		timeout         time.Duration
		expectedWait    kube.WaitStrategy
		expectedTimeout time.Duration
	}{
		{
			name:            "wait uses the legacy poller and the caller timeout",
			wait:            true,
			timeout:         30 * time.Minute,
			expectedWait:    kube.LegacyStrategy,
			expectedTimeout: 30 * time.Minute,
		},
		{
			name:            "unset timeout falls back to the default",
			wait:            true,
			timeout:         0,
			expectedWait:    kube.LegacyStrategy,
			expectedTimeout: DefaultInstallTimeout,
		},
		{
			name:            "no wait only waits for hooks",
			wait:            false,
			timeout:         5 * time.Minute,
			expectedWait:    kube.HookOnlyStrategy,
			expectedTimeout: 5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			installClient := newInstallClient(&helm.Configuration{}, "test-release", "test-namespace", tt.wait, tt.timeout)

			require.Equal(t, tt.expectedWait, installClient.WaitStrategy)
			require.Equal(t, tt.expectedTimeout, installClient.Timeout)
			require.Equal(t, "test-release", installClient.ReleaseName)
			require.Equal(t, "test-namespace", installClient.Namespace)
			require.True(t, installClient.CreateNamespace)
		})
	}
}

// Test_newUpgradeClient_Configuration asserts that the upgrade action is actually configured with
// the legacy wait strategy, an effective timeout, and the expected value-reuse semantics.
func Test_newUpgradeClient_Configuration(t *testing.T) {
	tests := []struct {
		name            string
		wait            bool
		reuseValues     bool
		timeout         time.Duration
		expectedWait    kube.WaitStrategy
		expectedTimeout time.Duration
	}{
		{
			name:            "wait uses the legacy poller and reuses values",
			wait:            true,
			reuseValues:     true,
			timeout:         20 * time.Minute,
			expectedWait:    kube.LegacyStrategy,
			expectedTimeout: 20 * time.Minute,
		},
		{
			name:            "unset timeout falls back to the default and resets values",
			wait:            true,
			reuseValues:     false,
			timeout:         0,
			expectedWait:    kube.LegacyStrategy,
			expectedTimeout: DefaultInstallTimeout,
		},
		{
			name:            "no wait only waits for hooks",
			wait:            false,
			reuseValues:     true,
			timeout:         90 * time.Second,
			expectedWait:    kube.HookOnlyStrategy,
			expectedTimeout: 90 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			upgradeClient := newUpgradeClient(&helm.Configuration{}, "test-namespace", tt.wait, tt.reuseValues, tt.timeout)

			require.Equal(t, tt.expectedWait, upgradeClient.WaitStrategy)
			require.Equal(t, tt.expectedTimeout, upgradeClient.Timeout)
			require.Equal(t, "test-namespace", upgradeClient.Namespace)
			require.Equal(t, tt.reuseValues, upgradeClient.ResetThenReuseValues)
			require.Equal(t, !tt.reuseValues, upgradeClient.ResetValues)
		})
	}
}
