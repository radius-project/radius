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
	"github.com/radius-project/radius/pkg/cli/pgbackup"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/statearchive"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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

func newTestRunner(t *testing.T, ctrl *gomock.Controller, client *fakeStateBackupClient) (*Runner, *statearchive.MockSession, string) {
	t.Helper()
	stateDir := t.TempDir()

	session := statearchive.NewMockSession(ctrl)
	session.EXPECT().Path().Return(stateDir).AnyTimes()
	session.EXPECT().Close(gomock.Any()).Times(1)

	archive := statearchive.NewMockArchive(ctrl)
	archive.EXPECT().Open(gomock.Any(), pgbackup.StateBranchName()).Return(session, nil).Times(1)

	r := &Runner{
		Output:      &output.MockOutput{},
		Workspace:   kubernetesWorkspace(),
		StateClient: client,
		Archive:     archive,
	}
	return r, session, stateDir
}

func Test_Run_BacksUpBothStoresAndCommits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := &fakeStateBackupClient{}
	r, session, stateDir := newTestRunner(t, ctrl, client)
	session.EXPECT().Commit(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	err := r.Run(context.Background())
	require.NoError(t, err)

	require.True(t, client.dbCalled, "databases should be backed up")
	require.True(t, client.tfCalled, "terraform state should be backed up")
	require.Equal(t, stateDir, client.stateDirSeen, "backup must target the archive path")
}

func Test_Run_DatabaseBackupFailureStopsBeforeCommit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := &fakeStateBackupClient{backupDBErr: errors.New("pg_dump boom")}
	// Commit is intentionally not expected: a backup failure must stop before committing. The
	// deferred session.Close is still expected (verified by the mock at ctrl.Finish).
	r, _, _ := newTestRunner(t, ctrl, client)

	err := r.Run(context.Background())
	require.ErrorContains(t, err, "pg_dump boom")
	require.False(t, client.tfCalled, "terraform backup should not run after database failure")
}

func Test_Run_CommitFailureIsReturned(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := &fakeStateBackupClient{}
	r, session, _ := newTestRunner(t, ctrl, client)
	session.EXPECT().Commit(gomock.Any(), gomock.Any()).Return(errors.New("push rejected")).Times(1)

	err := r.Run(context.Background())
	require.ErrorContains(t, err, "push rejected")
}
