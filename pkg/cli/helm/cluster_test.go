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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/version"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
)

func Test_DefaultsToHelmChartVersionValue(t *testing.T) {
	clusterOptions := PopulateDefaultClusterOptions(CLIClusterOptions{})

	// Not checking other values due to potential failures on release builds
	require.Equal(t, version.ChartVersion(), clusterOptions.Radius.ChartVersion)
}

func Test_Helm_InstallRadius(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"
	options := NewDefaultClusterOptions()

	// Mock Helm Pull
	mockHelmClient.EXPECT().
		RunHelmPull(gomock.Any(), fmt.Sprintf("%s/%s", options.Radius.ChartRepo, options.Radius.ReleaseName)).
		DoAndReturn(func(pullopts []helm.PullOpt, chartRef string) (string, error) {
			pull := helm.NewPullWithOpts(pullopts...)
			// Simulate downloading the chart to the temp dir
			err := os.WriteFile(filepath.Join(pull.DestDir, "Chart.yaml"), []byte("name: radius\nversion: 0.1.0"), 0644)
			require.NoError(t, err)
			return "Pulled", nil
		}).Times(1)
	mockHelmClient.EXPECT().
		RunHelmPull(gomock.Any(), options.Contour.ReleaseName).
		DoAndReturn(func(pullopts []helm.PullOpt, chartRef string) (string, error) {
			pull := helm.NewPullWithOpts(pullopts...)
			// Simulate downloading the chart to the temp dir
			err := os.WriteFile(filepath.Join(pull.DestDir, "Chart.yaml"), []byte("name: contour\nversion: 0.1.0"), 0644)
			require.NoError(t, err)
			return "Pulled", nil
		}).Times(1)

	radiusRelease := &release.Release{
		Name:  options.Radius.ReleaseName,
		Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}},
	}
	contourRelease := &release.Release{
		Name:  options.Contour.ReleaseName,
		Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}},
	}

	// Mock Helm List
	mockHelmClient.EXPECT().RunHelmList(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius", "radius-system").Return([]*release.Release{}, nil).Times(1)
	mockHelmClient.EXPECT().RunHelmList(gomock.AssignableToTypeOf(&helm.Configuration{}), "contour", "radius-system").Return([]*release.Release{}, nil).Times(1)

	// Mock Helm Install
	mockHelmClient.EXPECT().RunHelmInstall(gomock.AssignableToTypeOf(&helm.Configuration{}), gomock.AssignableToTypeOf(&chart.Chart{}), "radius", "radius-system").Return(radiusRelease, nil).Times(1)
	mockHelmClient.EXPECT().RunHelmInstall(gomock.AssignableToTypeOf(&helm.Configuration{}), gomock.AssignableToTypeOf(&chart.Chart{}), "contour", "radius-system").Return(contourRelease, nil).Times(1)

	// Mock Helm Chart Load
	mockHelmClient.EXPECT().LoadChart(gomock.Any()).Return(&chart.Chart{}, nil).Times(2)

	err := impl.InstallRadius(ctx, options, kubeContext)
	require.NoError(t, err)
}

func Test_Helm_UninstallRadius(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"
	options := NewDefaultClusterOptions()

	// Expect uninstall calls for Radius / Contour.
	for _, c := range []struct {
		releaseName string
		ns          string
	}{
		{options.Radius.ReleaseName, options.Radius.Namespace},
		{options.Contour.ReleaseName, options.Contour.Namespace},
	} {
		mockHelmClient.EXPECT().
			RunHelmUninstall(gomock.AssignableToTypeOf(&helm.Configuration{}), c.releaseName, c.ns).
			Return(&release.UninstallReleaseResponse{}, nil).
			Times(1)
	}

	err := impl.UninstallRadius(ctx, options, kubeContext)
	require.NoError(t, err)
}

func Test_Helm_UninstallRadius_ReleaseNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"
	options := NewDefaultClusterOptions()

	// Radius missing, other releases present.
	mockHelmClient.EXPECT().
		RunHelmUninstall(gomock.AssignableToTypeOf(&helm.Configuration{}), options.Radius.ReleaseName, options.Radius.Namespace).
		Return(&release.UninstallReleaseResponse{}, driver.ErrReleaseNotFound).
		Times(1)

	for _, c := range []struct {
		releaseName string
		ns          string
	}{
		{options.Contour.ReleaseName, options.Contour.Namespace},
	} {
		mockHelmClient.EXPECT().
			RunHelmUninstall(gomock.AssignableToTypeOf(&helm.Configuration{}), c.releaseName, c.ns).
			Return(&release.UninstallReleaseResponse{}, nil).
			Times(1)
	}

	err := impl.UninstallRadius(ctx, options, kubeContext)
	require.NoError(t, err) // ErrReleaseNotFound should be swallowed
}

