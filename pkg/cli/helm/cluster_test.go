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
	"errors"
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

	// Not checking other values due to potential failures on release builds, the chart version
	// is primarily the mail regression we see.
	require.Equal(t, version.ChartVersion(), clusterOptions.Radius.ChartVersion)
}

func Test_Impl_InstallRadius(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"
	options := NewDefaultClusterOptions()

	// Mock Helm Pull for Radius, Dapr, Contour
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
		RunHelmPull(gomock.Any(), options.Dapr.ReleaseName).
		DoAndReturn(func(pullopts []helm.PullOpt, chartRef string) (string, error) {
			pull := helm.NewPullWithOpts(pullopts...)
			// Simulate downloading the chart to the temp dir
			err := os.WriteFile(filepath.Join(pull.DestDir, "Chart.yaml"), []byte("name: dapr\nversion: 0.1.0"), 0644)
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

	// // Mock Helm History (to simulate chart not found initially)
	// mockHelmClient.EXPECT().RunHelmHistory(gomock.Any(), options.Radius.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(1)
	// mockHelmClient.EXPECT().RunHelmHistory(gomock.Any(), options.Dapr.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(1)
	// mockHelmClient.EXPECT().RunHelmHistory(gomock.Any(), options.Contour.ReleaseName).Return(nil, driver.ErrReleaseNotFound).Times(1)

	// // Mock Helm Install - Return values should match the HelmClient interface
	// mockHelmClient.EXPECT().RunHelmInstall(gomock.Any(), gomock.AssignableToTypeOf(&chart.Chart{})).Return(&release.Release{Name: options.Radius.ReleaseName}, nil).Times(1)
	// mockHelmClient.EXPECT().RunHelmInstall(gomock.Any(), gomock.AssignableToTypeOf(&chart.Chart{})).Return(&release.Release{Name: options.Dapr.ReleaseName}, nil).Times(1)
	// mockHelmClient.EXPECT().RunHelmInstall(gomock.Any(), gomock.AssignableToTypeOf(&chart.Chart{})).Return(&release.Release{Name: options.Contour.ReleaseName}, nil).Times(1)

	// Mock Helm Chart Load
	mockHelmClient.EXPECT().
		LoadChart(gomock.Any()).
		DoAndReturn(func(chartPath string) (*chart.Chart, error) {
			// Simulate the chart being loaded successfully
			return &chart.Chart{}, nil
		}).Times(3)

	err := impl.InstallRadius(ctx, options, kubeContext)
	require.NoError(t, err)
}

func Test_Impl_InstallRadius_AlreadyInstalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"
	options := NewDefaultClusterOptions()

	// Mock Helm Pull for Radius, Dapr, Contour
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
		RunHelmPull(gomock.Any(), options.Dapr.ReleaseName).
		DoAndReturn(func(pullopts []helm.PullOpt, chartRef string) (string, error) {
			pull := helm.NewPullWithOpts(pullopts...)
			// Simulate downloading the chart to the temp dir
			err := os.WriteFile(filepath.Join(pull.DestDir, "Chart.yaml"), []byte("name: dapr\nversion: 0.1.0"), 0644)
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

	// // Mock Helm History (to simulate charts already installed)
	// mockHelmClient.EXPECT().RunHelmHistory(gomock.Any(), options.Radius.ReleaseName).Return([]*release.Release{{Name: options.Radius.ReleaseName}}, nil).Times(1)
	// mockHelmClient.EXPECT().RunHelmHistory(gomock.Any(), options.Dapr.ReleaseName).Return([]*release.Release{{Name: options.Dapr.ReleaseName}}, nil).Times(1)
	// mockHelmClient.EXPECT().RunHelmHistory(gomock.Any(), options.Contour.ReleaseName).Return([]*release.Release{{Name: options.Contour.ReleaseName}}, nil).Times(1)

	// Mock Helm Chart Load (needed even if not upgrading, to check versions perhaps)
	mockHelmClient.EXPECT().
		LoadChart(gomock.Any()).
		DoAndReturn(func(chartPath string) (*chart.Chart, error) {
			// Simulate the chart being loaded successfully
			// The version here should match the version being checked against, if applicable
			return &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}}, nil
		}).Times(3) // Assuming LoadChart is called for each component

	// No Install/Upgrade calls expected

	err := impl.InstallRadius(ctx, options, kubeContext)
	require.NoError(t, err)
}

func Test_Impl_UninstallRadius(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"
	options := NewDefaultClusterOptions()

	// Mock Helm Uninstall for Radius, Dapr, Contour
	mockHelmClient.EXPECT().RunHelmUninstall(gomock.Any(), options.Radius.ReleaseName, options.Radius.Namespace).Return(&release.UninstallReleaseResponse{}, nil).Times(1)
	mockHelmClient.EXPECT().RunHelmUninstall(gomock.Any(), options.Dapr.ReleaseName, options.Dapr.Namespace).Return(&release.UninstallReleaseResponse{}, nil).Times(1)
	mockHelmClient.EXPECT().RunHelmUninstall(gomock.Any(), options.Contour.ReleaseName, options.Contour.Namespace).Return(&release.UninstallReleaseResponse{}, nil).Times(1)

	err := impl.UninstallRadius(ctx, options, kubeContext)
	require.NoError(t, err)
}

func Test_Impl_UninstallRadius_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"
	options := NewDefaultClusterOptions()

	// Mock Helm Uninstall returning ErrReleaseNotFound
	mockHelmClient.EXPECT().RunHelmUninstall(gomock.Any(), options.Radius.ReleaseName, options.Radius.Namespace).Return(nil, driver.ErrReleaseNotFound).Times(1)
	mockHelmClient.EXPECT().RunHelmUninstall(gomock.Any(), options.Dapr.ReleaseName, options.Dapr.Namespace).Return(nil, driver.ErrReleaseNotFound).Times(1)
	mockHelmClient.EXPECT().RunHelmUninstall(gomock.Any(), options.Contour.ReleaseName, options.Contour.Namespace).Return(nil, driver.ErrReleaseNotFound).Times(1)

	err := impl.UninstallRadius(ctx, options, kubeContext)
	require.NoError(t, err, "ErrReleaseNotFound should be handled gracefully")
}

