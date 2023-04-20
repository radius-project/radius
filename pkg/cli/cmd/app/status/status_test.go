// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package status

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/corerp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	config := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Status Command with default application",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadConfigWithWorkspaceAndApplication(t),
			},
		},
		{
			Name:          "Status Command with flag",
			Input:         []string{"-a", "test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
		},
		{
			Name:          "Status Command with positional arg",
			Input:         []string{"test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
		},
		{
			Name:          "Status Command with fallback workspace",
			Input:         []string{"--application", "test-app", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Status Command with incorrect args",
			Input:         []string{"foo", "bar"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Success: Application Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		application := v20230415preview.ApplicationResource{
			Name: to.Ptr("test-app"),
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			ShowApplication(gomock.Any(), "test-app").
			Return(application, nil).
			Times(1)

		resourceList := []generated.GenericResource{
			{
				Name: to.Ptr("test-container"),
				ID:   to.Ptr("/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/containers/test-container"),
			},
			{
				Name: to.Ptr("test-route"),
				ID:   to.Ptr("/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/httpRoutes/test-route"),
			},
			{
				Name: to.Ptr("test-gateway"),
				ID:   to.Ptr("/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/gateways/test-gateway"),
			},
		}

		appManagementClient.EXPECT().
			ListAllResourcesByApplication(gomock.Any(), "test-app").
			Return(resourceList, nil).
			Times(1)

		diagnosticsClient := clients.NewMockDiagnosticsClient(ctrl)
		diagnosticsClient.EXPECT().
			GetPublicEndpoint(gomock.Any(), clients.EndpointOptions{ResourceID: mustParse(t, "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/containers/test-container")}).
			Return(nil, nil).
			Times(1)

		diagnosticsClient.EXPECT().
			GetPublicEndpoint(gomock.Any(), clients.EndpointOptions{ResourceID: mustParse(t, "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/httpRoutes/test-route")}).
			Return(nil, nil).
			Times(1)

		diagnosticsClient.EXPECT().
			GetPublicEndpoint(gomock.Any(), clients.EndpointOptions{ResourceID: mustParse(t, "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/gateways/test-gateway")}).
			Return(to.Ptr("http://some-url.example.com"), nil).
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
			ConnectionFactory: &connections.MockFactory{
				ApplicationsManagementClient: appManagementClient,
				DiagnosticsClient:            diagnosticsClient,
			},
			Workspace:       workspace,
			Format:          "table",
			Output:          outputSink,
			ApplicationName: "test-app",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		applicationStatus := clients.ApplicationStatus{
			Name:          "test-app",
			ResourceCount: 3,
			Gateways: []clients.GatewayStatus{
				{
					Name:     "test-gateway",
					Endpoint: "http://some-url.example.com",
				},
			},
		}

		expected := []any{
			output.FormattedOutput{
				Format:  "table",
				Obj:     applicationStatus,
				Options: objectformats.GetApplicationStatusTableFormat(),
			},
			output.LogOutput{
				Format: "",
			},
			output.FormattedOutput{
				Format:  "table",
				Obj:     applicationStatus.Gateways,
				Options: objectformats.GetApplicationGatewaysTableFormat(),
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Error: Application Not Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			ShowApplication(gomock.Any(), "test-app").
			Return(v20230415preview.ApplicationResource{}, radcli.Create404Error()).
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
			Format:            "table",
			Output:            outputSink,
			ApplicationName:   "test-app",
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.ErrorIs(t, err, &cli.FriendlyError{Message: "The application \"test-app\" was not found or has been deleted."})

		require.Empty(t, outputSink.Writes)
	})
}

func mustParse(t *testing.T, s string) resources.ID {
	t.Helper()
	id, err := resources.Parse(s)
	require.NoError(t, err)
	return id
}