func Test_Helm_CheckRadiusInstall(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	kubeContext := "test-context"
	options := NewDefaultClusterOptions()

	// Helper to create a dummy release with the given version.
	newRel := func(name, ver string) *release.Release {
		return &release.Release{
			Name:  name,
			Chart: &chart.Chart{Metadata: &chart.Metadata{Version: ver}},
		}
	}

	// Radius is installed, Contour not installed.
	mockHelmClient.EXPECT().
		RunHelmList(gomock.AssignableToTypeOf(&helm.Configuration{}), options.Radius.ReleaseName, options.Radius.Namespace).
		Return([]*release.Release{newRel(options.Radius.ReleaseName, "0.1.0")}, nil).Times(1)
	mockHelmClient.EXPECT().
		RunHelmList(gomock.AssignableToTypeOf(&helm.Configuration{}), options.Contour.ReleaseName, options.Contour.Namespace).
		Return([]*release.Release{}, nil).Times(1)

	state, err := impl.CheckRadiusInstall(kubeContext)
	require.NoError(t, err)
	require.True(t, state.RadiusInstalled)
	require.False(t, state.ContourInstalled)
	require.Equal(t, "0.1.0", state.RadiusVersion)
	require.Equal(t, "", state.ContourVersion)
}

func Test_Helm_CheckRadiusInstall_ErrorOnQuery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	kubeContext := "test-context"
	options := NewDefaultClusterOptions()

	// First call (Radius) returns an error – the method should propagate it.
	mockHelmClient.EXPECT().
		RunHelmList(gomock.AssignableToTypeOf(&helm.Configuration{}), options.Radius.ReleaseName, options.Radius.Namespace).
		Return(nil, fmt.Errorf("query failed")).
		Times(1)

	// No further expectations – function should exit early.
	_, err := impl.CheckRadiusInstall(kubeContext)
	require.Error(t, err)
	require.Contains(t, err.Error(), "query failed")
}

func Test_Helm_UpgradeRadius(t *testing.T) {
	t.Skip()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"
	options := NewDefaultClusterOptions()

	// Mock Helm Pull
	mockHelmClient.EXPECT().
		RunHelmPull(gomock.Any(), fmt.Sprintf("%s/%s", options.Radius.ChartRepo, options.Radius.ReleaseName)).
		DoAndReturn(func(pullopts []helm.PullOpt, chartRef string) (string, error) {
			pull := helm.NewPullWithOpts(pullopts...)
			// Simulate downloading the chart to the temp dir
			err := os.WriteFile(filepath.Join(pull.DestDir, "Chart.yaml"), []byte("name: radius\nversion: 0.1.0"), 0644)
			require.NoError(t, err)
			return "Pulled", nil
		}).Times(1)
	mockHelmClient.EXPECT().
		RunHelmPull(gomock.Any(), options.Contour.ReleaseName).
		DoAndReturn(func(pullopts []helm.PullOpt, chartRef string) (string, error) {
			pull := helm.NewPullWithOpts(pullopts...)
			// Simulate downloading the chart to the temp dir
			err := os.WriteFile(filepath.Join(pull.DestDir, "Chart.yaml"), []byte("name: contour\nversion: 0.1.0"), 0644)
			require.NoError(t, err)
			return "Pulled", nil
		}).Times(1)

	radiusRelease := &release.Release{
		Name:  options.Radius.ReleaseName,
		Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}},
	}
	contourRelease := &release.Release{
		Name:  options.Contour.ReleaseName,
		Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}},
	}

	// Mock Helm Upgrade
	mockHelmClient.EXPECT().RunHelmUpgrade(gomock.AssignableToTypeOf(&helm.Configuration{}), gomock.AssignableToTypeOf(&chart.Chart{}), "radius", "radius-system").Return(radiusRelease, nil).Times(1)
	mockHelmClient.EXPECT().RunHelmUpgrade(gomock.AssignableToTypeOf(&helm.Configuration{}), gomock.AssignableToTypeOf(&chart.Chart{}), "contour", "radius-system").Return(contourRelease, nil).Times(1)

	// Mock Helm Chart Load
	mockHelmClient.EXPECT().LoadChart(gomock.Any()).Return(&chart.Chart{}, nil).Times(2)

	err := impl.UpgradeRadius(ctx, options, kubeContext)
	require.NoError(t, err)
}

func Test_PopulateDefaultClusterOptions(t *testing.T) {
	custom := CLIClusterOptions{
		Radius: ChartOptions{
			Reinstall:    true,
			ChartPath:    "custom-path",
			SetArgs:      []string{"foo=bar"},
			SetFileArgs:  []string{"cert=./ca.crt"},
			ChartVersion: "1.2.3",
		},
	}

	opts := PopulateDefaultClusterOptions(custom)

	require.True(t, opts.Radius.Reinstall)
	require.Equal(t, "custom-path", opts.Radius.ChartPath)
	require.Equal(t, []string{"foo=bar"}, opts.Radius.SetArgs)
	require.Equal(t, []string{"cert=./ca.crt"}, opts.Radius.SetFileArgs)
	require.Equal(t, "1.2.3", opts.Radius.ChartVersion)
}
