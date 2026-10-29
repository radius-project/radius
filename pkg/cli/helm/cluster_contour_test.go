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
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
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
		crds         []gatewayAPICRD
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
			contourChart: &chart.Chart{Metadata: &chart.Metadata{Name: "contour", Version: "9.9.9", AppVersion: "9.9.9"}},
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
				listGatewayAPICRDs:             staticGatewayAPICRDs(tt.crds...),
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
		listGatewayAPICRDs:             staticGatewayAPICRDs(traefikGatewayAPICRDs()...),
	}

	err := impl.InstallRadius(t.Context(), options, "test-context")
	require.Error(t, err)
	require.True(t, clierrors.IsFriendlyError(err), "rad install kubernetes must show the message as is: %v", err)
	require.Contains(t, err.Error(), "tlsroutes.gateway.networking.k8s.io v1alpha2 (the CRD is not installed)")
	require.Contains(t, err.Error(), "--contour-set '"+turnOffFeaturesForTraefikCRDs+"'")
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
		Manifest: gatewayAPICRDManifest(contourBundleCRDNames()...),
	}
	client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(contourRelease, nil).Times(1)
	radiusRelease := &releasev1.Release{Name: options.Radius.ReleaseName, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}}}
	client.EXPECT().RunHelmGet(gomock.Any(), options.Radius.ReleaseName).Return(radiusRelease, nil).Times(1)
	client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), options.Radius.ReleaseName, options.Radius.Namespace, true, true, gomock.Any()).
		Return(radiusRelease, nil).Times(1)
	// No Contour chart pull, install, or upgrade is expected.

	gatewayCalls := &defaultContourGatewayCalls{}
	impl := &Impl{
		Helm:                           client,
		configureDefaultContourGateway: gatewayCalls.configure,
		listGatewayAPICRDs: func(context.Context, string) ([]gatewayAPICRD, error) {
			t.Error("the Gateway API CRDs must not be checked when Contour does not change")
			return traefikGatewayAPICRDs(), nil
		},
	}

	require.NoError(t, impl.InstallRadius(t.Context(), options, "test-context"))
	require.Equal(t, 1, gatewayCalls.configureCalls)
}

func Test_Helm_InstallRadius_ListGatewayAPICRDsError(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
	expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
	client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(1)

	impl := &Impl{
		Helm: client,
		listGatewayAPICRDs: func(context.Context, string) ([]gatewayAPICRD, error) {
			return nil, errors.New("customresourcedefinitions is forbidden")
		},
	}

	err := impl.InstallRadius(t.Context(), options, "test-context")
	require.ErrorContains(t, err, "failed to list Gateway API CRDs")
	require.ErrorContains(t, err, "customresourcedefinitions is forbidden")
}

// Test_Helm_UpgradeRadius_ReusesStoredContourValues verifies that the check uses the Contour values that Helm keeps
// from the release, because rad upgrade kubernetes has no flag to set them again.
func Test_Helm_UpgradeRadius_ReusesStoredContourValues(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	contourRelease := newContourReleaseWithStoredValues(options)

	expectChartPull(t, client, radiusChartRef(options), &chart.Chart{})
	client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(contourRelease, nil).Times(1)
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
		listGatewayAPICRDs:             staticGatewayAPICRDs(traefikGatewayAPICRDs()...),
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
	client.EXPECT().RunHelmGet(gomock.Any(), options.Contour.ReleaseName).Return(newContourReleaseWithStoredValues(options), nil).Times(1)
	expectChartPull(t, client, options.Contour.ReleaseName, newTestContourChart())
	// No upgrade is expected.

	impl := &Impl{
		Helm:               client,
		listGatewayAPICRDs: staticGatewayAPICRDs(traefikGatewayAPICRDs()...),
	}

	err := impl.UpgradeRadius(t.Context(), options, "test-context")
	require.Error(t, err)
	require.True(t, clierrors.IsFriendlyError(err))
	require.Contains(t, err.Error(), "tcproutes.gateway.networking.k8s.io v1alpha2 (the CRD is not installed)")
	require.Contains(t, err.Error(), "run the upgrade without --reset-values")
}

// Test_Helm_InstallRadius_ContourOptionsReachHelm fails when rad install kubernetes passes a Contour option to
// InstallRadius and InstallRadius ignores it, as it ignored --contour-set and --contour-set-file.
func Test_Helm_InstallRadius_ContourOptionsReachHelm(t *testing.T) {
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

	defaults := PopulateDefaultClusterOptions(CLIClusterOptions{})
	baseline := recordInstallRadius(t, defaults)

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

			require.NotEqual(t, baseline, recordInstallRadius(t, options), "InstallRadius ignores the Contour option %s", field.Name)
		})
	}
}

// recordInstallRadius runs InstallRadius on a fresh cluster and records what it asks Helm to do.
func recordInstallRadius(t *testing.T, options ClusterOptions) []string {
	t.Helper()

	var calls []string
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
	client.EXPECT().RunHelmGet(gomock.Any(), gomock.Any()).Return(nil, driver.ErrReleaseNotFound).AnyTimes()
	client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ *helm.Configuration, _ *chart.Chart, vals map[string]any, releaseName string, namespace string, wait bool, timeout time.Duration) (*releasev1.Release, error) {
			calls = append(calls, fmt.Sprintf("install %s namespace=%s wait=%t timeout=%s values=%v", releaseName, namespace, wait, timeout, vals))
			return &releasev1.Release{}, nil
		}).AnyTimes()

	impl := &Impl{
		Helm:                           client,
		configureDefaultContourGateway: (&defaultContourGatewayCalls{}).configure,
		listGatewayAPICRDs:             staticGatewayAPICRDs(),
	}
	require.NoError(t, impl.InstallRadius(t.Context(), options, "test-context"))

	return calls
}

// newContourReleaseWithStoredValues returns a Contour release that an earlier rad install kubernetes installed on a
// k3s cluster with Traefik, with the Contour values that turn off the features that the Traefik CRDs lack.
func newContourReleaseWithStoredValues(options ClusterOptions) *releasev1.Release {
	return &releasev1.Release{
		Name:  options.Contour.ReleaseName,
		Chart: newTestContourChart(),
		Info:  &releasev1.Info{Status: releasecommon.StatusDeployed},
		Config: map[string]any{
			"gatewayAPI": map[string]any{"manageCRDs": false},
			"contour": map[string]any{
				"extraArgs": []any{"--disable-feature=tlsroutes", "--disable-feature=tcproutes", "--disable-feature=backendtlspolicies"},
			},
		},
	}
}

// staticGatewayAPICRDs returns a Gateway API CRD lister that returns crds without contacting a cluster.
func staticGatewayAPICRDs(crds ...gatewayAPICRD) func(context.Context, string) ([]gatewayAPICRD, error) {
	return func(context.Context, string) ([]gatewayAPICRD, error) {
		return crds, nil
	}
}

// newTestContourChart returns a chart with the metadata of the default Contour chart.
func newTestContourChart() *chart.Chart {
	return &chart.Chart{Metadata: &chart.Metadata{Name: "contour", Version: ContourChartDefaultVersion, AppVersion: "1.32.0"}}
}

// gatewayAPICRDManifest returns a release manifest that contains the named Gateway API CRDs.
func gatewayAPICRDManifest(names ...string) string {
	var manifest strings.Builder
	for _, name := range names {
		fmt.Fprintf(&manifest, "---\napiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: %s\nspec:\n  group: %s\n", name, gatewayAPIGroup)
	}
	return manifest.String()
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
