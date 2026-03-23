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

package shutdown

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	githubConfig := radcli.LoadConfig(t, `
workspaces:
  default: github-workspace
  items:
    github-workspace:
      connection:
        kind: github
        context: k3d-radius-github
      environment: /planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default
      scope: /planes/radius/local/resourceGroups/default
`)

	kubernetesConfig := radcli.LoadConfig(t, `
workspaces:
  default: k8s-workspace
  items:
    k8s-workspace:
      connection:
        kind: kubernetes
        context: kind-kind
      environment: /planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default
      scope: /planes/radius/local/resourceGroups/default
`)

	testcases := []radcli.ValidateInput{
		{
			Name:          "github workspace valid",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: githubConfig},
		},
		{
			Name:          "github workspace with cleanup flag",
			Input:         []string{"--cleanup"},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: githubConfig},
		},
		{
			Name:          "kubernetes workspace invalid",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: kubernetesConfig},
		},
		{
			Name:          "too many args invalid",
			Input:         []string{"extra-arg"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: githubConfig},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

// --- Run tests ---------------------------------------------------------------

// newRunnerForTest creates a Runner wired with fake implementations so
// no real git, kubectl, or k3d calls are made.
func newRunnerForTest(t *testing.T,
	openWorktree func(context.Context) (worktreeHandle, error),
	pgClient PGBackupClient,
	deleteCluster func(context.Context, string) error,
	workspace *workspaces.Workspace,
	cleanup bool,
) *Runner {
	t.Helper()
	return &Runner{
		ConfigHolder:   &framework.ConfigHolder{},
		Output:         &output.MockOutput{},
		Workspace:      workspace,
		Cleanup:        cleanup,
		PGBackupClient: pgClient,
		openWorktree:   openWorktree,
		deleteCluster:  deleteCluster,
	}
}

func githubWorkspace(kubeCtx string) *workspaces.Workspace {
	return &workspaces.Workspace{
		Name: "default",
		Connection: map[string]any{
			"kind":    workspaces.KindGitHub,
			"context": kubeCtx,
		},
	}
}

func noopWorktree(_ context.Context) (worktreeHandle, error) {
	return worktreeHandle{
		path:       "/tmp/fake-state-dir",
		clearLock:  func(_ context.Context) error { return nil },
		removeFunc: func(_ context.Context) {},
	}, nil
}

func Test_Run_Success_NoCleanup(t *testing.T) {
	ctrl := gomock.NewController(t)
	pg := NewMockPGBackupClient(ctrl)
	pg.EXPECT().Backup(gomock.Any(), "k3d-radius-github", "radius-system", "/tmp/fake-state-dir").Return(nil)

	runner := newRunnerForTest(t,
		noopWorktree,
		pg,
		func(_ context.Context, _ string) error { t.Fatal("deleteCluster should not be called"); return nil },
		githubWorkspace("k3d-radius-github"),
		false,
	)

	require.NoError(t, runner.Run(context.Background()))
}

func Test_Run_Success_WithCleanup_DefaultCluster(t *testing.T) {
	ctrl := gomock.NewController(t)
	pg := NewMockPGBackupClient(ctrl)
	pg.EXPECT().Backup(gomock.Any(), "k3d-radius-github", "radius-system", "/tmp/fake-state-dir").Return(nil)

	deletedName := ""
	runner := newRunnerForTest(t,
		noopWorktree,
		pg,
		func(_ context.Context, name string) error { deletedName = name; return nil },
		githubWorkspace("k3d-radius-github"),
		true,
	)

	require.NoError(t, runner.Run(context.Background()))
	require.Equal(t, "radius-github", deletedName)
}

func Test_Run_Success_WithCleanup_CustomCluster(t *testing.T) {
	ctrl := gomock.NewController(t)
	pg := NewMockPGBackupClient(ctrl)
	pg.EXPECT().Backup(gomock.Any(), "k3d-radius-github", "radius-system", "/tmp/fake-state-dir").Return(nil)

	ws := githubWorkspace("k3d-radius-github")
	ws.Connection["cluster"] = "my-custom-cluster"

	deletedName := ""
	runner := newRunnerForTest(t,
		noopWorktree,
		pg,
		func(_ context.Context, name string) error { deletedName = name; return nil },
		ws,
		true,
	)

	require.NoError(t, runner.Run(context.Background()))
	require.Equal(t, "my-custom-cluster", deletedName)
}

func Test_Run_NoKubernetesContext_Error(t *testing.T) {
	runner := newRunnerForTest(t,
		nil,
		nil,
		nil,
		// Workspace with no connection at all — KubernetesContext returns false.
		&workspaces.Workspace{Name: "broken", Connection: map[string]any{}},
		false,
	)

	err := runner.Run(context.Background())
	require.Error(t, err)
}

func Test_Run_OpenWorktree_Error(t *testing.T) {
	runner := newRunnerForTest(t,
		func(_ context.Context) (worktreeHandle, error) { return worktreeHandle{}, errors.New("git not available") },
		nil,
		nil,
		githubWorkspace("k3d-radius-github"),
		false,
	)

	err := runner.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "git not available")
}

func Test_Run_Backup_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	pg := NewMockPGBackupClient(ctrl)
	pg.EXPECT().Backup(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("kubectl unavailable"))

	runner := newRunnerForTest(t, noopWorktree, pg, nil, githubWorkspace("k3d-radius-github"), false)

	err := runner.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "kubectl unavailable")
}

func Test_Run_ClearLock_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	pg := NewMockPGBackupClient(ctrl)
	pg.EXPECT().Backup(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	runner := newRunnerForTest(t,
		func(_ context.Context) (worktreeHandle, error) {
			return worktreeHandle{
				path:       "/tmp/fake-state-dir",
				clearLock:  func(_ context.Context) error { return errors.New("git push failed") },
				removeFunc: func(_ context.Context) {},
			}, nil
		},
		pg, nil, githubWorkspace("k3d-radius-github"), false,
	)

	err := runner.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "git push failed")
}

func Test_Run_DeleteCluster_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	pg := NewMockPGBackupClient(ctrl)
	pg.EXPECT().Backup(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	runner := newRunnerForTest(t,
		noopWorktree,
		pg,
		func(_ context.Context, _ string) error { return errors.New("k3d not found") },
		githubWorkspace("k3d-radius-github"),
		true,
	)

	err := runner.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "k3d not found")
}

func Test_NewRunner_CreatesDefaultFields(t *testing.T) {
	factory := &framework.Impl{}
	runner := NewRunner(factory)
	require.NotNil(t, runner.PGBackupClient)
	require.NotNil(t, runner.openWorktree)
	require.NotNil(t, runner.deleteCluster)
}

func Test_NewPGBackupClient_ReturnsNonNil(t *testing.T) {
	client := NewPGBackupClient()
	require.NotNil(t, client)
}
