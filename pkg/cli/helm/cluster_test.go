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
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	helmtime "helm.sh/helm/v3/pkg/time"
)

func Test_Helm_InstallRadius(t *testing.T) {
	t.Parallel()

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

	// Mock Helm Get
	mockHelmClient.EXPECT().RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius").Return(nil, driver.ErrReleaseNotFound).Times(1)
	mockHelmClient.EXPECT().RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "contour").Return(nil, driver.ErrReleaseNotFound).Times(1)

	// Mock Helm Install
	mockHelmClient.EXPECT().RunHelmInstall(gomock.AssignableToTypeOf(&helm.Configuration{}), gomock.AssignableToTypeOf(&chart.Chart{}), "radius", "radius-system", true).Return(radiusRelease, nil).Times(1)
	mockHelmClient.EXPECT().RunHelmInstall(gomock.AssignableToTypeOf(&helm.Configuration{}), gomock.AssignableToTypeOf(&chart.Chart{}), "contour", "radius-system", false).Return(contourRelease, nil).Times(1)

	// Mock Helm Chart Load
	mockHelmClient.EXPECT().LoadChart(gomock.Any()).Return(&chart.Chart{}, nil).Times(2)

	err := impl.InstallRadius(ctx, options, kubeContext)
	require.NoError(t, err)
}

func Test_Helm_UninstallRadius(t *testing.T) {
	t.Parallel()

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
			RunHelmUninstall(gomock.AssignableToTypeOf(&helm.Configuration{}), c.releaseName, c.ns, true).
			Return(&release.UninstallReleaseResponse{}, nil).
			Times(1)
	}

	err := impl.UninstallRadius(ctx, options, kubeContext)
	require.NoError(t, err)
}

func Test_Helm_UninstallRadius_ReleaseNotFound(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"
	options := NewDefaultClusterOptions()

	// Radius missing, other releases present.
	mockHelmClient.EXPECT().
		RunHelmUninstall(gomock.AssignableToTypeOf(&helm.Configuration{}), options.Radius.ReleaseName, options.Radius.Namespace, true).
		Return(&release.UninstallReleaseResponse{}, driver.ErrReleaseNotFound).
		Times(1)

	for _, c := range []struct {
		releaseName string
		ns          string
	}{
		{options.Contour.ReleaseName, options.Contour.Namespace},
	} {
		mockHelmClient.EXPECT().
			RunHelmUninstall(gomock.AssignableToTypeOf(&helm.Configuration{}), c.releaseName, c.ns, true).
			Return(&release.UninstallReleaseResponse{}, nil).
			Times(1)
	}

	err := impl.UninstallRadius(ctx, options, kubeContext)
	require.NoError(t, err) // ErrReleaseNotFound should be swallowed
}

func Test_Helm_CheckRadiusInstall(t *testing.T) {
	t.Parallel()

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
			Chart: &chart.Chart{Metadata: &chart.Metadata{Version: ver, AppVersion: ver}},
		}
	}

	// Radius is installed, Contour not installed.
	radiusRelease := newRel(options.Radius.ReleaseName, "0.1.0")
	// Set the release status to deployed for the history check
	radiusRelease.Info = &release.Info{Status: release.StatusDeployed}

	mockHelmClient.EXPECT().
		RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), options.Radius.ReleaseName).
		Return(radiusRelease, nil).Times(1)
	// Mock the history call that happens when Radius is installed
	mockHelmClient.EXPECT().
		RunHelmHistory(gomock.AssignableToTypeOf(&helm.Configuration{}), options.Radius.ReleaseName).
		Return([]*release.Release{radiusRelease}, nil).Times(1)
	mockHelmClient.EXPECT().
		RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), options.Contour.ReleaseName).
		Return(nil, driver.ErrReleaseNotFound).Times(1)

	state, err := impl.CheckRadiusInstall(kubeContext)
	require.NoError(t, err)
	require.True(t, state.RadiusInstalled)
	require.False(t, state.ContourInstalled)
	require.Equal(t, "0.1.0", state.RadiusVersion)
	require.Equal(t, "", state.ContourVersion)
}

func Test_Helm_CheckRadiusInstall_ErrorOnQuery(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	kubeContext := "test-context"
	options := NewDefaultClusterOptions()

	// First call (Radius) returns an error – the method should propagate it.
	mockHelmClient.EXPECT().
		RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), options.Radius.ReleaseName).
		Return(nil, fmt.Errorf("query failed")).
		Times(1)

	// No further expectations – function should exit early.
	_, err := impl.CheckRadiusInstall(kubeContext)
	require.Error(t, err)
	require.Contains(t, err.Error(), "query failed")
}

