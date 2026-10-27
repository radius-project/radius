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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/kube"
	kubefake "helm.sh/helm/v4/pkg/kube/fake"
	"helm.sh/helm/v4/pkg/release"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
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
				"rp":         map[string]any{"publicEndpointOverride": "retained.example.test"},
				"preupgrade": map[string]any{"enabled": true},
			}
			if testCase.storedTag != "" {
				values["global"] = map[string]any{"imageTag": testCase.storedTag}
			}
			installed, err := client.RunHelmInstall(configuration, previousChart, values, "radius", "radius-system", false, DefaultInstallTimeout, nil)
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
	t.Parallel()

	tests := []struct {
		name      string
		revisions []int
		expected  []int
	}{
		{"secret name order", []int{1, 10, 11, 2, 9}, []int{1, 2, 9, 10, 11}},
		{"reverse revision order", []int{11, 10, 9}, []int{9, 10, 11}},
		{"revision order", []int{9, 10, 11}, []int{9, 10, 11}},
		{"single revision", []int{10}, []int{10}},
		{"empty history", nil, []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			history := make([]release.Releaser, 0, len(tt.revisions))
			for _, revision := range tt.revisions {
				history = append(history, &releasev1.Release{Name: "radius", Version: revision})
			}
			configuration := newHistoryTestConfiguration(history, nil)

			releases, err := NewHelmClient().RunHelmHistory(configuration, "radius")
			require.NoError(t, err)

			revisions := make([]int, 0, len(releases))
			for _, rel := range releases {
				revisions = append(revisions, rel.Version)
			}
			require.Equal(t, tt.expected, revisions)
		})
	}
}

func TestHelmClientImpl_RunHelmHistory_Error(t *testing.T) {
	t.Parallel()

	expected := errors.New("failed to query release history")
	configuration := newHistoryTestConfiguration(nil, expected)

	releases, err := NewHelmClient().RunHelmHistory(configuration, "radius")
	require.ErrorIs(t, err, expected)
	require.Nil(t, releases)
}

type historyTestDriver struct {
	driver.Driver
	releases []release.Releaser
	err      error
}

func (d *historyTestDriver) Query(map[string]string) ([]release.Releaser, error) {
	return d.releases, d.err
}

func newHistoryTestConfiguration(releases []release.Releaser, err error) *helm.Configuration {
	return &helm.Configuration{
		Releases: storage.Init(&historyTestDriver{
			Driver:   driver.NewMemory(),
			releases: releases,
			err:      err,
		}),
		KubeClient: &kubefake.PrintingKubeClient{Out: io.Discard},
	}
}

func TestHelmClientImpl_RunHelmRollback(t *testing.T) {
	t.Parallel()
	newChart := func(version string) *chart.Chart {
		return &chart.Chart{
			Metadata: &chart.Metadata{APIVersion: chart.APIVersionV2, Name: "radius", Version: version, AppVersion: version},
			Values:   map[string]any{"setting": "default"},
			Templates: []*common.File{{
				Name: "templates/configmap.yaml",
				Data: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: rollback\ndata:\n  setting: {{ .Values.setting | quote }}\n"),
			}},
		}
	}
	setup := func(t *testing.T) *helm.Configuration {
		configuration := &helm.Configuration{
			Releases:     storage.Init(driver.NewMemory()),
			KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
			Capabilities: common.DefaultCapabilities,
		}
		client := NewHelmClient()
		_, err := client.RunHelmInstall(configuration, newChart("0.61.0"), map[string]any{"setting": "first"}, "radius", "radius-system", false, time.Minute, nil)
		require.NoError(t, err)
		_, err = client.RunHelmUpgrade(configuration, newChart("0.62.0"), map[string]any{"setting": "second"}, "radius", "radius-system", false, false, time.Minute)
		require.NoError(t, err)
		return configuration
	}

	tests := []struct {
		name     string
		revision int
	}{
		{name: "explicit revision", revision: 1},
		{name: "revision 0 means the previous revision", revision: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			configuration := setup(t)
			require.NoError(t, NewHelmClient().RunHelmRollback(configuration, "radius", tt.revision, false))

			latest, err := NewHelmClient().RunHelmGet(configuration, "radius")
			require.NoError(t, err)
			require.Equal(t, 3, latest.Version)
			require.Equal(t, releasecommon.StatusDeployed, latest.Info.Status)
			require.Equal(t, "Rollback to 1", latest.Info.Description)
			require.Equal(t, "0.61.0", latest.Chart.Metadata.Version)
			require.Equal(t, map[string]any{"setting": "first"}, latest.Config)
			require.Contains(t, latest.Manifest, `setting: "first"`)
		})
	}

	t.Run("errors", func(t *testing.T) {
		t.Parallel()
		client := NewHelmClient()
		require.ErrorContains(t, client.RunHelmRollback(nil, "radius", 1, false), "not initialized")
		configuration := setup(t)
		require.ErrorContains(t, client.RunHelmRollback(configuration, "radius", -1, false), "invalid revision")
		require.ErrorContains(t, client.RunHelmRollback(configuration, "radius", 9, false), "failed to get revision 9")

		installedOnly := &helm.Configuration{
			Releases:     storage.Init(driver.NewMemory()),
			KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
			Capabilities: common.DefaultCapabilities,
		}
		_, err := client.RunHelmInstall(installedOnly, newChart("0.61.0"), nil, "radius", "radius-system", false, time.Minute, nil)
		require.NoError(t, err)
		require.ErrorContains(t, client.RunHelmRollback(installedOnly, "radius", 0, false), "no previous revision")
	})
}

