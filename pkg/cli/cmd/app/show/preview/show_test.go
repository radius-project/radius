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
	"github.com/radius-project/radius/pkg/cli/objectformats"
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
			Name:          "Show Command with flag",
			Input:         []string{"-a", "test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with positional arg",
			Input:         []string{"test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with fallback workspace",
			Input:         []string{"--application", "test-app", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Show Command with incorrect args",
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
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	testcases := []struct {
		name            string
		appFactory      func() fake.ApplicationsServer
		applicationName string
		expectedOutput  []any
	}{
		{
			name:            "application found",
			appFactory:      test_client_factory.WithApplicationsServerNoError,
			applicationName: "test-app",
			expectedOutput: []any{
				output.FormattedOutput{
					Format: "table",
					Obj: corerpv20250801.ApplicationResource{
						Name: new("test-app"),
					},
					Options: objectformats.GetResourceTableFormat(),
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, tc.appFactory)
			require.NoError(t, err)

			outputSink := &output.MockOutput{}
			runner := &Runner{
				RadiusCoreClientFactory: factory,
				Workspace:               workspace,
				ApplicationName:         tc.applicationName,
				Format:                  "table",
				Output:                  outputSink,
			}

			err = runner.Run(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.expectedOutput, outputSink.Writes)
		})
	}

	t.Run("Error: application not found (404)", func(t *testing.T) {
		notFoundServer := func() fake.ApplicationsServer {
			return fake.ApplicationsServer{
				Get: func(
					ctx context.Context,
					applicationName string,
					options *corerpv20250801.ApplicationsClientGetOptions,
				) (resp azfake.Responder[corerpv20250801.ApplicationsClientGetResponse], errResp azfake.ErrorResponder) {
					errResp.SetResponseError(http.StatusNotFound, "NotFound")
					return
				},
			}
		}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, notFoundServer)
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
		require.Error(t, err)
		require.Equal(t, clierrors.Message("The application %q was not found or has been deleted.", "test-app"), err)
		require.Empty(t, outputSink.Writes)
	})

	t.Run("Error: non-404 error is propagated", func(t *testing.T) {
		serverError := func() fake.ApplicationsServer {
			return fake.ApplicationsServer{
				Get: func(
					ctx context.Context,
					applicationName string,
					options *corerpv20250801.ApplicationsClientGetOptions,
				) (resp azfake.Responder[corerpv20250801.ApplicationsClientGetResponse], errResp azfake.ErrorResponder) {
					errResp.SetResponseError(http.StatusInternalServerError, "InternalServerError")
					return
				},
			}
		}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, serverError)
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
		require.Error(t, err)
		// Should NOT be the 404 user-facing message.
		require.NotEqual(t, clierrors.Message("The application %q was not found or has been deleted.", "test-app"), err)
		require.Empty(t, outputSink.Writes)
	})

	t.Run("Success: json output format", func(t *testing.T) {
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
		require.Equal(t, []any{
			output.FormattedOutput{
				Format:  "json",
				Obj:     corerpv20250801.ApplicationResource{Name: new("test-app")},
				Options: objectformats.GetResourceTableFormat(),
			},
		}, outputSink.Writes)
	})
}