func Test_Helm_UpgradeRadius(t *testing.T) {
	t.Parallel()

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
	mockHelmClient.EXPECT().RunHelmUpgrade(gomock.AssignableToTypeOf(&helm.Configuration{}), gomock.AssignableToTypeOf(&chart.Chart{}), "radius", "radius-system", true).Return(radiusRelease, nil).Times(1)
	mockHelmClient.EXPECT().RunHelmUpgrade(gomock.AssignableToTypeOf(&helm.Configuration{}), gomock.AssignableToTypeOf(&chart.Chart{}), "contour", "radius-system", false).Return(contourRelease, nil).Times(1)

	// Mock Helm Chart Load
	mockHelmClient.EXPECT().LoadChart(gomock.Any()).Return(&chart.Chart{}, nil).Times(2)

	err := impl.UpgradeRadius(ctx, options, kubeContext)
	require.NoError(t, err)
}

func Test_PopulateDefaultClusterOptions(t *testing.T) {
	t.Parallel()

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

func Test_Helm_RollbackRadius_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"

	// Mock history with multiple versions
	history := []*release.Release{
		{Version: 1, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}}, Info: &release.Info{Status: "superseded"}},
		{Version: 2, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}}, Info: &release.Info{Status: "deployed"}},
	}

	mockHelmClient.EXPECT().
		RunHelmHistory(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius").
		Return(history, nil).
		Times(1)

	mockHelmClient.EXPECT().
		RunHelmRollback(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius", 1, true).
		Return(nil).
		Times(1)

	err := impl.RollbackRadius(ctx, kubeContext)
	require.NoError(t, err)
}

func Test_Helm_RollbackRadius_NoOlderVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"

	// Mock history with only one version
	history := []*release.Release{
		{Version: 1, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}}, Info: &release.Info{Status: "deployed"}},
	}

	mockHelmClient.EXPECT().
		RunHelmHistory(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius").
		Return(history, nil).
		Times(1)

	err := impl.RollbackRadius(ctx, kubeContext)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no previous revision available for rollback")
}

func Test_Helm_RollbackRadius_SameVersionSkipped(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"

	// Mock history with same versions (no semantic rollback available)
	history := []*release.Release{
		{Version: 1, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}}, Info: &release.Info{Status: "superseded"}},
		{Version: 2, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}}, Info: &release.Info{Status: "deployed"}},
	}

	mockHelmClient.EXPECT().
		RunHelmHistory(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius").
		Return(history, nil).
		Times(1)

	err := impl.RollbackRadius(ctx, kubeContext)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no older version found for rollback. Current version 0.46.0 is the oldest available")
}

func Test_Helm_RollbackRadius_HistoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"

	mockHelmClient.EXPECT().
		RunHelmHistory(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius").
		Return(nil, fmt.Errorf("history error")).
		Times(1)

	err := impl.RollbackRadius(ctx, kubeContext)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get release history")
}

func Test_Helm_RollbackRadiusToRevision_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"
	targetRevision := 3

	// Mock history containing the target revision
	history := []*release.Release{
		{Version: 1, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}}, Info: &release.Info{Status: "superseded"}},
		{Version: 2, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}}, Info: &release.Info{Status: "superseded"}},
		{Version: 3, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}}, Info: &release.Info{Status: "superseded"}},
		{Version: 4, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}}, Info: &release.Info{Status: "deployed"}},
	}

	mockHelmClient.EXPECT().
		RunHelmHistory(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius").
		Return(history, nil).
		Times(1)

	mockHelmClient.EXPECT().
		RunHelmRollback(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius", targetRevision, true).
		Return(nil).
		Times(1)

	err := impl.RollbackRadiusToRevision(ctx, kubeContext, targetRevision)
	require.NoError(t, err)
}

func Test_Helm_RollbackRadiusToRevision_RevisionNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"
	targetRevision := 999

	// Mock history without the target revision
	history := []*release.Release{
		{Version: 1, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}}, Info: &release.Info{Status: "superseded"}},
		{Version: 2, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}}, Info: &release.Info{Status: "deployed"}},
	}

	mockHelmClient.EXPECT().
		RunHelmHistory(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius").
		Return(history, nil).
		Times(1)

	err := impl.RollbackRadiusToRevision(ctx, kubeContext, targetRevision)
	require.Error(t, err)
	require.Contains(t, err.Error(), "revision 999 not found in release history")
}

