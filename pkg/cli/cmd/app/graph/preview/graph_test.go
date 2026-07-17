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

package preview

import (
	"context"
	"net/http"
	"testing"

	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Graph command with flag",
			Input:         []string{"-a", "test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Graph command with positional arg",
			Input:         []string{"test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Graph command with fallback workspace",
			Input:         []string{"--application", "test-app", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Graph command with incorrect args",
			Input:         []string{"foo", "bar"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Enriched: -a name plus bicep file path",
			Input:         []string{"-a", "test-app", "./app.bicep"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Enriched: bicep file path without -a name is invalid",
			Input:         []string{"./app.bicep"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	t.Run("Success: empty graph (table)", func(t *testing.T) {
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "table",
			Output:                  outputSink,
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)
		require.Len(t, outputSink.Writes, 1)

		logOutput, ok := outputSink.Writes[0].(output.LogOutput)
		require.True(t, ok)
		require.Contains(t, logOutput.Format, "Displaying application: test-app")
		require.Contains(t, logOutput.Format, "(empty)")
	})

	t.Run("Success: graph with resources (table)", func(t *testing.T) {
		graphServer := func() fake.ApplicationsServer {
			srv := test_client_factory.WithApplicationsServerNoError()
			srv.GetGraph = func(
				ctx context.Context,
				rootScope string,
				applicationName string,
				body corerpv20250801.GetGraphRequest,
				options *corerpv20250801.ApplicationsClientGetGraphOptions,
			) (resp azfake.Responder[corerpv20250801.ApplicationsClientGetGraphResponse], errResp azfake.ErrorResponder) {
				containerID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/containers/web"
				containerName := "web"
				containerType := "Applications.Core/containers"

				resp.SetResponse(http.StatusOK, corerpv20250801.ApplicationsClientGetGraphResponse{
					ApplicationGraphResponse: corerpv20250801.ApplicationGraphResponse{
						Resources: []*corerpv20250801.ApplicationGraphResource{
							{
								ID:              &containerID,
								Name:            &containerName,
								Type:            &containerType,
								Connections:     []*corerpv20250801.ApplicationGraphConnection{},
								OutputResources: []*corerpv20250801.ApplicationGraphOutputResource{},
							},
						},
					},
				}, nil)
				return
			}
			return srv
		}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, graphServer)
		require.NoError(t, err)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "table",
			Output:                  outputSink,
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)
		require.Len(t, outputSink.Writes, 1)

		logOutput, ok := outputSink.Writes[0].(output.LogOutput)
		require.True(t, ok)
		require.Contains(t, logOutput.Format, "Name: web (Applications.Core/containers)")
	})

	t.Run("Success: graph (JSON)", func(t *testing.T) {
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "json",
			Output:                  outputSink,
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)
		require.Len(t, outputSink.Writes, 1)

		formatted, ok := outputSink.Writes[0].(output.FormattedOutput)
		require.True(t, ok)
		require.Equal(t, "json", formatted.Format)
	})

	// Assert the --include-icons flag threads through into GetGraphRequest.IncludeIcons.
	// The default (flag not set) must send nil so the server treats it as false.
	t.Run("Success: IncludeIcons defaults to nil on request body", func(t *testing.T) {
		var received corerpv20250801.GetGraphRequest
		observeServer := func() fake.ApplicationsServer {
			srv := test_client_factory.WithApplicationsServerNoError()
			srv.GetGraph = func(
				ctx context.Context,
				rootScope string,
				applicationName string,
				body corerpv20250801.GetGraphRequest,
				options *corerpv20250801.ApplicationsClientGetGraphOptions,
			) (resp azfake.Responder[corerpv20250801.ApplicationsClientGetGraphResponse], errResp azfake.ErrorResponder) {
				received = body
				resp.SetResponse(http.StatusOK, corerpv20250801.ApplicationsClientGetGraphResponse{}, nil)
				return
			}
			return srv
		}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, observeServer)
		require.NoError(t, err)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "table",
			IncludeIcons:            false,
			Output:                  &output.MockOutput{},
		}

		require.NoError(t, runner.Run(context.Background()))
		require.Nil(t, received.IncludeIcons, "IncludeIcons must be nil by default (opt-in only)")
	})

	t.Run("Success: --include-icons forwards includeIcons=true", func(t *testing.T) {
		var received corerpv20250801.GetGraphRequest
		observeServer := func() fake.ApplicationsServer {
			srv := test_client_factory.WithApplicationsServerNoError()
			srv.GetGraph = func(
				ctx context.Context,
				rootScope string,
				applicationName string,
				body corerpv20250801.GetGraphRequest,
				options *corerpv20250801.ApplicationsClientGetGraphOptions,
			) (resp azfake.Responder[corerpv20250801.ApplicationsClientGetGraphResponse], errResp azfake.ErrorResponder) {
				received = body
				resp.SetResponse(http.StatusOK, corerpv20250801.ApplicationsClientGetGraphResponse{}, nil)
				return
			}
			return srv
		}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, observeServer)
		require.NoError(t, err)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "json",
			IncludeIcons:            true,
			Output:                  &output.MockOutput{},
		}

		require.NoError(t, runner.Run(context.Background()))
		require.NotNil(t, received.IncludeIcons)
		require.True(t, *received.IncludeIcons)
	})

	t.Run("Error: application not found (404)", func(t *testing.T) {
		notFoundServer := func() fake.ApplicationsServer {
			return fake.ApplicationsServer{
				GetGraph: func(
					ctx context.Context,
					rootScope string,
					applicationName string,
					body corerpv20250801.GetGraphRequest,
					options *corerpv20250801.ApplicationsClientGetGraphOptions,
				) (resp azfake.Responder[corerpv20250801.ApplicationsClientGetGraphResponse], errResp azfake.ErrorResponder) {
					errResp.SetResponseError(http.StatusNotFound, "NotFound")
					return
				},
			}
		}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, notFoundServer)
		require.NoError(t, err)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "table",
			Output:                  &output.MockOutput{},
		}

		err = runner.Run(context.Background())
		require.Error(t, err)
		require.Equal(t, clierrors.Message("Application %q does not exist or has been deleted.", "test-app"), err)
	})

	t.Run("Error: GetGraph failure is propagated", func(t *testing.T) {
		graphErrorServer := func() fake.ApplicationsServer {
			srv := test_client_factory.WithApplicationsServerNoError()
			srv.GetGraph = func(
				ctx context.Context,
				rootScope string,
				applicationName string,
				body corerpv20250801.GetGraphRequest,
				options *corerpv20250801.ApplicationsClientGetGraphOptions,
			) (resp azfake.Responder[corerpv20250801.ApplicationsClientGetGraphResponse], errResp azfake.ErrorResponder) {
				errResp.SetResponseError(http.StatusInternalServerError, "InternalServerError")
				return
			}
			return srv
		}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, graphErrorServer)
		require.NoError(t, err)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "table",
			Output:                  &output.MockOutput{},
		}

		err = runner.Run(context.Background())
		require.Error(t, err)
	})
}

