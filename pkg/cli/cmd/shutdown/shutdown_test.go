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
	nonKubernetesConfig := radcli.LoadConfig(t, `
workspaces:
  default: github-workspace
  items:
    github-workspace:
      connection:
        kind: notkubernetes
      environment: /planes/radius/local/resourceGroups/test/providers/Applications.Core/environments/test
      scope: /planes/radius/local/resourceGroups/test
`)

	testcases := []radcli.ValidateInput{
		{
			Name:          "shutdown with a kubernetes workspace is valid",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{ConfigFilePath: "/weird/path", Config: radcli.LoadConfigWithWorkspace(t)},
		},
		{
			Name:          "shutdown with a non-kubernetes workspace is invalid",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{ConfigFilePath: "/weird/path", Config: nonKubernetesConfig},
		},
		{
			Name:          "shutdown does not accept positional args",
			Input:         []string{"unexpected"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{ConfigFilePath: "/weird/path", Config: radcli.LoadConfigWithWorkspace(t)},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

// fakeStateBackupClient records calls and returns canned errors.
type fakeStateBackupClient struct {
	backupDBErr  error
	backupTFErr  error
	dbCalled     bool
	tfCalled     bool
	stateDirSeen string
}

func (f *fakeStateBackupClient) BackupDatabases(ctx context.Context, kubeContext, namespace, stateDir string) error {
	f.dbCalled = true
	f.stateDirSeen = stateDir
	return f.backupDBErr
}

func (f *fakeStateBackupClient) BackupTerraform(ctx context.Context, kubeContext, namespace, stateDir string) error {
	f.tfCalled = true
	return f.backupTFErr
}

func kubernetesWorkspace() *workspaces.Workspace {
	return &workspaces.Workspace{
		Name: "test",
		Connection: map[string]any{
			"kind":    workspaces.KindKubernetes,
			"context": "k3d-test",
		},
	}
}

func newTestRunner(t *testing.T, ws *workspaces.Workspace, client *fakeStateBackupClient) (*Runner, *fakeWorktree) {
	t.Helper()
	wt := &fakeWorktree{path: t.TempDir()}
	r := &Runner{
		Output:      &output.MockOutput{},
		Workspace:   ws,
		StateClient: client,
		openWorktree: func(ctx context.Context) (worktreeHandle, error) {
			return worktreeHandle{
				path:          wt.path,
				commitAndPush: wt.commitAndPush,
				remove:        wt.remove,
			}, nil
		},
	}
	return r, wt
}

// fakeWorktree records commit/remove invocations.
type fakeWorktree struct {
	path          string
	committed     bool
	removed       bool
	commitMessage string
	commitErr     error
}

func (w *fakeWorktree) commitAndPush(ctx context.Context, message string) error {
	w.committed = true
	w.commitMessage = message
	return w.commitErr
}

func (w *fakeWorktree) remove(ctx context.Context) { w.removed = true }

func Test_Run_BacksUpBothStoresAndCommits(t *testing.T) {
	client := &fakeStateBackupClient{}
	r, wt := newTestRunner(t, kubernetesWorkspace(), client)

	err := r.Run(context.Background())
	require.NoError(t, err)

	require.True(t, client.dbCalled, "databases should be backed up")
	require.True(t, client.tfCalled, "terraform state should be backed up")
	require.Equal(t, wt.path, client.stateDirSeen, "backup must target the worktree path")
	require.True(t, wt.committed, "state should be committed and pushed")
	require.True(t, wt.removed, "worktree should be removed")
}

func Test_Run_DatabaseBackupFailureStopsAndStillRemovesWorktree(t *testing.T) {
	client := &fakeStateBackupClient{backupDBErr: errors.New("pg_dump boom")}
	r, wt := newTestRunner(t, kubernetesWorkspace(), client)

	err := r.Run(context.Background())
	require.ErrorContains(t, err, "pg_dump boom")
	require.False(t, client.tfCalled, "terraform backup should not run after database failure")
	require.False(t, wt.committed, "nothing should be committed on failure")
	require.True(t, wt.removed, "worktree must still be removed via defer")
}

func Test_Run_CommitFailureIsReturned(t *testing.T) {
	client := &fakeStateBackupClient{}
	r, wt := newTestRunner(t, kubernetesWorkspace(), client)
	wt.commitErr = errors.New("push rejected")

	err := r.Run(context.Background())
	require.ErrorContains(t, err, "push rejected")
	require.True(t, wt.removed)
}
