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
	"fmt"
	"net/http"
	"testing"

	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/cli/clients"
	generated "github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/cmd/app/status"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Status command with flag",
			Input:         []string{"-a", "test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Status command with positional arg",
			Input:         []string{"test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Status command with fallback workspace",
			Input:         []string{"--application", "test-app", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Status command with incorrect args",
			Input:         []string{"foo", "bar"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func mustParse(t *testing.T, s string) resources.ID {
	t.Helper()
	id, err := resources.ParseResource(s)
	require.NoError(t, err)
	return id
}

func Test_Run(t *testing.T) {
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	t.Run("Success: application with no resources", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		appID := workspace.Scope + "/providers/Radius.Core/applications/test-app"

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInApplication(gomock.Any(), appID).
			Return([]generated.GenericResource{}, nil).
			Times(1)

		mockDiag := clients.NewMockDiagnosticsClient(ctrl)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory: &connections.MockFactory{
				ApplicationsManagementClient: mockMgmt,
				DiagnosticsClient:            mockDiag,
			},
			Workspace:       workspace,
			ApplicationName: "test-app",
			Format:          "table",
			Output:          outputSink,
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)
		require.Len(t, outputSink.Writes, 1)

		formatted, ok := outputSink.Writes[0].(output.FormattedOutput)
		require.True(t, ok)
		require.Equal(t, status.StatusFormat(), formatted.Options)

		appStatus, ok := formatted.Obj.(clients.ApplicationStatus)
		require.True(t, ok)
		require.Equal(t, "test-app", appStatus.Name)
		require.Equal(t, 0, appStatus.ResourceCount)
	})

	t.Run("Success: application with resources and gateways", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		appID := workspace.Scope + "/providers/Radius.Core/applications/test-app"
		containerID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/containers/web"
		containerName := "web"
		containerType := "Applications.Core/containers"
		gatewayID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/gateways/gw"
		gatewayName := "gw"
		gatewayType := "Applications.Core/gateways"

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInApplication(gomock.Any(), appID).
			Return([]generated.GenericResource{
				{ID: &containerID, Name: &containerName, Type: &containerType},
				{ID: &gatewayID, Name: &gatewayName, Type: &gatewayType},
			}, nil).
			Times(1)

		containerParsedID := mustParse(t, containerID)
		gatewayParsedID := mustParse(t, gatewayID)
		endpoint := "http://gw.example.com"

		mockDiag := clients.NewMockDiagnosticsClient(ctrl)
		mockDiag.EXPECT().
			GetPublicEndpoint(gomock.Any(), clients.EndpointOptions{ResourceID: containerParsedID}).
			Return(nil, nil).
			Times(1)
		mockDiag.EXPECT().
			GetPublicEndpoint(gomock.Any(), clients.EndpointOptions{ResourceID: gatewayParsedID}).
			Return(&endpoint, nil).
			Times(1)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory: &connections.MockFactory{
				ApplicationsManagementClient: mockMgmt,
				DiagnosticsClient:            mockDiag,
			},
			Workspace:       workspace,
			ApplicationName: "test-app",
			Format:          "table",
			Output:          outputSink,
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)

		// Should have: status table, blank line, gateway table
		require.Len(t, outputSink.Writes, 3)

		// Verify status
		formatted, ok := outputSink.Writes[0].(output.FormattedOutput)
		require.True(t, ok)
		appStatus, ok := formatted.Obj.(clients.ApplicationStatus)
		require.True(t, ok)
		require.Equal(t, 2, appStatus.ResourceCount)
		require.Len(t, appStatus.Gateways, 1)
		require.Equal(t, "gw", appStatus.Gateways[0].Name)
		require.Equal(t, "http://gw.example.com", appStatus.Gateways[0].Endpoint)

		// Verify gateway table format
		gwFormatted, ok := outputSink.Writes[2].(output.FormattedOutput)
		require.True(t, ok)
		require.Equal(t, status.GatewayFormat(), gwFormatted.Options)
	})

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

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "table",
			Output:                  &output.MockOutput{},
		}

		err = runner.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "was not found or has been deleted")
	})

	t.Run("Success: JSON output format", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		appID := workspace.Scope + "/providers/Radius.Core/applications/test-app"

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInApplication(gomock.Any(), appID).
			Return([]generated.GenericResource{}, nil).
			Times(1)

		mockDiag := clients.NewMockDiagnosticsClient(ctrl)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory: &connections.MockFactory{
				ApplicationsManagementClient: mockMgmt,
				DiagnosticsClient:            mockDiag,
			},
			Workspace:       workspace,
			ApplicationName: "test-app",
			Format:          "json",
			Output:          outputSink,
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)
		require.Len(t, outputSink.Writes, 1)

		formatted, ok := outputSink.Writes[0].(output.FormattedOutput)
		require.True(t, ok)
		require.Equal(t, "json", formatted.Format)
	})

	t.Run("Error: resource list failure is propagated", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		appID := workspace.Scope + "/providers/Radius.Core/applications/test-app"

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInApplication(gomock.Any(), appID).
			Return(nil, fmt.Errorf("simulated error")).
			Times(1)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "table",
			Output:                  &output.MockOutput{},
		}

		err = runner.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "simulated error")
	})
}
