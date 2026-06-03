// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package graph

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/output"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/graph/persistence"
	gitstore "github.com/radius-project/radius/pkg/graph/persistence/git"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const sampleBicepPath = "/tmp/app.bicep"

// sampleTemplate returns a minimal ARM template containing a single
// Applications.Core/containers resource.
func sampleTemplate() map[string]any {
	return map[string]any{
		"resources": []any{
			map[string]any{
				"type":       "Applications.Core/containers",
				"name":       "frontend",
				"properties": map[string]any{"image": "nginx"},
			},
		},
	}
}

func TestIsModeledGraphArg(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"":                 false,
		"my-app":           false,
		"./app.bicep":      true,
		"/abs/app.BICEP":   true,
		"./app.json":       false,
		"./app.txt":        false,
		"./nested/x.bicep": true,
	}
	for in, want := range cases {
		require.Equal(t, want, isModeledGraphArg(in), in)
	}
}

func TestRunner_RunModeled_LocalFilesystem(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	withTempCwd(t)
	t.Setenv("GITHUB_ACTIONS", "")

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(sampleBicepPath).
		Return(sampleTemplate(), nil).
		Times(1)

	runner := &Runner{
		Bicep:         bicepMock,
		Output:        &output.MockOutput{},
		BicepFilePath: sampleBicepPath,
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	contents, err := os.ReadFile(defaultModeledGraphFile)
	require.NoError(t, err)
	require.Contains(t, string(contents), "frontend")
	require.Contains(t, string(contents), "Applications.Core/containers")
}

func TestRunner_RunModeled_OrphanBranchPersistence(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	withTempCwd(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_HEAD_REF", "feature/foo")
	t.Setenv("GITHUB_REF_NAME", "42/merge")

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(sampleBicepPath).
		Return(sampleTemplate(), nil).
		Times(1)

	storeMock := persistence.NewMockStore(ctrl)
	expectedKey := persistence.Key{Namespace: "feature/foo", Name: modeledGraphKeyName}
	storeMock.EXPECT().
		Save(gomock.Any(), expectedKey, gomock.Any(), gomock.Any()).
		DoAndReturn(saveAssertion(t, "feature/foo", "frontend")).
		Times(1)

	runner := &Runner{
		Bicep:         bicepMock,
		Output:        &output.MockOutput{},
		BicepFilePath: sampleBicepPath,
		GraphStore:    storeMock,
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	_, statErr := os.Stat(defaultModeledGraphFile)
	require.True(t, os.IsNotExist(statErr), "modeled graph must not be written locally in repo-radius mode")
}

func TestRunner_RunModeled_FallsBackToRefName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	withTempCwd(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_HEAD_REF", "")
	t.Setenv("GITHUB_REF_NAME", "main")

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(sampleBicepPath).
		Return(sampleTemplate(), nil).
		Times(1)

	storeMock := persistence.NewMockStore(ctrl)
	storeMock.EXPECT().
		Save(gomock.Any(), persistence.Key{Namespace: "main", Name: modeledGraphKeyName}, gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	runner := &Runner{
		Bicep:         bicepMock,
		Output:        &output.MockOutput{},
		BicepFilePath: sampleBicepPath,
		GraphStore:    storeMock,
	}

	require.NoError(t, runner.Run(context.Background()))
}

func TestRunner_RunModeled_NoBranchInEnv(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	withTempCwd(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_HEAD_REF", "")
	t.Setenv("GITHUB_REF_NAME", "")

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(sampleBicepPath).
		Return(sampleTemplate(), nil).
		Times(1)

	runner := &Runner{
		Bicep:         bicepMock,
		Output:        &output.MockOutput{},
		BicepFilePath: sampleBicepPath,
		GraphStore:    persistence.NewMockStore(ctrl),
	}

	err := runner.Run(context.Background())
	require.Error(t, err)
}

func TestRunner_RunModeled_BicepCompileError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	withTempCwd(t)
	t.Setenv("GITHUB_ACTIONS", "")

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(sampleBicepPath).
		Return(nil, errors.New("syntax error")).
		Times(1)

	runner := &Runner{
		Bicep:         bicepMock,
		Output:        &output.MockOutput{},
		BicepFilePath: sampleBicepPath,
	}

	err := runner.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "syntax error")
}

func TestRunner_RunModeled_NilGraphStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	withTempCwd(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_HEAD_REF", "feature/foo")

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(sampleBicepPath).
		Return(sampleTemplate(), nil).
		Times(1)

	runner := &Runner{
		Bicep:         bicepMock,
		Output:        &output.MockOutput{},
		BicepFilePath: sampleBicepPath,
		GraphStore:    nil,
	}

	err := runner.Run(context.Background())
	require.Error(t, err)
}

func TestRunner_RunModeled_StoreSaveError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	withTempCwd(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_HEAD_REF", "feature/foo")

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(sampleBicepPath).
		Return(sampleTemplate(), nil).
		Times(1)

	storeMock := persistence.NewMockStore(ctrl)
	storeMock.EXPECT().
		Save(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("push rejected")).
		Times(1)

	runner := &Runner{
		Bicep:         bicepMock,
		Output:        &output.MockOutput{},
		BicepFilePath: sampleBicepPath,
		GraphStore:    storeMock,
	}

	err := runner.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "push rejected")
	require.Contains(t, err.Error(), gitstore.DefaultGraphBranch)
}

// withTempCwd switches the current working directory to a freshly-created
// temp directory and restores the original on test cleanup.
func withTempCwd(t *testing.T) {
	t.Helper()
	original, err := os.Getwd()
	require.NoError(t, err)
	tmp := t.TempDir()
	require.NoError(t, os.Chdir(tmp))
	t.Cleanup(func() {
		_ = os.Chdir(original)
	})
	require.True(t, filepath.IsAbs(tmp))
}

// saveAssertion returns a Save implementation that asserts the inbound
// payload before recording success.
func saveAssertion(t *testing.T, wantBranch, wantResource string) func(ctx context.Context, key persistence.Key, graph *corerpv20250801preview.ApplicationGraphResponse, opts persistence.SaveOptions) error {
	t.Helper()
	return func(_ context.Context, key persistence.Key, graph *corerpv20250801preview.ApplicationGraphResponse, opts persistence.SaveOptions) error {
		require.Equal(t, wantBranch, key.Namespace)
		require.Equal(t, modeledGraphKeyName, key.Name)
		require.NotNil(t, graph)
		require.Len(t, graph.Resources, 1)
		require.Equal(t, wantResource, *graph.Resources[0].Name)
		require.Contains(t, opts.Message, wantBranch)
		return nil
	}
}
