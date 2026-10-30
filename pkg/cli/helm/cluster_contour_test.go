/*
Copyright 2026 The Radius Authors.

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
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/kube"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// turnOffFeaturesForTraefikCRDs turns off the Contour 1.32.0 features that need CRD versions that k3s Traefik lacks.
const turnOffFeaturesForTraefikCRDs = "contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes,--disable-feature=backendtlspolicies}"

// Test_Helm_InstallRadius_ContourValues verifies the values that InstallRadius gives Helm for Contour, including the
// --contour-set and --contour-set-file values that it used to ignore.
func Test_Helm_InstallRadius_ContourValues(t *testing.T) {
	t.Parallel()

	setFile := filepath.Join(t.TempDir(), "annotation.txt")
	require.NoError(t, os.WriteFile(setFile, []byte("from-file"), 0600))

	tests := []struct {
		name         string
		crds         []crdInfo
		contourChart *chart.Chart
		setArgs      []string
		setFileArgs  []string
		want         map[string]any
	}{
		{
			name: "fresh cluster",
			want: map[string]any{
				"gatewayAPI.manageCRDs":                     true,
				"configInline.gateway.gatewayRef.name":      DefaultContourGatewayName,
				"configInline.gateway.gatewayRef.namespace": DefaultContourGatewayNamespace,
			},
		},
		{
			name:        "k3s Traefik CRDs with Contour values that turn off the features they lack",
			crds:        traefikGatewayAPICRDs(),
			setArgs:     []string{turnOffFeaturesForTraefikCRDs, "envoy.service.type=NodePort"},
			setFileArgs: []string{"envoy.service.annotations.example=" + setFile},
			want: map[string]any{
				"gatewayAPI.manageCRDs":                false,
				"contour.extraArgs":                    []any{"--disable-feature=tlsroutes", "--disable-feature=tcproutes", "--disable-feature=backendtlspolicies"},
				"envoy.service.type":                   "NodePort",
				"envoy.service.annotations.example":    "from-file",
				"configInline.gateway.gatewayRef.name": DefaultContourGatewayName,
			},
		},
		{
			name:         "gatewayAPI.manageCRDs=false with a Contour chart that Radius does not know",
			contourChart: newStubContourChart("9.9.9", contourBundleCRDs()),
			setArgs:      []string{"gatewayAPI.manageCRDs=false"},
			want:         map[string]any{"gatewayAPI.manageCRDs": false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := NewMockHelmClient(gomock.NewController(t))
			options := NewDefaultClusterOptions()
			options.Contour.SetArgs = tt.setArgs
			options.Contour.SetFileArgs = tt.setFileArgs
			contourChart := tt.contourChart
			if contourChart == nil {
				contourChart = newTestContourChart()
			}

			expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
			expectChartPull(t, client, options.Contour.ReleaseName, contourChart)
			client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(2)
			client.EXPECT().RunHelmGet(gomock.Any(), options.Radius.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(1)
			client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), options.Radius.ReleaseName, gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&releasev1.Release{}, nil).Times(1)
			var contourValues map[string]any
			client.EXPECT().RunHelmInstall(gomock.Any(), contourChart, gomock.Any(), options.Contour.ReleaseName, gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ *helm.Configuration, _ *chart.Chart, vals map[string]any, _ string, _ string, _ bool, _ time.Duration) (*releasev1.Release, error) {
					contourValues = vals
					return &releasev1.Release{}, nil
				}).Times(1)

			impl := &Impl{
				Helm:                           client,
				configureDefaultContourGateway: (&defaultContourGatewayCalls{}).configure,
				contourCluster:                 staticCRDs(tt.crds...),
			}
			require.NoError(t, impl.InstallRadius(t.Context(), options, "test-context"))

			for valuePath, want := range tt.want {
				require.Equal(t, want, nestedValue(contourValues, strings.Split(valuePath, ".")...), valuePath)
			}
		})
	}
}

// Test_Helm_InstallRadius_ContourPreflightFailureInstallsNothing reproduces the k3s Traefik failure. The Contour check
// fails before Radius is installed, so the user can fix the problem and run the same command again.
func Test_Helm_InstallRadius_ContourPreflightFailureInstallsNothing(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
	expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
	client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(1)
	// No other Helm call is expected. In particular, Radius must not be installed.

	gatewayCalls := &defaultContourGatewayCalls{}
	impl := &Impl{
		Helm:                           client,
		configureDefaultContourGateway: gatewayCalls.configure,
		contourCluster:                 staticCRDs(traefikGatewayAPICRDs()...),
	}

	err := impl.InstallRadius(t.Context(), options, "test-context")
	require.Error(t, err)
	require.True(t, clierrors.IsFriendlyError(err), "rad install kubernetes must show the message as is: %v", err)
	require.Contains(t, err.Error(), "If you used rad init, run rad install kubernetes with this flag first, and then run rad init again.")
	require.Contains(t, err.Error(), "tlsroutes.gateway.networking.k8s.io v1alpha2 (the CRD is not installed)")
	require.Contains(t, err.Error(), "--contour-set \""+turnOffFeaturesForTraefikCRDs+"\"")
	require.NotContains(t, err.Error(), "invalid ownership metadata")
	require.Zero(t, gatewayCalls.configureCalls)
}

// Test_Helm_InstallRadius_ExistingContourIsNotChecked verifies that rad install kubernetes --reinstall does not check
// an installed Contour release, because it does not change that release.
func Test_Helm_InstallRadius_ExistingContourIsNotChecked(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	options.Radius.Reinstall = true
	options.Contour.SetArgs = []string{turnOffFeaturesForTraefikCRDs}

	expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
	contourRelease := &releasev1.Release{
		Name:     options.Contour.ReleaseName,
		Chart:    newTestContourChart(),
		Info:     &releasev1.Info{Status: releasecommon.StatusDeployed},
		Manifest: crdManifest(contourBundleCRDs()...),
	}
	client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(contourRelease, nil).Times(1)
	radiusRelease := &releasev1.Release{Name: options.Radius.ReleaseName, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}}}
	client.EXPECT().RunHelmGet(gomock.Any(), options.Radius.ReleaseName).Return(radiusRelease, nil).Times(1)
	client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), options.Radius.ReleaseName, options.Radius.Namespace, true, true, gomock.Any()).
		Return(radiusRelease, nil).Times(1)
	// No Contour history, chart pull, install, or upgrade is expected.

	gatewayCalls := &defaultContourGatewayCalls{}
	impl := &Impl{
		Helm:                           client,
		configureDefaultContourGateway: gatewayCalls.configure,
		contourCluster: &fakeContourCluster{readCRDs: func(context.Context, string, []string) ([]crdInfo, error) {
			t.Error("the CRDs must not be checked when Contour does not change")
			return traefikGatewayAPICRDs(), nil
		}},
	}

	require.NoError(t, impl.InstallRadius(t.Context(), options, "test-context"))
	require.Equal(t, 1, gatewayCalls.configureCalls)
}

func Test_Helm_InstallRadius_ListCRDsError(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
	expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
	client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(1)

	impl := &Impl{
		Helm: client,
		contourCluster: &fakeContourCluster{readCRDs: func(context.Context, string, []string) ([]crdInfo, error) {
			return nil, errors.New("customresourcedefinitions is forbidden")
		}},
	}

	err := impl.InstallRadius(t.Context(), options, "test-context")
	require.ErrorContains(t, err, "failed to read CRDs")
	require.ErrorContains(t, err, "customresourcedefinitions is forbidden")
}

// Test_Helm_UpgradeRadius_ReusesStoredContourValues verifies that the check uses the Contour values that Helm keeps
// from the release, so an upgrade without --contour-set keeps the Contour features that the install turned off.
func Test_Helm_UpgradeRadius_ReusesStoredContourValues(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	contourRelease := newContourReleaseWithStoredValues(options)

	expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
	expectContourHistory(client, options, contourRelease)
	expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
	client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), options.Radius.ReleaseName, options.Radius.Namespace, true, true, gomock.Any()).
		Return(&releasev1.Release{}, nil).Times(1)
	var contourValues map[string]any
	client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), options.Contour.ReleaseName, options.Contour.Namespace, false, true, gomock.Any()).
		DoAndReturn(func(_ *helm.Configuration, _ *chart.Chart, vals map[string]any, _ string, _ string, _ bool, _ bool, _ time.Duration) (*releasev1.Release, error) {
			contourValues = vals
			return &releasev1.Release{}, nil
		}).Times(1)

	impl := &Impl{
		Helm:                           client,
		configureDefaultContourGateway: (&defaultContourGatewayCalls{}).configure,
		contourCluster:                 staticCRDs(traefikGatewayAPICRDs()...),
	}

	require.NoError(t, impl.UpgradeRadius(t.Context(), options, "test-context"))
	require.Equal(t, false, nestedValue(contourValues, "gatewayAPI", "manageCRDs"))
	require.Nil(t, nestedValue(contourValues, "contour", "extraArgs"), "Helm reuses the stored value, so Radius does not send it again")
}

// Test_Helm_UpgradeRadius_ResetValuesWithoutContourValuesFails verifies that --reset-values, which discards the stored
// Contour values, fails the check before anything is upgraded.
func Test_Helm_UpgradeRadius_ResetValuesWithoutContourValuesFails(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	options.ResetValues = true

	expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
	expectContourHistory(client, options, newContourReleaseWithStoredValues(options))
	expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
	// No upgrade is expected.

	impl := &Impl{
		Helm:           client,
		contourCluster: staticCRDs(traefikGatewayAPICRDs()...),
	}

	err := impl.UpgradeRadius(t.Context(), options, "test-context")
	require.Error(t, err)
	require.True(t, clierrors.IsFriendlyError(err))
	require.Contains(t, err.Error(), "tcproutes.gateway.networking.k8s.io v1alpha2 (the CRD is not installed)")
	require.Contains(t, err.Error(), "Add this flag to the rad upgrade kubernetes command:\n      --contour-set \""+turnOffFeaturesForTraefikCRDs+"\"")
	require.Contains(t, err.Error(), "you can instead run the upgrade without --reset-values")
}

// Test_Helm_UpgradeRadius_ContourValuesFromFlags verifies that rad upgrade kubernetes --contour-set values reach Helm
// and pass the check, whether or not the upgrade reuses the values stored on the Contour release.
func Test_Helm_UpgradeRadius_ContourValuesFromFlags(t *testing.T) {
	t.Parallel()

	for _, resetValues := range []bool{false, true} {
		t.Run(fmt.Sprintf("resetValues=%t", resetValues), func(t *testing.T) {
			t.Parallel()

			client := NewMockHelmClient(gomock.NewController(t))
			options := NewDefaultClusterOptions()
			options.ResetValues = resetValues
			options.Contour.SetArgs = []string{turnOffFeaturesForTraefikCRDs}
			// The release has no stored Contour values, like one that a Radius version without the check installed.
			contourRelease := &releasev1.Release{
				Name:    options.Contour.ReleaseName,
				Version: 1,
				Chart:   newTestContourChart(),
				Info:    &releasev1.Info{Status: releasecommon.StatusDeployed},
			}

			expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
			expectContourHistory(client, options, contourRelease)
			expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
			client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), options.Radius.ReleaseName, options.Radius.Namespace, true, !resetValues, gomock.Any()).
				Return(&releasev1.Release{}, nil).Times(1)
			var contourValues map[string]any
			client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), options.Contour.ReleaseName, options.Contour.Namespace, false, !resetValues, gomock.Any()).
				DoAndReturn(func(_ *helm.Configuration, _ *chart.Chart, vals map[string]any, _ string, _ string, _ bool, _ bool, _ time.Duration) (*releasev1.Release, error) {
					contourValues = vals
					return &releasev1.Release{}, nil
				}).Times(1)

			impl := &Impl{
				Helm:                           client,
				configureDefaultContourGateway: (&defaultContourGatewayCalls{}).configure,
				contourCluster:                 staticCRDs(traefikGatewayAPICRDs()...),
			}

			require.NoError(t, impl.UpgradeRadius(t.Context(), options, "test-context"))
			require.Equal(t, false, nestedValue(contourValues, "gatewayAPI", "manageCRDs"))
			require.Equal(t, []any{"--disable-feature=tlsroutes", "--disable-feature=tcproutes", "--disable-feature=backendtlspolicies"},
				nestedValue(contourValues, "contour", "extraArgs"))
		})
	}
}

// Test_Helm_UpgradeRadius_ContourUpgradeBaseline verifies that the check uses the release revision that Helm's upgrade
// starts from: Helm reuses its values and deletes the resources in its manifest that the chart no longer renders.
func Test_Helm_UpgradeRadius_ContourUpgradeBaseline(t *testing.T) {
	t.Parallel()

	turnOffFeatures := map[string]any{
		"gatewayAPI": map[string]any{"manageCRDs": false},
		"contour": map[string]any{
			"extraArgs": []any{"--disable-feature=tlsroutes", "--disable-feature=tcproutes", "--disable-feature=backendtlspolicies"},
		},
	}
	revision := func(version int, status releasecommon.Status, config map[string]any, crds ...crdInfo) *releasev1.Release {
		return &releasev1.Release{
			Name:     contourReleaseName,
			Version:  version,
			Chart:    newTestContourChart(),
			Info:     &releasev1.Info{Status: status},
			Config:   copyValues(config),
			Manifest: crdManifest(crds...),
		}
	}
	// listenerSets is a Gateway API CRD that a custom Contour chart installed and that the default chart does not.
	listenerSets := newHelmOwnedCRD(contourReleaseName, RadiusSystemNamespace, "listenersets", "v1")

	tests := []struct {
		name string
		// crds are the CRDs in the cluster.
		crds    []crdInfo
		history []*releasev1.Release
		// wantErr is empty when the upgrade succeeds.
		wantErr string
	}{
		{
			// Only the deployed revision has the values that turn off the features that the Traefik CRDs lack.
			name:    "failed latest revision on top of a deployed one uses the deployed values",
			crds:    traefikGatewayAPICRDs(),
			history: []*releasev1.Release{revision(1, releasecommon.StatusDeployed, turnOffFeatures), revision(2, releasecommon.StatusFailed, nil)},
		},
		{
			// Only the deployed revision installed listenerSets, so only its manifest makes Helm delete the CRD.
			name: "failed latest revision on top of a deployed one uses the deployed manifest",
			crds: append(contourBundleCRDs(), listenerSets),
			history: []*releasev1.Release{
				revision(1, releasecommon.StatusDeployed, nil, append(contourBundleCRDs(), listenerSets)...),
				revision(2, releasecommon.StatusFailed, nil, contourBundleCRDs()...),
			},
			wantErr: "the Contour chart no longer includes these CRDs, which Contour release \"contour\" in namespace \"radius-system\" installed:\n  - listenersets.gateway.networking.k8s.io\n",
		},
		{
			name:    "history with only a failed revision uses that revision",
			crds:    traefikGatewayAPICRDs(),
			history: []*releasev1.Release{revision(1, releasecommon.StatusFailed, turnOffFeatures)},
		},
		{
			name:    "failed latest revision without a deployed one uses the latest revision",
			crds:    traefikGatewayAPICRDs(),
			history: []*releasev1.Release{revision(1, releasecommon.StatusFailed, turnOffFeatures), revision(2, releasecommon.StatusFailed, nil)},
			wantErr: "tlsroutes.gateway.networking.k8s.io v1alpha2 (the CRD is not installed)",
		},
		{
			name:    "pending latest revision",
			crds:    traefikGatewayAPICRDs(),
			history: []*releasev1.Release{revision(1, releasecommon.StatusDeployed, turnOffFeatures), revision(2, releasecommon.StatusPendingUpgrade, nil)},
			wantErr: "another operation is in progress on Contour release \"contour\" in namespace \"radius-system\": revision 2 is pending-upgrade",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := NewMockHelmClient(gomock.NewController(t))
			options := NewDefaultClusterOptions()
			expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
			expectContourHistory(client, options, tt.history...)
			// Radius reads the history before it downloads the Contour chart, so a pending operation stops it earlier.
			if !revisionStatus(tt.history[len(tt.history)-1]).IsPending() {
				expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
			}
			if tt.wantErr == "" {
				client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), options.Radius.ReleaseName, options.Radius.Namespace, true, true, gomock.Any()).
					Return(&releasev1.Release{}, nil).Times(1)
				client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), options.Contour.ReleaseName, options.Contour.Namespace, false, true, gomock.Any()).
					Return(&releasev1.Release{}, nil).Times(1)
			}
			// Otherwise no upgrade is expected.

			impl := &Impl{
				Helm:                           client,
				configureDefaultContourGateway: (&defaultContourGatewayCalls{}).configure,
				contourCluster:                 staticCRDs(tt.crds...),
			}

			err := impl.UpgradeRadius(t.Context(), options, "test-context")
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			require.True(t, clierrors.IsFriendlyError(err), "the CLI shows the message as is: %v", err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// Test_Helm_InstallRadius_ReinstallContourUsesUpgradeBaseline verifies that reinstalling an existing Contour release
// checks the revision that Helm's upgrade starts from.
func Test_Helm_InstallRadius_ReinstallContourUsesUpgradeBaseline(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	options.Contour.Reinstall = true
	deployed := newContourReleaseWithStoredValues(options)
	// The failed revision lost the values that turn off the features that the Traefik CRDs lack.
	failed := &releasev1.Release{
		Name:    options.Contour.ReleaseName,
		Version: 2,
		Chart:   newTestContourChart(),
		Info:    &releasev1.Info{Status: releasecommon.StatusFailed},
	}

	expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
	client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(failed, nil).Times(2)
	client.EXPECT().RunHelmHistory(gomock.Any(), options.Contour.ReleaseName).Return([]*releasev1.Release{deployed, failed}, nil).Times(1)
	expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
	client.EXPECT().RunHelmGet(gomock.Any(), options.Radius.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(1)
	client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), options.Radius.ReleaseName, gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&releasev1.Release{}, nil).Times(1)
	client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), options.Contour.ReleaseName, options.Contour.Namespace, false, true, gomock.Any()).
		Return(&releasev1.Release{}, nil).Times(1)

	impl := &Impl{
		Helm:                           client,
		configureDefaultContourGateway: (&defaultContourGatewayCalls{}).configure,
		contourCluster:                 staticCRDs(traefikGatewayAPICRDs()...),
	}
	require.NoError(t, impl.InstallRadius(t.Context(), options, "test-context"))
}

// Test_Helm_UpgradeRadius_RefusesToDeleteCRDs verifies that an upgrade from a Contour chart that installed a Gateway
// API CRD to a chart without it stops before it changes anything, because Helm would delete the CRD.
func Test_Helm_UpgradeRadius_RefusesToDeleteCRDs(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	listenerSets := newHelmOwnedCRD(contourReleaseName, RadiusSystemNamespace, "listenersets", "v1")
	customChartRelease := &releasev1.Release{
		Name:     options.Contour.ReleaseName,
		Version:  1,
		Chart:    newStubContourChart("1.32.0", append(contourBundleCRDs(), listenerSets)),
		Info:     &releasev1.Info{Status: releasecommon.StatusDeployed},
		Manifest: crdManifest(append(contourBundleCRDs(), listenerSets)...),
	}

	expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
	expectContourHistory(client, options, customChartRelease)
	expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
	// No upgrade is expected.

	impl := &Impl{
		Helm:           client,
		contourCluster: staticCRDs(append(contourBundleCRDs(), listenerSets)...),
	}

	err := impl.UpgradeRadius(t.Context(), options, "test-context")
	require.Error(t, err)
	require.True(t, clierrors.IsFriendlyError(err))
	require.Contains(t, err.Error(), "the Contour chart no longer includes these CRDs")
	require.Contains(t, err.Error(), "  - listenersets.gateway.networking.k8s.io\n")
	require.Contains(t, err.Error(), `kubectl --context "test-context" annotate crd listenersets.gateway.networking.k8s.io helm.sh/resource-policy=keep`)
}

func Test_Helm_CheckContourUpgrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		disabled    bool
		installed   bool
		resetValues bool
		wantUpgrade bool
		wantErr     string
	}{
		{
			name:     "Contour disabled",
			disabled: true,
		},
		{
			name: "Contour not installed",
		},
		{
			name:        "Contour installed",
			installed:   true,
			wantUpgrade: true,
		},
		{
			name:        "Contour installed and --reset-values discards the values it needs",
			installed:   true,
			resetValues: true,
			wantErr:     "Radius stopped before changing the cluster because Contour 1.32.0 cannot use the Gateway API CRDs in this cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := NewMockHelmClient(gomock.NewController(t))
			options := NewDefaultClusterOptions()
			options.Contour.Disabled = tt.disabled
			options.ResetValues = tt.resetValues
			switch {
			case tt.disabled:
			case tt.installed:
				expectContourHistory(client, options, newContourReleaseWithStoredValues(options))
				expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
			default:
				client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(1)
			}
			// No install or upgrade is expected.

			impl := &Impl{Helm: client, contourCluster: staticCRDs(traefikGatewayAPICRDs()...)}
			upgrade, err := impl.CheckContourUpgrade(t.Context(), options, "test-context")
			if tt.wantErr != "" {
				require.True(t, clierrors.IsFriendlyError(err), "the CLI shows the message as is: %v", err)
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantUpgrade, upgrade)
		})
	}
}

// Test_Helm_InstallRadius_InstallsContourBeforeRadius verifies that InstallRadius installs Contour right after its
// checks and before it waits for Radius, so that the Gateway API CRDs cannot change between the checks and the
// Contour install while Radius starts.
func Test_Helm_InstallRadius_InstallsContourBeforeRadius(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
	expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
	client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(2)
	client.EXPECT().RunHelmGet(gomock.Any(), options.Radius.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(1)
	gatewayCalls := &defaultContourGatewayCalls{}
	gomock.InOrder(
		client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), options.Contour.ReleaseName, options.Contour.Namespace, false, gomock.Any()).
			Return(&releasev1.Release{}, nil).Times(1),
		client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), options.Radius.ReleaseName, options.Radius.Namespace, true, gomock.Any()).
			DoAndReturn(func(*helm.Configuration, *chart.Chart, map[string]any, string, string, bool, time.Duration) (*releasev1.Release, error) {
				require.Zero(t, gatewayCalls.configureCalls, "the Gateway is configured after Radius is installed")
				return &releasev1.Release{}, nil
			}).Times(1),
	)

	impl := &Impl{
		Helm:                           client,
		configureDefaultContourGateway: gatewayCalls.configure,
		contourCluster:                 staticCRDs(),
	}
	require.NoError(t, impl.InstallRadius(t.Context(), options, "test-context"))
	require.Equal(t, 1, gatewayCalls.configureCalls)
}

// Test_Helm_InstallRadius_ContourInstallFailureInstallsNoRadius verifies that a failed Contour install leaves Radius
// uninstalled, so that running the command again starts from the same state.
func Test_Helm_InstallRadius_ContourInstallFailureInstallsNoRadius(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
	expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
	client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(2)
	client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), options.Contour.ReleaseName, gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New(`CustomResourceDefinition "gateways.gateway.networking.k8s.io" exists and cannot be imported into the current release: invalid ownership metadata`)).
		Times(1)
	// Radius is neither queried nor installed.

	gatewayCalls := &defaultContourGatewayCalls{}
	impl := &Impl{
		Helm:                           client,
		configureDefaultContourGateway: gatewayCalls.configure,
		contourCluster:                 staticCRDs(),
	}
	err := impl.InstallRadius(t.Context(), options, "test-context")
	require.ErrorContains(t, err, "failed to apply Contour Helm chart")
	require.Zero(t, gatewayCalls.configureCalls)
}

// Test_Helm_InstallRadius_ContourInstallUsesClusterConfiguration verifies that the Contour install gets a Helm
// configuration for the cluster. The checks render the chart with a client-only install, which replaces the
// capabilities, Kubernetes client, and release storage of its configuration with offline ones, so the checks must not
// share their configuration with the install.
func Test_Helm_InstallRadius_ContourInstallUsesClusterConfiguration(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
	expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
	client.EXPECT().RunHelmGet(gomock.Any(), gomock.Any()).Return(nil, driver.ErrReleaseNotFound).Times(3)
	client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), options.Radius.ReleaseName, gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&releasev1.Release{}, nil).Times(1)
	var contourConfiguration *helm.Configuration
	client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), options.Contour.ReleaseName, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(helmConf *helm.Configuration, _ *chart.Chart, _ map[string]any, _ string, _ string, _ bool, _ time.Duration) (*releasev1.Release, error) {
			contourConfiguration = helmConf
			return &releasev1.Release{}, nil
		}).Times(1)

	impl := &Impl{
		Helm:                           client,
		configureDefaultContourGateway: (&defaultContourGatewayCalls{}).configure,
		contourCluster:                 staticCRDs(),
	}
	require.NoError(t, impl.InstallRadius(t.Context(), options, "test-context"))

	require.NotNil(t, contourConfiguration)
	require.Nil(t, contourConfiguration.Capabilities, "Helm discovers the capabilities of the cluster")
	require.IsType(t, &kube.Client{}, contourConfiguration.KubeClient)
	require.Equal(t, driver.SecretsDriverName, contourConfiguration.Releases.Driver.Name())
}

// Test_Helm_UpgradeRadius_DownloadsContourChartOnce verifies that the upgrade reuses the Contour chart that its
// preflight check downloaded, and still checks the upgrade itself, for example after --skip-preflight.
func Test_Helm_UpgradeRadius_DownloadsContourChartOnce(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	contourRelease := newContourReleaseWithStoredValues(options)
	expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
	expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
	client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(contourRelease, nil).Times(2)
	client.EXPECT().RunHelmHistory(gomock.Any(), options.Contour.ReleaseName).Return([]*releasev1.Release{contourRelease}, nil).Times(2)
	client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), options.Radius.ReleaseName, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&releasev1.Release{}, nil).Times(1)
	client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), options.Contour.ReleaseName, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&releasev1.Release{}, nil).Times(1)

	var checks int
	impl := &Impl{
		Helm:                           client,
		configureDefaultContourGateway: (&defaultContourGatewayCalls{}).configure,
		contourCluster: &fakeContourCluster{readCRDs: func(ctx context.Context, kubeContext string, names []string) ([]crdInfo, error) {
			checks++
			return staticCRDs(traefikGatewayAPICRDs()...).CRDs(ctx, kubeContext, names)
		}},
	}

	upgrade, err := impl.CheckContourUpgrade(t.Context(), options, "test-context")
	require.NoError(t, err)
	require.True(t, upgrade)
	require.NoError(t, impl.UpgradeRadius(t.Context(), options, "test-context"))
	require.Equal(t, 2, checks, "the upgrade checks Contour again")
}

// Test_Helm_UpgradeRadius_WaitsForK3sOnce reproduces rad upgrade kubernetes on a k3s cluster where Contour owns the
// Gateway API CRDs, as v0.62.0 could leave it. k3s cannot install its Traefik CRDs, so their release never appears.
// Radius does not wait for it, and warns once although the preflight check and the upgrade both check Contour.
func Test_Helm_UpgradeRadius_WaitsForK3sOnce(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		client := NewMockHelmClient(gomock.NewController(t))
		options := NewDefaultClusterOptions()
		// The Contour release of v0.62.0 manages the Gateway API CRDs.
		contourRelease := &releasev1.Release{
			Name:     options.Contour.ReleaseName,
			Version:  1,
			Chart:    newTestContourChart(),
			Info:     &releasev1.Info{Status: releasecommon.StatusDeployed},
			Config:   map[string]any{"gatewayAPI": map[string]any{"manageCRDs": true}},
			Manifest: crdManifest(contourBundleCRDs()...),
		}
		expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
		expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
		client.EXPECT().RunHelmGet(gomock.Any(), "traefik-crd").Return(nil, driver.ErrReleaseNotFound).Times(1)
		client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(contourRelease, nil).Times(2)
		client.EXPECT().RunHelmHistory(gomock.Any(), options.Contour.ReleaseName).Return([]*releasev1.Release{contourRelease}, nil).Times(2)
		client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), options.Radius.ReleaseName, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&releasev1.Release{}, nil).Times(1)
		client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), options.Contour.ReleaseName, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&releasev1.Release{}, nil).Times(1)

		var logs []string
		impl := &Impl{
			Helm:                           client,
			configureDefaultContourGateway: (&defaultContourGatewayCalls{}).configure,
			contourCluster: &fakeContourCluster{
				crds:       contourBundleCRDs(),
				helmCharts: []helmControllerChart{{name: "traefik-crd", namespace: "kube-system", releaseNamespace: "kube-system", jobName: "helm-install-traefik-crd"}},
				jobs:       map[string]helmControllerJob{"kube-system/helm-install-traefik-crd": {}},
			},
			logger: func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) },
		}

		start := time.Now()
		upgrade, err := impl.CheckContourUpgrade(t.Context(), options, "test-context")
		require.NoError(t, err)
		require.True(t, upgrade)
		require.NoError(t, impl.UpgradeRadius(t.Context(), options, "test-context"))
		require.Zero(t, time.Since(start), "Radius must not wait for an install that k3s cannot do")
		require.Len(t, logs, 1, "%q", logs)
		require.Contains(t, logs[0], `Warning: k3s cannot install the Traefik CRDs of HelmChart kube-system/traefik-crd because CRD backendtlspolicies.gateway.networking.k8s.io belongs to Helm release "contour" in namespace "radius-system"`)
	})
}

// Test_Helm_InstallRadius_WaitsForK3sTraefikCRDs reproduces rad install kubernetes right after k3d cluster create: k3s
// installs its Traefik CRDs while Radius checks the cluster. Radius waits for k3s, so that it decides with the CRDs of
// Traefik instead of letting Contour install its own CRDs at the same time.
func Test_Helm_InstallRadius_WaitsForK3sTraefikCRDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		withFlag bool
		// k3sDeploys is false when k3s never finishes installing its CRDs.
		k3sDeploys bool
	}{
		{name: "k3s installs its CRDs while Radius waits", k3sDeploys: true},
		{name: "k3s installs its CRDs while Radius waits, with the suggested flag", withFlag: true, k3sDeploys: true},
		{name: "k3s does not finish"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				client := NewMockHelmClient(gomock.NewController(t))
				options := NewDefaultClusterOptions()
				if tt.withFlag {
					options.Contour.SetArgs = []string{turnOffFeaturesForTraefikCRDs}
				}
				expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
				expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())

				// k3s deploys its traefik-crd release, with the Traefik CRDs, between the first and the second look.
				k3sLooks := 0
				k3sCall := client.EXPECT().RunHelmGet(gomock.Any(), "traefik-crd").
					DoAndReturn(func(*helm.Configuration, string) (*releasev1.Release, error) {
						k3sLooks++
						if k3sLooks == 1 || !tt.k3sDeploys {
							return nil, driver.ErrReleaseNotFound
						}
						return &releasev1.Release{Name: "traefik-crd", Version: 1, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}}, nil
					})
				if tt.k3sDeploys {
					k3sCall.Times(2)
				} else {
					k3sCall.MinTimes(2)
				}
				cluster := &fakeContourCluster{
					helmCharts: []helmControllerChart{{name: "traefik-crd", namespace: "kube-system", releaseNamespace: "kube-system"}},
					readCRDs: func(ctx context.Context, kubeContext string, names []string) ([]crdInfo, error) {
						if k3sLooks < 2 {
							return nil, nil
						}
						return staticCRDs(traefikGatewayAPICRDs()...).CRDs(ctx, kubeContext, names)
					},
				}

				var contourValues map[string]any
				if tt.withFlag {
					client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(2)
					client.EXPECT().RunHelmGet(gomock.Any(), options.Radius.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(1)
					client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), options.Contour.ReleaseName, gomock.Any(), gomock.Any(), gomock.Any()).
						DoAndReturn(func(_ *helm.Configuration, _ *chart.Chart, vals map[string]any, _ string, _ string, _ bool, _ time.Duration) (*releasev1.Release, error) {
							contourValues = vals
							return &releasev1.Release{}, nil
						}).Times(1)
					client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), options.Radius.ReleaseName, gomock.Any(), gomock.Any(), gomock.Any()).
						Return(&releasev1.Release{}, nil).Times(1)
				} else {
					client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(1)
					// Nothing is installed.
				}

				impl := &Impl{
					Helm:                           client,
					configureDefaultContourGateway: (&defaultContourGatewayCalls{}).configure,
					contourCluster:                 cluster,
				}
				start := time.Now()
				err := impl.InstallRadius(t.Context(), options, "test-context")

				if !tt.k3sDeploys {
					require.Equal(t, k3sCRDWaitTimeout, time.Since(start))
					require.True(t, clierrors.IsFriendlyError(err), "%v", err)
					require.ErrorContains(t, err, "k3s has not finished installing the Traefik CRDs after 3m0s")
					return
				}
				require.Equal(t, k3sCRDWaitInterval, time.Since(start), "Radius waits for k3s once")
				if tt.withFlag {
					require.NoError(t, err)
					require.Equal(t, false, nestedValue(contourValues, "gatewayAPI", "manageCRDs"))
					return
				}
				require.True(t, clierrors.IsFriendlyError(err), "%v", err)
				require.ErrorContains(t, err, "tlsroutes.gateway.networking.k8s.io v1alpha2 (the CRD is not installed)")
				require.ErrorContains(t, err, `belong to Helm release "traefik-crd" in namespace "kube-system"`)
			})
		})
	}
}

// Test_Helm_InstallRadius_ExistingContourRelease verifies how rad install kubernetes treats the Contour release that
// an earlier command left. It repairs a failed release before it installs Radius, explains how to recover a release
// with a pending operation, and leaves a working release unchanged.
func Test_Helm_InstallRadius_ExistingContourRelease(t *testing.T) {
	t.Parallel()

	revision := func(version int, status releasecommon.Status) *releasev1.Release {
		return &releasev1.Release{
			Name:     contourReleaseName,
			Version:  version,
			Chart:    newTestContourChart(),
			Info:     &releasev1.Info{Status: status},
			Config:   map[string]any{"gatewayAPI": map[string]any{"manageCRDs": true}},
			Manifest: crdManifest(contourBundleCRDs()...),
		}
	}

	tests := []struct {
		name    string
		history []*releasev1.Release
		// wantRepair is true when Radius upgrades the Contour release before it installs Radius.
		wantRepair bool
		// wantErr is the friendly error, or empty when Radius is installed.
		wantErr string
	}{
		{
			name:    "deployed release",
			history: []*releasev1.Release{revision(1, releasecommon.StatusDeployed)},
		},
		{
			name:       "failed install",
			history:    []*releasev1.Release{revision(1, releasecommon.StatusFailed)},
			wantRepair: true,
		},
		{
			name:       "failed upgrade on top of a deployed revision",
			history:    []*releasev1.Release{revision(1, releasecommon.StatusDeployed), revision(2, releasecommon.StatusFailed)},
			wantRepair: true,
		},
		{
			name:    "pending install",
			history: []*releasev1.Release{revision(1, releasecommon.StatusPendingInstall)},
			wantErr: `kubectl --context "test-context" -n radius-system delete secret sh.helm.release.v1.contour.v1`,
		},
		{
			name:    "pending upgrade on top of a deployed revision",
			history: []*releasev1.Release{revision(1, releasecommon.StatusDeployed), revision(2, releasecommon.StatusPendingUpgrade)},
			wantErr: `helm --kube-context "test-context" -n radius-system rollback contour 1`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := NewMockHelmClient(gomock.NewController(t))
			options := NewDefaultClusterOptions()
			latest := tt.history[len(tt.history)-1]
			expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
			gatewayCalls := &defaultContourGatewayCalls{}
			var contourValues map[string]any
			switch {
			case tt.wantErr != "":
				client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(latest, nil).Times(1)
				client.EXPECT().RunHelmHistory(gomock.Any(), options.Contour.ReleaseName).Return(tt.history, nil).Times(1)
				// Nothing is installed.
			case tt.wantRepair:
				// Radius plans the repair, and ApplyHelmChart checks the release again.
				client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(latest, nil).Times(2)
				client.EXPECT().RunHelmHistory(gomock.Any(), options.Contour.ReleaseName).Return(tt.history, nil).Times(1)
				expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
				client.EXPECT().RunHelmGet(gomock.Any(), options.Radius.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(1)
				gomock.InOrder(
					client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), options.Contour.ReleaseName, options.Contour.Namespace, false, true, gomock.Any()).
						DoAndReturn(func(_ *helm.Configuration, _ *chart.Chart, vals map[string]any, _ string, _ string, _ bool, _ bool, _ time.Duration) (*releasev1.Release, error) {
							contourValues = vals
							return &releasev1.Release{}, nil
						}).Times(1),
					client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), options.Radius.ReleaseName, gomock.Any(), gomock.Any(), gomock.Any()).
						Return(&releasev1.Release{}, nil).Times(1),
				)
			default:
				client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(latest, nil).Times(1)
				client.EXPECT().RunHelmGet(gomock.Any(), options.Radius.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(1)
				client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), options.Radius.ReleaseName, gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&releasev1.Release{}, nil).Times(1)
				// The Contour release is neither read nor changed.
			}

			impl := &Impl{
				Helm:                           client,
				configureDefaultContourGateway: gatewayCalls.configure,
				contourCluster:                 staticCRDs(contourBundleCRDs()...),
			}
			err := impl.InstallRadius(t.Context(), options, "test-context")
			if tt.wantErr != "" {
				require.True(t, clierrors.IsFriendlyError(err), "%v", err)
				require.ErrorContains(t, err, "another operation is in progress on Contour release")
				require.ErrorContains(t, err, tt.wantErr)
				require.Zero(t, gatewayCalls.configureCalls)
				return
			}

			require.NoError(t, err)
			require.Equal(t, 1, gatewayCalls.configureCalls)
			if tt.wantRepair {
				// Contour keeps managing the CRDs that the release installed.
				require.Equal(t, true, nestedValue(contourValues, "gatewayAPI", "manageCRDs"))
			}
		})
	}
}

// Test_Helm_ContourOptionsReachHelm fails when rad install kubernetes or rad upgrade kubernetes passes a Contour option
// to InstallRadius or UpgradeRadius and the option does not change what Radius asks Helm to do, as InstallRadius
// ignored --contour-set and --contour-set-file.
func Test_Helm_ContourOptionsReachHelm(t *testing.T) {
	t.Parallel()

	setFile := filepath.Join(t.TempDir(), "value.txt")
	require.NoError(t, os.WriteFile(setFile, []byte("from-file"), 0600))
	samples := map[string]any{
		"ChartPath":    "custom-contour-chart",
		"ChartRepo":    "https://charts.example.com",
		"ChartVersion": "9.9.9",
		"SetArgs":      []string{"contour.replicaCount=3"},
		"SetFileArgs":  []string{"envoy.service.annotations.example=" + setFile},
	}

	for _, upgrade := range []bool{false, true} {
		operation := "InstallRadius"
		if upgrade {
			operation = "UpgradeRadius"
		}

		t.Run(operation, func(t *testing.T) {
			t.Parallel()

			defaults := PopulateDefaultClusterOptions(CLIClusterOptions{})
			baseline := recordHelmCalls(t, defaults, upgrade)

			for _, field := range reflect.VisibleFields(reflect.TypeFor[ChartOptions]()) {
				t.Run(field.Name, func(t *testing.T) {
					t.Parallel()

					cliOptions := CLIClusterOptions{}
					value := reflect.ValueOf(&cliOptions.Contour).Elem().FieldByIndex(field.Index)
					if sample, found := samples[field.Name]; found {
						value.Set(reflect.ValueOf(sample))
					} else {
						switch value.Kind() {
						case reflect.Bool:
							value.SetBool(true)
						case reflect.String:
							value.SetString("sample")
						case reflect.Int64:
							value.SetInt(int64(time.Minute))
						default:
							t.Fatalf("add a sample value for ChartOptions.%s", field.Name)
						}
					}

					options := PopulateDefaultClusterOptions(cliOptions)
					if reflect.DeepEqual(defaults, options) {
						t.Skipf("PopulateDefaultClusterOptions does not pass ChartOptions.%s for Contour", field.Name)
					}

					require.NotEqual(t, baseline, recordHelmCalls(t, options, upgrade), "%s ignores the Contour option %s", operation, field.Name)
				})
			}
		})
	}
}

// recordHelmCalls runs InstallRadius on a fresh cluster, or UpgradeRadius on a cluster with Contour installed, and
// records what it asks Helm to do.
func recordHelmCalls(t *testing.T, options ClusterOptions, upgrade bool) []string {
	t.Helper()

	var calls []string
	contourRelease := &releasev1.Release{Name: contourReleaseName, Version: 1, Chart: newTestContourChart(), Info: &releasev1.Info{Status: releasecommon.StatusDeployed}}
	client := NewMockHelmClient(gomock.NewController(t))
	client.EXPECT().RunHelmPull(gomock.Any(), gomock.Any()).
		DoAndReturn(func(pullopts []helm.PullOpt, chartRef string) (string, error) {
			pull := helm.NewPull(pullopts...)
			calls = append(calls, fmt.Sprintf("pull %s version=%s repo=%s", chartRef, pull.Version, pull.RepoURL))
			require.NoError(t, os.WriteFile(filepath.Join(pull.DestDir, "pulled.tgz"), []byte("chart"), 0600))
			return "Pulled", nil
		}).AnyTimes()
	client.EXPECT().LoadChart(gomock.Any()).
		DoAndReturn(func(chartPath string) (*chart.Chart, error) {
			if filepath.Base(chartPath) == "pulled.tgz" {
				chartPath = "the pulled chart"
			}
			calls = append(calls, "load "+chartPath)
			return newTestContourChart(), nil
		}).AnyTimes()
	client.EXPECT().RunHelmGet(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ *helm.Configuration, releaseName string) (*releasev1.Release, error) {
			if upgrade && releaseName == contourReleaseName {
				return contourRelease, nil
			}
			return nil, driver.ErrReleaseNotFound
		}).AnyTimes()
	client.EXPECT().RunHelmHistory(gomock.Any(), contourReleaseName).Return([]*releasev1.Release{contourRelease}, nil).AnyTimes()
	client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ *helm.Configuration, _ *chart.Chart, vals map[string]any, releaseName string, namespace string, wait bool, timeout time.Duration) (*releasev1.Release, error) {
			calls = append(calls, fmt.Sprintf("install %s namespace=%s wait=%t timeout=%s values=%v", releaseName, namespace, wait, timeout, vals))
			return &releasev1.Release{}, nil
		}).AnyTimes()
	client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ *helm.Configuration, _ *chart.Chart, vals map[string]any, releaseName string, namespace string, wait bool, reuseValues bool, timeout time.Duration) (*releasev1.Release, error) {
			calls = append(calls, fmt.Sprintf("upgrade %s namespace=%s wait=%t reuse=%t timeout=%s values=%v", releaseName, namespace, wait, reuseValues, timeout, vals))
			return &releasev1.Release{}, nil
		}).AnyTimes()

	impl := &Impl{
		Helm:                           client,
		configureDefaultContourGateway: (&defaultContourGatewayCalls{}).configure,
		contourCluster:                 staticCRDs(),
	}
	if upgrade {
		require.NoError(t, impl.UpgradeRadius(t.Context(), options, "test-context"))
	} else {
		require.NoError(t, impl.InstallRadius(t.Context(), options, "test-context"))
	}

	return calls
}

// newContourReleaseWithStoredValues returns a Contour release that an earlier rad install kubernetes installed on a
// k3s cluster with Traefik, with the Contour values that turn off the features that the Traefik CRDs lack.
func newContourReleaseWithStoredValues(options ClusterOptions) *releasev1.Release {
	return &releasev1.Release{
		Name:    options.Contour.ReleaseName,
		Version: 1,
		Chart:   newTestContourChart(),
		Info:    &releasev1.Info{Status: releasecommon.StatusDeployed},
		Config: map[string]any{
			"gatewayAPI": map[string]any{"manageCRDs": false},
			"contour": map[string]any{
				"extraArgs": []any{"--disable-feature=tlsroutes", "--disable-feature=tcproutes", "--disable-feature=backendtlspolicies"},
			},
		},
	}
}

// expectContourHistory expects one check that the Contour release exists, which returns its latest revision, and one
// read of its history.
func expectContourHistory(client *MockHelmClient, options ClusterOptions, history ...*releasev1.Release) {
	latest := history[len(history)-1]
	client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(latest, nil).Times(1)
	client.EXPECT().RunHelmHistory(gomock.Any(), options.Contour.ReleaseName).Return(history, nil).Times(1)
}

// fakeContourCluster is the cluster state that the Contour checks read, without a cluster.
type fakeContourCluster struct {
	crds []crdInfo

	// readCRDs reads the CRDs instead of crds when it is set.
	readCRDs func(ctx context.Context, kubeContext string, names []string) ([]crdInfo, error)

	// crdReads counts the reads of CRDs.
	crdReads atomic.Int32

	helmCharts    []helmControllerChart
	helmChartsErr error

	// jobs and jobPods are the Jobs and their Pods by namespace/name. A Job that is not in jobs does not exist. jobErr
	// and jobPodsErr make the reads fail instead.
	jobs       map[string]helmControllerJob
	jobErr     error
	jobPods    map[string]helmControllerPods
	jobPodsErr error

	// capabilities are the capabilities of the cluster, or nil for Helm's default capabilities.
	capabilities *common.Capabilities
}

func (f *fakeContourCluster) CRDs(ctx context.Context, kubeContext string, names []string) ([]crdInfo, error) {
	f.crdReads.Add(1)
	if f.readCRDs != nil {
		return f.readCRDs(ctx, kubeContext, names)
	}
	return crdLookup(f.crds)(ctx, names)
}

func (f *fakeContourCluster) HelmCharts(_ context.Context, _ string, namespace string, names []string) ([]helmControllerChart, error) {
	if f.helmChartsErr != nil {
		return nil, f.helmChartsErr
	}
	var charts []helmControllerChart
	for _, chart := range f.helmCharts {
		if chart.namespace == namespace && slices.Contains(names, chart.name) {
			charts = append(charts, chart)
		}
	}
	return charts, nil
}

func (f *fakeContourCluster) Job(_ context.Context, _ string, namespace string, name string) (helmControllerJob, error) {
	if f.jobErr != nil {
		return helmControllerJob{}, f.jobErr
	}
	job, found := f.jobs[namespace+"/"+name]
	if !found {
		return helmControllerJob{}, apierrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, name)
	}
	return job, nil
}

func (f *fakeContourCluster) JobPods(_ context.Context, _ string, namespace string, job string) (helmControllerPods, error) {
	if f.jobPodsErr != nil {
		return helmControllerPods{}, f.jobPodsErr
	}
	return f.jobPods[namespace+"/"+job], nil
}

func (f *fakeContourCluster) RenderConfiguration(string, string) (*helm.Configuration, error) {
	return newTestRenderConfiguration(f.capabilities), nil
}

// staticCRDs returns the state of a cluster without k3s HelmCharts that has crds.
func staticCRDs(crds ...crdInfo) *fakeContourCluster {
	return &fakeContourCluster{crds: crds}
}

func radiusChartRef(options ClusterOptions) string {
	return fmt.Sprintf("%s/%s", options.Radius.ChartRepo, options.Radius.ReleaseName)
}

// expectChartPull expects one pull of chartRef and returns loaded as the pulled chart.
func expectChartPull(t *testing.T, client *MockHelmClient, chartRef string, loaded *chart.Chart) {
	t.Helper()

	fileName := path.Base(chartRef) + ".tgz"
	client.EXPECT().RunHelmPull(gomock.Any(), chartRef).
		DoAndReturn(func(pullopts []helm.PullOpt, _ string) (string, error) {
			pull := helm.NewPull(pullopts...)
			require.NoError(t, os.WriteFile(filepath.Join(pull.DestDir, fileName), []byte("chart"), 0600))
			return "Pulled", nil
		}).Times(1)
	client.EXPECT().LoadChart(gomock.Cond(func(chartPath string) bool { return filepath.Base(chartPath) == fileName })).
		Return(loaded, nil).Times(1)
}
