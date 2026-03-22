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

package radinit

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/cli/gitstate"
	"github.com/radius-project/radius/pkg/cli/workspaces"
)

// makeGitHubRunner returns a minimal Runner configured for testing runGitHubPostInstall.
// The Worktree.Path is set to a temp dir so pgbackup stateDir reads/writes don't fail on
// path validation — the PGBackupClient mock short-circuits actual kubectl calls anyway.
func makeGitHubRunner(t *testing.T, pg PGBackupClient, semaphore gitstate.SemaphoreState) *Runner {
	t.Helper()
	return &Runner{
		Workspace: &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    workspaces.KindGitHub,
				"context": "k3d-radius-github",
			},
		},
		Options: &initOptions{
			Cluster: clusterOptions{Namespace: "radius-system"},
		},
		Worktree:             &gitstate.StateWorktree{Path: t.TempDir()},
		GitHubSemaphoreState: semaphore,
		PGBackupClient:       pg,
	}
}

// --- Nil worktree ---

func Test_runGitHubPostInstall_NilWorktree(t *testing.T) {
	runner := &Runner{
		Workspace: &workspaces.Workspace{
			Connection: map[string]any{"kind": workspaces.KindGitHub, "context": "ctx"},
		},
		Options:        &initOptions{Cluster: clusterOptions{Namespace: "radius-system"}},
		Worktree:       nil,
		PGBackupClient: nil,
	}
	err := runner.runGitHubPostInstall(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "state worktree is not initialised")
}

// --- WaitForReady failure ---

func Test_runGitHubPostInstall_WaitForReady_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	pg := NewMockPGBackupClient(ctrl)
	pg.EXPECT().WaitForReady(gomock.Any(), "k3d-radius-github", "radius-system").
		Return(errors.New("postgres not ready"))

	err := makeGitHubRunner(t, pg, gitstate.SemaphoreFirstRun).runGitHubPostInstall(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "postgres not ready")
}

// --- SemaphoreInterrupted ---

func Test_runGitHubPostInstall_Interrupted(t *testing.T) {
	ctrl := gomock.NewController(t)
	pg := NewMockPGBackupClient(ctrl)
	pg.EXPECT().WaitForReady(gomock.Any(), "k3d-radius-github", "radius-system").Return(nil)
	// No backup or restore calls expected.

	err := makeGitHubRunner(t, pg, gitstate.SemaphoreInterrupted).runGitHubPostInstall(context.Background())
	require.NoError(t, err)
}

// --- SemaphoreFirstRun ---

func Test_runGitHubPostInstall_FirstRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	pg := NewMockPGBackupClient(ctrl)
	pg.EXPECT().WaitForReady(gomock.Any(), "k3d-radius-github", "radius-system").Return(nil)

	err := makeGitHubRunner(t, pg, gitstate.SemaphoreFirstRun).runGitHubPostInstall(context.Background())
	require.NoError(t, err)
}

// --- SemaphoreClean, no backup files ---

func Test_runGitHubPostInstall_Clean_NoBackup(t *testing.T) {
	ctrl := gomock.NewController(t)
	pg := NewMockPGBackupClient(ctrl)
	pg.EXPECT().WaitForReady(gomock.Any(), "k3d-radius-github", "radius-system").Return(nil)
	pg.EXPECT().HasBackup(gomock.Any()).Return(false)

	err := makeGitHubRunner(t, pg, gitstate.SemaphoreClean).runGitHubPostInstall(context.Background())
	require.NoError(t, err)
}

// --- SemaphoreClean, backup present, restore succeeds ---

func Test_runGitHubPostInstall_Clean_Restore_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	pg := NewMockPGBackupClient(ctrl)
	pg.EXPECT().WaitForReady(gomock.Any(), "k3d-radius-github", "radius-system").Return(nil)
	pg.EXPECT().HasBackup(gomock.Any()).Return(true)
	pg.EXPECT().Restore(gomock.Any(), "k3d-radius-github", "radius-system", gomock.Any()).Return(nil)

	err := makeGitHubRunner(t, pg, gitstate.SemaphoreClean).runGitHubPostInstall(context.Background())
	require.NoError(t, err)
}

// --- SemaphoreClean, backup present, restore fails ---

func Test_runGitHubPostInstall_Clean_Restore_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	pg := NewMockPGBackupClient(ctrl)
	pg.EXPECT().WaitForReady(gomock.Any(), "k3d-radius-github", "radius-system").Return(nil)
	pg.EXPECT().HasBackup(gomock.Any()).Return(true)
	pg.EXPECT().Restore(gomock.Any(), "k3d-radius-github", "radius-system", gomock.Any()).
		Return(errors.New("restore failed"))

	err := makeGitHubRunner(t, pg, gitstate.SemaphoreClean).runGitHubPostInstall(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "restore failed")
}

// --- PGBackupClient production wrapper sanity check ---

func Test_NewPGBackupClient_ReturnsNonNil(t *testing.T) {
	require.NotNil(t, NewPGBackupClient())
}
