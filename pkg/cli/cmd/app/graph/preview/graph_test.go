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
				applicationName string,
				body any,
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

	t.Run("Error: application not found (404)", func(t *testing.T) {
		notFoundServer := func() fake.ApplicationsServer {
			return fake.ApplicationsServer{
				GetGraph: func(
					ctx context.Context,
					applicationName string,
					body any,
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
				applicationName string,
				body any,
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