func Test_Impl_UninstallRadius_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"
	options := NewDefaultClusterOptions()
	testError := errors.New("helm uninstall failed")

	// Mock Helm Uninstall returning an error for Radius
	mockHelmClient.EXPECT().RunHelmUninstall(gomock.Any(), options.Radius.ReleaseName, options.Radius.Namespace).Return(nil, testError).Times(1)
	// No further calls expected after the first error for Dapr or Contour

	err := impl.UninstallRadius(ctx, options, kubeContext)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to uninstall radius")
	require.ErrorIs(t, err, testError)
}

func Test_Impl_CheckRadiusInstall(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	// ctx := context.Background() // Context not used by CheckRadiusInstall
	kubeContext := "test-context"
	radiusVersion := "1.2.3"
	daprVersion := "4.5.6"

	// Mock Helm List for Radius (found)
	mockHelmClient.EXPECT().
		RunHelmList(gomock.Any(), radiusReleaseName, RadiusSystemNamespace).
		Return([]*release.Release{{Name: radiusReleaseName, Version: 1, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: radiusVersion}}}}, nil).
		Times(1)

	// Mock Helm List for Dapr (found)
	mockHelmClient.EXPECT().
		RunHelmList(gomock.Any(), daprReleaseName, DaprSystemNamespace).
		Return([]*release.Release{{Name: daprReleaseName, Version: 1, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: daprVersion}}}}, nil).
		Times(1)

	state, err := impl.CheckRadiusInstall(kubeContext)
	require.NoError(t, err)
	require.True(t, state.RadiusInstalled)
	require.Equal(t, radiusVersion, state.RadiusVersion)
	require.True(t, state.DaprInstalled)
	require.Equal(t, daprVersion, state.DaprVersion)
}

func Test_Impl_CheckRadiusInstall_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	// ctx := context.Background() // Context not used by CheckRadiusInstall
	kubeContext := "test-context"

	// Mock Helm List for Radius (not found)
	mockHelmClient.EXPECT().
		RunHelmList(gomock.Any(), radiusReleaseName, RadiusSystemNamespace).
		Return(nil, driver.ErrReleaseNotFound).
		Times(1)

	// Mock Helm List for Dapr (not found)
	mockHelmClient.EXPECT().
		RunHelmList(gomock.Any(), daprReleaseName, DaprSystemNamespace).
		Return(nil, driver.ErrReleaseNotFound).
		Times(1)

	state, err := impl.CheckRadiusInstall(kubeContext)
	require.NoError(t, err)
	require.False(t, state.RadiusInstalled)
	require.Empty(t, state.RadiusVersion)
	require.False(t, state.DaprInstalled)
	require.Empty(t, state.DaprVersion)
}

func Test_Impl_CheckRadiusInstall_ListError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	// ctx := context.Background() // Context not used by CheckRadiusInstall
	kubeContext := "test-context"
	testError := errors.New("helm list failed")

	// Mock Helm List for Radius returning an error
	mockHelmClient.EXPECT().
		RunHelmList(gomock.Any(), radiusReleaseName, RadiusSystemNamespace).
		Return(nil, testError).
		Times(1)

	// No further calls expected for Dapr

	state, err := impl.CheckRadiusInstall(kubeContext)
	require.Error(t, err)
	require.ErrorIs(t, err, testError)
	require.False(t, state.RadiusInstalled) // State should reflect failure
	require.Empty(t, state.RadiusVersion)
	require.False(t, state.DaprInstalled)
	require.Empty(t, state.DaprVersion)
}
