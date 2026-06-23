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

func kubernetesWorkspace() *workspaces.Workspace {
	return &workspaces.Workspace{
		Name: "test",
		Connection: map[string]any{
			"kind":    workspaces.KindKubernetes,
			"context": "k3d-test",
		},
	}
}

func newTestRunner(t *testing.T, client *fakeStateRestoreClient) (*Runner, *fakeWorktree) {
	t.Helper()
	wt := &fakeWorktree{path: t.TempDir()}
	r := &Runner{
		Output:      &output.MockOutput{},
		Workspace:   kubernetesWorkspace(),
		StateClient: client,
		openWorktree: func(ctx context.Context) (worktreeHandle, error) {
			return worktreeHandle{path: wt.path, remove: wt.remove}, nil
		},
	}
	return r, wt
}

func Test_Run_RestoresInOrderWaitDatabaseTerraform(t *testing.T) {
	client := &fakeStateRestoreClient{}
	r, wt := newTestRunner(t, client)

	err := r.Run(context.Background())
	require.NoError(t, err)

	require.True(t, client.waited)
	require.True(t, client.dbCalled)
	require.True(t, client.tfCalled)
	require.Equal(t, []string{"wait", "db", "tf"}, client.order, "must wait, then restore databases, then terraform")
	require.True(t, wt.removed)
}

func Test_Run_WaitFailureStopsBeforeRestore(t *testing.T) {
	client := &fakeStateRestoreClient{waitErr: errors.New("db never ready")}
	r, wt := newTestRunner(t, client)

	err := r.Run(context.Background())
	require.ErrorContains(t, err, "db never ready")
	require.False(t, client.dbCalled)
	require.False(t, client.tfCalled)
	require.True(t, wt.removed)
}

func Test_Run_DatabaseRestoreFailureStopsBeforeTerraform(t *testing.T) {
	client := &fakeStateRestoreClient{restoreDBErr: errors.New("psql boom")}
	r, wt := newTestRunner(t, client)

	err := r.Run(context.Background())
	require.ErrorContains(t, err, "psql boom")
	require.False(t, client.tfCalled, "terraform restore must not run after database restore failure")
	require.True(t, wt.removed)
}
