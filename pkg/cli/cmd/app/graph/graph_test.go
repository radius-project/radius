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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/graph/persistence"
	gitstore "github.com/radius-project/radius/pkg/graph/persistence/git"
	"github.com/radius-project/radius/test/radcli"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	application := corerpv20231001preview.ApplicationResource{
		Name: new("test-app"),
		ID:   new(applicationResourceID),
		Type: new("Applications.Core/applications"),
		Properties: &corerpv20231001preview.ApplicationProperties{
			Environment: new(environmentResourceID),
		},
	}

	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Graph command application (positional)",
			Input:         []string{"test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetApplication(gomock.Any(), "test-app").
					Return(application, nil).
					Times(1)
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				// These values are used by Run()
				require.Equal(t, "test-app", runner.ApplicationName)
			},
		},
		{
			Name:          "Graph command application (flag)",
			Input:         []string{"-a", "test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetApplication(gomock.Any(), "test-app").
					Return(application, nil).
					Times(1)
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				// These values are used by Run()
				require.Equal(t, "test-app", runner.ApplicationName)
			},
		},
		{
			Name:          "Graph command missing application",
			Input:         []string{"-a", "test-app"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetApplication(gomock.Any(), "test-app").
					Return(corerpv20231001preview.ApplicationResource{}, &azcore.ResponseError{ErrorCode: v1.CodeNotFound}).
					Times(1)
			},
		},
		{
			Name:          "Graph command with incorrect args",
			Input:         []string{"foo", "bar"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	// This example is a very simple example of the application graph as an integration test.
	// The unit tests for this package cover the more complex cases.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	graph := corerpv20231001preview.ApplicationGraphResponse{
		Resources: []*corerpv20231001preview.ApplicationGraphResource{
			{
				ID:                new(containerResourceID),
				Name:              new(containerResourceName),
				Type:              new(containerResourceType),
				ProvisioningState: new(provisioningStateSuccess),
				OutputResources: []*corerpv20231001preview.ApplicationGraphOutputResource{
					{
						ID:   new("/planes/radius/local/resourcegroups/test-group/providers/kubernetes/Deployments/demo"),
						Type: new("kubernetes: apps/Deployment"),
						Name: new("demo"),
					},
				},
				Connections: []*corerpv20231001preview.ApplicationGraphConnection{
					{
						ID:        new(redisResourceID),
						Direction: &directionOutbound,
					},
				},
			},
			{
				ID:                new(redisResourceID),
				Name:              new(redisResourceName),
				Type:              new(redisResourceType),
				ProvisioningState: new(provisioningStateSuccess),
				OutputResources: []*corerpv20231001preview.ApplicationGraphOutputResource{
					{
						ID:   new("/planes/radius/local/resourcegroups/test-group/providers/AWS.MemoryDB/Cluster/redis-aqbjixghynqgg"),
						Type: new("aws: AWS.MemoryDB/Cluster"),
						Name: new("redis-aqbjixghynqgg"),
					},
				},
				Connections: []*corerpv20231001preview.ApplicationGraphConnection{
					{
						ID:        new(containerResourceID),
						Direction: &directionInbound,
					},
				},
			},
		},
	}

	appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
	appManagementClient.EXPECT().
		GetApplicationGraph(gomock.Any(), "test-app").
		Return(graph, nil).
		Times(1)

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind":    "kubernetes",
			"context": "kind-kind",
		},
		Name:  "kind-kind",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}
	outputSink := &output.MockOutput{}
	runner := &Runner{
		ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
		Workspace:         workspace,
		Output:            outputSink,

		// Populated by Validate()
		ApplicationName: "test-app",
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	expectedOutput := `Displaying application: test-app

Name: webapp (Applications.Core/containers)
Connections:
  webapp -> redis (Applications.Datastores/redisCaches)
Resources:
  demo (kubernetes: apps/Deployment)

Name: redis (Applications.Datastores/redisCaches)
Connections:
  webapp (Applications.Core/containers) -> redis
Resources:
  redis-aqbjixghynqgg (aws: AWS.MemoryDB/Cluster)

`

	expected := []any{
		output.LogOutput{
			Format: expectedOutput,
		},
	}

	require.Equal(t, expected, outputSink.Writes)
}

func Test_Run_JSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	graph := corerpv20231001preview.ApplicationGraphResponse{
		Resources: []*corerpv20231001preview.ApplicationGraphResource{
			{
				ID:                new(containerResourceID),
				Name:              new(containerResourceName),
				Type:              new(containerResourceType),
				ProvisioningState: new(provisioningStateSuccess),
				OutputResources: []*corerpv20231001preview.ApplicationGraphOutputResource{
					{
						ID:   new("/planes/radius/local/resourcegroups/test-group/providers/kubernetes/Deployments/demo"),
						Type: new("kubernetes: apps/Deployment"),
						Name: new("demo"),
					},
				},
				Connections: []*corerpv20231001preview.ApplicationGraphConnection{
					{
						ID:        new(redisResourceID),
						Direction: &directionOutbound,
					},
				},
			},
		},
	}

	appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
	appManagementClient.EXPECT().
		GetApplicationGraph(gomock.Any(), "test-app").
		Return(graph, nil).
		Times(1)

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind":    "kubernetes",
			"context": "kind-kind",
		},
		Name:  "kind-kind",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}
	outputSink := &output.MockOutput{}
	runner := &Runner{
		ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
		Workspace:         workspace,
		Output:            outputSink,
		Format:            output.FormatJson,

		// Populated by Validate()
		ApplicationName: "test-app",
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	require.Len(t, outputSink.Writes, 1)
	formatted, ok := outputSink.Writes[0].(output.FormattedOutput)
	require.True(t, ok, "expected FormattedOutput but got %T", outputSink.Writes[0])
	require.Equal(t, output.FormatJson, formatted.Format)
	require.Equal(t, graph, formatted.Obj)
}

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
