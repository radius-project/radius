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

	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/stretchr/testify/require"
)

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

type fakeWorktree struct {
	path    string
	removed bool
}

func (w *fakeWorktree) remove(ctx context.Context) { w.removed = true }

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

func newTestRunner(t *testing.T, client *fakeStateRestoreClient) (*Runner, *fakeWorktree, *fakeScaler) {
	t.Helper()
	wt := &fakeWorktree{path: t.TempDir()}
	scaler := &fakeScaler{order: &client.order}
	r := &Runner{
		Output:      &output.MockOutput{},
		Workspace:   kubernetesWorkspace(),
		StateClient: client,
		openWorktree: func(ctx context.Context) (worktreeHandle, error) {
			return worktreeHandle{path: wt.path, remove: wt.remove}, nil
		},
		newScaler: func(kubeContext, namespace string) (ControlPlaneScaler, error) {
			return scaler, nil
		},
	}
	return r, wt, scaler
}

func Test_Run_RestoresInOrderWaitDatabaseTerraform(t *testing.T) {
	client := &fakeStateRestoreClient{}
	r, wt, scaler := newTestRunner(t, client)

	err := r.Run(context.Background())
	require.NoError(t, err)

	require.True(t, client.waited)
	require.True(t, client.dbCalled)
	require.True(t, client.tfCalled)
	require.True(t, scaler.downCalled)
	require.True(t, scaler.upCalled)
	require.Equal(t, []string{"scaledown", "wait", "db", "tf", "scaleup"}, client.order,
		"must scale down, wait, restore databases, restore terraform, then scale up")
	require.True(t, wt.removed)
}

func Test_Run_ScaleDownFailureStopsBeforeRestore(t *testing.T) {
	client := &fakeStateRestoreClient{}
	r, wt, scaler := newTestRunner(t, client)
	scaler.scaleDownErr = errors.New("scale down boom")

	err := r.Run(context.Background())
	require.ErrorContains(t, err, "scale down boom")
	require.False(t, client.waited, "no restore should run if scale down failed")
	require.False(t, scaler.upCalled, "scale up should not run if scale down failed")
	require.True(t, wt.removed)
}

func Test_Run_WaitFailureStopsBeforeRestore(t *testing.T) {
	client := &fakeStateRestoreClient{waitErr: errors.New("db never ready")}
	r, wt, scaler := newTestRunner(t, client)

	err := r.Run(context.Background())
	require.ErrorContains(t, err, "db never ready")
	require.False(t, client.dbCalled)
	require.False(t, client.tfCalled)
	require.True(t, scaler.upCalled, "control plane must be scaled back up even when a restore step fails")
	require.True(t, wt.removed)
}

func Test_Run_DatabaseRestoreFailureScalesBackUp(t *testing.T) {
	client := &fakeStateRestoreClient{restoreDBErr: errors.New("psql boom")}
	r, wt, scaler := newTestRunner(t, client)

	err := r.Run(context.Background())
	require.ErrorContains(t, err, "psql boom")
	require.False(t, client.tfCalled, "terraform restore must not run after database restore failure")
	require.True(t, scaler.upCalled, "control plane must be scaled back up after a failed restore")
	require.True(t, wt.removed)
}