func Test_Helm_RollbackRadiusToRevision_RollbackError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"
	targetRevision := 1

	// Mock history containing the target revision
	history := []*release.Release{
		{Version: 1, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}}, Info: &release.Info{Status: "superseded"}},
		{Version: 2, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}}, Info: &release.Info{Status: "deployed"}},
	}

	mockHelmClient.EXPECT().
		RunHelmHistory(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius").
		Return(history, nil).
		Times(1)

	mockHelmClient.EXPECT().
		RunHelmRollback(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius", targetRevision, true).
		Return(fmt.Errorf("rollback failed")).
		Times(1)

	err := impl.RollbackRadiusToRevision(ctx, kubeContext, targetRevision)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to rollback Radius to revision 1")
}

func Test_Helm_GetRadiusRevisions_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"

	// Mock history with multiple versions
	history := []*release.Release{
		{
			Version: 1,
			Chart:   &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}},
			Info: &release.Info{
				Status:        release.StatusSuperseded,
				FirstDeployed: helmtime.Time{Time: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)},
				LastDeployed:  helmtime.Time{Time: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)},
				Description:   "Install complete",
			},
		},
		{
			Version: 2,
			Chart:   &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}},
			Info: &release.Info{
				Status:        release.StatusDeployed,
				FirstDeployed: helmtime.Time{Time: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)},
				LastDeployed:  helmtime.Time{Time: time.Date(2023, 1, 2, 14, 30, 0, 0, time.UTC)},
				Description:   "Upgrade complete",
			},
		},
	}

	mockHelmClient.EXPECT().
		RunHelmHistory(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius").
		Return(history, nil).
		Times(1)

	revisions, err := impl.GetRadiusRevisions(ctx, kubeContext)
	require.NoError(t, err)
	require.Len(t, revisions, 2)

	// Check that revisions are in reverse order (latest first)
	require.Equal(t, 2, revisions[0].Revision)
	require.Equal(t, "0.46.0", revisions[0].ChartVersion)
	require.Equal(t, "deployed", revisions[0].Status)
	require.Equal(t, "Upgrade complete", revisions[0].Description)
	require.Equal(t, "2023-01-02 14:30:00", revisions[0].UpdatedAt)

	require.Equal(t, 1, revisions[1].Revision)
	require.Equal(t, "0.45.0", revisions[1].ChartVersion)
	require.Equal(t, "superseded", revisions[1].Status)
	require.Equal(t, "Install complete", revisions[1].Description)
	require.Equal(t, "2023-01-01 12:00:00", revisions[1].UpdatedAt)
}

func Test_Helm_GetRadiusRevisions_NoRevisions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"

	// Mock empty history
	history := []*release.Release{}

	mockHelmClient.EXPECT().
		RunHelmHistory(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius").
		Return(history, nil).
		Times(1)

	_, err := impl.GetRadiusRevisions(ctx, kubeContext)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no revisions found for Radius installation")
}

func Test_Helm_GetRadiusRevisions_HistoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"

	mockHelmClient.EXPECT().
		RunHelmHistory(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius").
		Return(nil, fmt.Errorf("history error")).
		Times(1)

	_, err := impl.GetRadiusRevisions(ctx, kubeContext)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get release history")
}