// Test_Run_EnrichedMode asserts that when a Bicep path is provided the runner
// compiles the template and attaches the extracted dependsOn edges to
// GetGraphRequest.DependsOnEdges. The transport-shape and merge-policy sides
// are covered elsewhere (ExtractDependsOnEdges and MergeDependencyEdges tests
// in pkg/cli/graph and pkg/graph/edges); this test only pins the CLI-side
// plumbing between BicepFilePath, PrepareTemplate, and the request body.
func Test_Run_EnrichedMode(t *testing.T) {
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	// Template: a container that dependsOn a rabbitmq queue via
	// resourceId(). ExtractDependsOnEdges resolves both endpoints to
	// canonical Radius IDs.
	template := map[string]any{
		"resources": []any{
			map[string]any{
				"type": "Radius.Compute/containers",
				"name": "consumer",
				"dependsOn": []any{
					"[resourceId('Radius.Messaging/rabbitMQQueues', 'queue')]",
				},
			},
			map[string]any{
				"type": "Radius.Messaging/rabbitMQQueues",
				"name": "queue",
			},
		},
	}
	const (
		consumerID = "/planes/radius/local/resourcegroups/default/providers/Radius.Compute/containers/consumer"
		queueID    = "/planes/radius/local/resourcegroups/default/providers/Radius.Messaging/rabbitMQQueues/queue"
	)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate("./app.bicep").
		Return(template, nil).
		Times(1)

	var received corerpv20250801.GetGraphRequest
	observeServer := func() fake.ApplicationsServer {
		srv := test_client_factory.WithApplicationsServerNoError()
		srv.GetGraph = func(
			ctx context.Context,
			rootScope string,
			applicationName string,
			body corerpv20250801.GetGraphRequest,
			options *corerpv20250801.ApplicationsClientGetGraphOptions,
		) (resp azfake.Responder[corerpv20250801.ApplicationsClientGetGraphResponse], errResp azfake.ErrorResponder) {
			received = body
			resp.SetResponse(http.StatusOK, corerpv20250801.ApplicationsClientGetGraphResponse{}, nil)
			return
		}
		return srv
	}

	factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, observeServer)
	require.NoError(t, err)

	runner := &Runner{
		RadiusCoreClientFactory: factory,
		Bicep:                   bicepMock,
		Workspace:               workspace,
		ApplicationName:         "test-app",
		BicepFilePath:           "./app.bicep",
		Format:                  "table",
		Output:                  &output.MockOutput{},
	}

	require.NoError(t, runner.Run(context.Background()))

	require.NotNil(t, received.DependsOnEdges, "enriched mode must forward extracted edges")
	entries, ok := received.DependsOnEdges[consumerID]
	require.True(t, ok, "consumer entry missing; got keys %v", received.DependsOnEdges)
	require.Len(t, entries, 1)
	require.NotNil(t, entries[0].ID)
	require.Equal(t, queueID, *entries[0].ID)
	require.NotNil(t, entries[0].Direction)
	require.Equal(t, corerpv20250801.DirectionOutbound, *entries[0].Direction)
	require.NotNil(t, entries[0].Kind)
	require.Equal(t, corerpv20250801.ConnectionKindDependency, *entries[0].Kind)
}

// Test_Run_EnrichedMode_CompileError_Wrapped asserts that a Bicep compile
// failure is surfaced as a clierrors.Message so users see the file path in the
// error rather than the raw underlying error.
func Test_Run_EnrichedMode_CompileError_Wrapped(t *testing.T) {
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate("./bad.bicep").
		Return(nil, clierrors.Message("syntax error"))

	factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
	require.NoError(t, err)

	runner := &Runner{
		RadiusCoreClientFactory: factory,
		Bicep:                   bicepMock,
		Workspace:               workspace,
		ApplicationName:         "test-app",
		BicepFilePath:           "./bad.bicep",
		Format:                  "table",
		Output:                  &output.MockOutput{},
	}

	err = runner.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "./bad.bicep")
}
