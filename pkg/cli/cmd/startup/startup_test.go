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

package startup

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
			Name:          "startup with a kubernetes workspace is valid",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{ConfigFilePath: "/weird/path", Config: radcli.LoadConfigWithWorkspace(t)},
		},
		{
			Name:          "startup with a non-kubernetes workspace is invalid",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{ConfigFilePath: "/weird/path", Config: nonKubernetesConfig},
		},
		{
			Name:          "startup does not accept positional args",
			Input:         []string{"unexpected"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{ConfigFilePath: "/weird/path", Config: radcli.LoadConfigWithWorkspace(t)},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

// fakeStateRestoreClient records calls and returns canned errors.
type fakeStateRestoreClient struct {
	waitErr      error
	restoreDBErr error
	restoreTFErr error

	waited   bool
	dbCalled bool
	tfCalled bool

	order []string
}

func (f *fakeStateRestoreClient) WaitForDatabaseReady(ctx context.Context, kubeContext, namespace string) error {
	f.waited = true
	f.order = append(f.order, "wait")
	return f.waitErr
}

func (f *fakeStateRestoreClient) RestoreDatabases(ctx context.Context, kubeContext, namespace, stateDir string) error {
	f.dbCalled = true
	f.order = append(f.order, "db")
	return f.restoreDBErr
}

func (f *fakeStateRestoreClient) RestoreTerraform(ctx context.Context, kubeContext, namespace, stateDir string) error {
	f.tfCalled = true
	f.order = append(f.order, "tf")
	return f.restoreTFErr
}

// fakeScaler records scale operations and appends them to a shared order slice so tests can assert
// that the control plane is scaled down before any restore and back up afterward.
type fakeScaler struct {
	scaleDownErr error
	scaleUpErr   error
	downCalled   bool
	upCalled     bool
	order        *[]string
}

func (s *fakeScaler) ScaleDown(ctx context.Context) (map[string]int32, error) {
	s.downCalled = true
	if s.order != nil {
		*s.order = append(*s.order, "scaledown")
	}
	if s.scaleDownErr != nil {
		return nil, s.scaleDownErr
	}
	return map[string]int32{"ucp": 1}, nil
}

func (s *fakeScaler) ScaleUp(ctx context.Context, saved map[string]int32) error {
	s.upCalled = true
	if s.order != nil {
		*s.order = append(*s.order, "scaleup")
	}
	return s.scaleUpErr
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

func newTestRunner(t *testing.T, ctrl *gomock.Controller, client *fakeStateRestoreClient) (*Runner, *fakeScaler) {
	t.Helper()

	session := statearchive.NewMockSession(ctrl)
	session.EXPECT().Path().Return(t.TempDir()).AnyTimes()
	session.EXPECT().Close(gomock.Any()).Times(1)

	archive := statearchive.NewMockArchive(ctrl)
	archive.EXPECT().Open(gomock.Any(), pgbackup.StateBranchName()).Return(session, nil).Times(1)

	scaler := &fakeScaler{order: &client.order}
	r := &Runner{
		Output:      &output.MockOutput{},
		Workspace:   kubernetesWorkspace(),
		StateClient: client,
		Archive:     archive,
		newScaler: func(kubeContext, namespace string) (ControlPlaneScaler, error) {
			return scaler, nil
		},
	}
	return r, scaler
}

func Test_Run_RestoresInOrderWaitDatabaseTerraform(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := &fakeStateRestoreClient{}
	r, scaler := newTestRunner(t, ctrl, client)

	err := r.Run(context.Background())
	require.NoError(t, err)

	require.True(t, client.waited)
	require.True(t, client.dbCalled)
	require.True(t, client.tfCalled)
	require.True(t, scaler.downCalled)
	require.True(t, scaler.upCalled)
	require.Equal(t, []string{"scaledown", "wait", "db", "tf", "scaleup"}, client.order,
		"must scale down, wait, restore databases, restore terraform, then scale up")
}

func Test_Run_ScaleDownFailureStopsBeforeRestore(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := &fakeStateRestoreClient{}
	r, scaler := newTestRunner(t, ctrl, client)
	scaler.scaleDownErr = errors.New("scale down boom")

	err := r.Run(context.Background())
	require.ErrorContains(t, err, "scale down boom")
	require.False(t, client.waited, "no restore should run if scale down failed")
	require.False(t, scaler.upCalled, "scale up should not run if scale down failed")
}

func Test_Run_WaitFailureStopsBeforeRestore(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := &fakeStateRestoreClient{waitErr: errors.New("db never ready")}
	r, scaler := newTestRunner(t, ctrl, client)

	err := r.Run(context.Background())
	require.ErrorContains(t, err, "db never ready")
	require.False(t, client.dbCalled)
	require.False(t, client.tfCalled)
	require.True(t, scaler.upCalled, "control plane must be scaled back up even when a restore step fails")
}

func Test_Run_DatabaseRestoreFailureScalesBackUp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := &fakeStateRestoreClient{restoreDBErr: errors.New("psql boom")}
	r, scaler := newTestRunner(t, ctrl, client)

	err := r.Run(context.Background())
	require.ErrorContains(t, err, "psql boom")
	require.False(t, client.tfCalled, "terraform restore must not run after database restore failure")
	require.True(t, scaler.upCalled, "control plane must be scaled back up after a failed restore")
}

// Test_Run_ArchiveOpenFailureIsReturned verifies that when the archive cannot be opened (for
// example, running outside a git repository) Run returns the wrapped error before touching the
// control plane.
func Test_Run_ArchiveOpenFailureIsReturned(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	archive := statearchive.NewMockArchive(ctrl)
	archive.EXPECT().Open(gomock.Any(), pgbackup.StateBranchName()).Return(nil, errors.New("not a git repo")).Times(1)

	client := &fakeStateRestoreClient{}
	r := &Runner{
		Output:      &output.MockOutput{},
		Workspace:   kubernetesWorkspace(),
		StateClient: client,
		Archive:     archive,
		newScaler: func(kubeContext, namespace string) (ControlPlaneScaler, error) {
			t.Fatal("newScaler must not be called when the archive cannot be opened")
			return nil, nil
		},
	}

	err := r.Run(context.Background())
	require.ErrorContains(t, err, "failed to open state archive")
	require.ErrorContains(t, err, "not a git repo")
	require.False(t, client.waited, "no restore should run when the archive cannot be opened")
}