func Test_Helm_GetRadiusRevisions_MultipleUpgradesWithDifferentTimestamps(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	ctx := context.Background()
	kubeContext := "test-context"

	// Mock history simulating multiple upgrades and rollbacks across different days
	// This test validates that each revision shows its own LastDeployed timestamp,
	// not the FirstDeployed timestamp which would be the same for all revisions
	history := []*release.Release{
		{
			Version: 1,
			Chart:   &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}},
			Info: &release.Info{
				Status:        release.StatusSuperseded,
				FirstDeployed: helmtime.Time{Time: time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)},
				LastDeployed:  helmtime.Time{Time: time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)},
				Description:   "Install complete",
			},
		},
		{
			Version: 2,
			Chart:   &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}},
			Info: &release.Info{
				Status:        release.StatusSuperseded,
				FirstDeployed: helmtime.Time{Time: time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)},
				LastDeployed:  helmtime.Time{Time: time.Date(2023, 1, 2, 15, 30, 0, 0, time.UTC)},
				Description:   "Upgrade complete",
			},
		},
		{
			Version: 3,
			Chart:   &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}},
			Info: &release.Info{
				Status:        release.StatusSuperseded,
				FirstDeployed: helmtime.Time{Time: time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)},
				LastDeployed:  helmtime.Time{Time: time.Date(2023, 1, 3, 9, 15, 0, 0, time.UTC)},
				Description:   "Rollback to 1",
			},
		},
		{
			Version: 4,
			Chart:   &chart.Chart{Metadata: &chart.Metadata{Version: "0.47.0"}},
			Info: &release.Info{
				Status:        release.StatusDeployed,
				FirstDeployed: helmtime.Time{Time: time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)},
				LastDeployed:  helmtime.Time{Time: time.Date(2023, 1, 4, 11, 45, 0, 0, time.UTC)},
				Description:   "Upgrade complete",
			},
		},
	}

	mockHelmClient.EXPECT().
		RunHelmHistory(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius").
		Return(history, nil).
		Times(1)

	revisions, err := impl.GetRadiusRevisions(ctx, kubeContext)
	require.NoError(t, err)
	require.Len(t, revisions, 4)

	// Verify that each revision has its own unique LastDeployed timestamp
	// (not the same FirstDeployed timestamp for all)
	require.Equal(t, 4, revisions[0].Revision)
	require.Equal(t, "0.47.0", revisions[0].ChartVersion)
	require.Equal(t, "deployed", revisions[0].Status)
	require.Equal(t, "2023-01-04 11:45:00", revisions[0].UpdatedAt)

	require.Equal(t, 3, revisions[1].Revision)
	require.Equal(t, "0.45.0", revisions[1].ChartVersion)
	require.Equal(t, "superseded", revisions[1].Status)
	require.Equal(t, "2023-01-03 09:15:00", revisions[1].UpdatedAt)

	require.Equal(t, 2, revisions[2].Revision)
	require.Equal(t, "0.46.0", revisions[2].ChartVersion)
	require.Equal(t, "superseded", revisions[2].Status)
	require.Equal(t, "2023-01-02 15:30:00", revisions[2].UpdatedAt)

	require.Equal(t, 1, revisions[3].Revision)
	require.Equal(t, "0.45.0", revisions[3].ChartVersion)
	require.Equal(t, "superseded", revisions[3].Status)
	require.Equal(t, "2023-01-01 10:00:00", revisions[3].UpdatedAt)
}

func Test_Helm_CheckRadiusInstall_UsesAppVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	impl := &Impl{Helm: mockHelmClient}
	kubeContext := "test-context"
	options := NewDefaultClusterOptions()

	// Helper to create a dummy release with both chart version and app version.
	newRelWithAppVersion := func(name, chartVer, appVer string) *release.Release {
		return &release.Release{
			Name:  name,
			Chart: &chart.Chart{Metadata: &chart.Metadata{Version: chartVer, AppVersion: appVer}},
		}
	}

	// Create release with both chart version and app version
	radiusRelease := newRelWithAppVersion(options.Radius.ReleaseName, "1.0.0", "v0.43.0")
	// Set the release status to deployed for the history check
	radiusRelease.Info = &release.Info{Status: release.StatusDeployed}

	// Radius is installed with AppVersion, Contour not installed.
	mockHelmClient.EXPECT().
		RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), options.Radius.ReleaseName).
		Return(radiusRelease, nil).Times(1)
	// Mock the history call that happens when Radius is installed
	mockHelmClient.EXPECT().
		RunHelmHistory(gomock.AssignableToTypeOf(&helm.Configuration{}), options.Radius.ReleaseName).
		Return([]*release.Release{radiusRelease}, nil).Times(1)
	mockHelmClient.EXPECT().
		RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), options.Contour.ReleaseName).
		Return(nil, driver.ErrReleaseNotFound).Times(1)

	state, err := impl.CheckRadiusInstall(kubeContext)
	require.NoError(t, err)
	require.True(t, state.RadiusInstalled)
	require.False(t, state.ContourInstalled)
	// Should return AppVersion (v0.43.0) instead of chart Version (1.0.0)
	require.Equal(t, "v0.43.0", state.RadiusVersion)
	require.Equal(t, "", state.ContourVersion)
}