func Test_newRollbackClient_Configuration(t *testing.T) {
	t.Parallel()
	for _, serverSideApply := range []bool{true, false} {
		rollbackClient := newRollbackClient(&helm.Configuration{}, "test-namespace", 4, true, serverSideApply)

		require.Equal(t, "test-namespace", rollbackClient.Namespace)
		require.Equal(t, "Rollback to 4", rollbackClient.Description)
		require.Equal(t, rollbackTimeout, rollbackClient.Timeout)
		require.Equal(t, kube.LegacyStrategy, rollbackClient.WaitStrategy)
		require.True(t, rollbackClient.ResetValues)
		require.False(t, rollbackClient.ResetThenReuseValues)
		require.True(t, rollbackClient.DisableHooks)
		require.Equal(t, serverSideApply, rollbackClient.ForceConflicts)
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

func TestHelmClient_FieldManager(t *testing.T) {
	require.Equal(t, "rad", FieldManager)
	require.Equal(t, FieldManager, kube.ManagedFieldsManager)
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
// the legacy wait strategy, an effective timeout, the expected value-reuse semantics, and forced
// conflicts whenever the upgrade uses server-side apply.
func Test_newUpgradeClient_Configuration(t *testing.T) {
	tests := []struct {
		name            string
		wait            bool
		reuseValues     bool
		timeout         time.Duration
		serverSideApply bool
		expectedWait    kube.WaitStrategy
		expectedTimeout time.Duration
	}{
		{
			name:            "wait uses the legacy poller and reuses values",
			wait:            true,
			reuseValues:     true,
			timeout:         20 * time.Minute,
			serverSideApply: true,
			expectedWait:    kube.LegacyStrategy,
			expectedTimeout: 20 * time.Minute,
		},
		{
			name:            "unset timeout falls back to the default and resets values",
			wait:            true,
			reuseValues:     false,
			timeout:         0,
			serverSideApply: true,
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

			upgradeClient := newUpgradeClient(&helm.Configuration{}, "test-namespace", tt.wait, tt.reuseValues, tt.timeout, tt.serverSideApply)

			require.Equal(t, tt.expectedWait, upgradeClient.WaitStrategy)
			require.Equal(t, tt.expectedTimeout, upgradeClient.Timeout)
			require.Equal(t, "test-namespace", upgradeClient.Namespace)
			require.Equal(t, tt.reuseValues, upgradeClient.ResetThenReuseValues)
			require.Equal(t, !tt.reuseValues, upgradeClient.ResetValues)
			require.Equal(t, tt.serverSideApply, upgradeClient.ForceConflicts)
			require.Equal(t, "auto", upgradeClient.ServerSideApply)
		})
	}
}

func Test_usesServerSideApply(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		applyMethod string
		noRelease   bool
		expected    bool
	}{
		{name: "server-side apply release", applyMethod: "ssa", expected: true},
		{name: "client-side apply release", applyMethod: "csa", expected: false},
		{name: "release from Helm 3", applyMethod: "", expected: false},
		{name: "no release", noRelease: true, expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			configuration := &helm.Configuration{Releases: storage.Init(driver.NewMemory())}
			if !tt.noRelease {
				require.NoError(t, configuration.Releases.Create(&releasev1.Release{
					Name:        "radius",
					Version:     1,
					Namespace:   "radius-system",
					ApplyMethod: tt.applyMethod,
					Info:        &releasev1.Info{Status: releasecommon.StatusDeployed},
				}))
			}
			require.Equal(t, tt.expected, usesServerSideApply(configuration, "radius"))
		})
	}
}

// Test_RetryAfterFailedUpgrade_CertManagerMatchesHelm runs rad upgrade and rad install --reinstall against real
// Helm storage and checks that the cert-manager decision matches the authorization mode Helm renders.
func Test_RetryAfterFailedUpgrade_CertManagerMatchesHelm(t *testing.T) {
	t.Parallel()

	dryRun := map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}
	tests := []struct {
		name     string
		deployed map[string]any
		failed   map[string]any
		setArgs  []string
		want     string
	}{
		{name: "deployed dry run with failed off", deployed: dryRun, failed: map[string]any{}, want: "dryRun"},
		{name: "deployed off with failed dry run", deployed: map[string]any{}, failed: dryRun, want: "off"},
		{name: "--set global.rbac=false clears stored dry run", deployed: dryRun, setArgs: []string{"global.rbac=false"}, want: "off"},
		{name: "--set global.rbac=0 clears stored dry run", deployed: dryRun, setArgs: []string{"global.rbac=0"}, want: "off"},
		{name: "--set global.rbac= clears stored dry run", deployed: dryRun, setArgs: []string{"global.rbac="}, want: "off"},
	}

	loadChart := func(t *testing.T) *chart.Chart {
		c, err := loader.Load("../../../deploy/Chart")
		require.NoError(t, err)
		c.Files = slices.DeleteFunc(c.Files, func(file *common.File) bool {
			return strings.HasPrefix(file.Name, "crds/")
		})
		return c
	}

	for _, tt := range tests {
		for _, command := range []string{"upgrade", "install --reinstall"} {
			t.Run(tt.name+"/"+command, func(t *testing.T) {
				t.Parallel()

				configuration := &helm.Configuration{
					Releases:     storage.Init(driver.NewMemory()),
					KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
					Capabilities: common.DefaultCapabilities,
				}
				helmClient := NewHelmClient()
				installed, err := helmClient.RunHelmInstall(configuration, loadChart(t), tt.deployed, "radius", "radius-system", false, DefaultInstallTimeout, nil)
				require.NoError(t, err)
				if tt.failed != nil {
					failed := *installed
					failed.Version = 2
					failed.Info = &releasev1.Info{Status: releasecommon.StatusFailed}
					failed.Config = tt.failed
					require.NoError(t, configuration.Releases.Create(&failed))
				}

				client := NewMockHelmClient(gomock.NewController(t))
				client.EXPECT().RunHelmGet(gomock.Any(), gomock.Any()).AnyTimes().
					DoAndReturn(func(_ *helm.Configuration, name string) (*releasev1.Release, error) {
						return helmClient.RunHelmGet(configuration, name)
					})
				client.EXPECT().RunHelmHistory(gomock.Any(), gomock.Any()).AnyTimes().
					DoAndReturn(func(_ *helm.Configuration, name string) ([]*releasev1.Release, error) {
						return helmClient.RunHelmHistory(configuration, name)
					})
				client.EXPECT().LoadChart(gomock.Any()).AnyTimes().
					DoAndReturn(func(string) (*chart.Chart, error) { return loadChart(t), nil })
				client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), "radius", gomock.Any(), gomock.Any(), true, gomock.Any()).
					DoAndReturn(func(_ *helm.Configuration, c *chart.Chart, vals map[string]any, name, namespace string, wait, reuse bool, timeout time.Duration) (*releasev1.Release, error) {
						return helmClient.RunHelmUpgrade(configuration, c, vals, name, namespace, false, reuse, timeout)
					})

				detectCalls := 0
				impl := &Impl{
					Helm: client,
					newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) {
						detectCalls++
						kubeClient, apiextClient := fakeCertManagerCluster{crd: true, controller: certManagerController(CertManagerNamespace, CertManagerChartDefaultVersion, "")}.clients()
						return kubeClient, apiextClient, nil
					},
				}

				state, err := impl.CheckRadiusInstall("test-context")
				require.NoError(t, err)
				options := NewDefaultClusterOptions()
				options.Radius.ChartPath = "chart"
				options.Radius.SetArgs = tt.setArgs
				options.Contour.Disabled = true
				options.RadiusReleaseValues = state.RadiusValues
				if command == "upgrade" {
					require.NoError(t, impl.UpgradeRadius(t.Context(), options, "test-context"))
				} else {
					options.Radius.Reinstall = true
					require.NoError(t, impl.InstallRadius(t.Context(), options, "test-context"))
				}

				upgraded, err := helmClient.RunHelmGet(configuration, "radius")
				require.NoError(t, err)
				sum := sha256.Sum256([]byte(tt.want))
				require.Contains(t, upgraded.Manifest, "checksum/authz-config: "+hex.EncodeToString(sum[:]))
				if tt.want == "off" {
					require.Zero(t, detectCalls)
				} else {
					require.Equal(t, 1, detectCalls)
				}
			})
		}
	}
}
