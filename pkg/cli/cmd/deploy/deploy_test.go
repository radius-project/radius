// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deploy

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/bicep"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/config"
	"github.com/project-radius/radius/pkg/cli/deploy"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "rad deploy - valid",
			Input:         []string{"app.bicep"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvDetails(gomock.Any(), radcli.TestEnvironmentName).
					Return(v20220315privatepreview.EnvironmentResource{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad deploy - valid with parameters",
			Input:         []string{"app.bicep", "-p", "foo=bar", "--parameters", "a=b"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvDetails(gomock.Any(), radcli.TestEnvironmentName).
					Return(v20220315privatepreview.EnvironmentResource{}, nil).
					Times(1)

			},
		},
		{
			Name:          "rad deploy - valid with environment",
			Input:         []string{"app.bicep", "-e", "prod"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvDetails(gomock.Any(), "prod").
					Return(v20220315privatepreview.EnvironmentResource{}, nil).
					Times(1)

			},
		},
		{
			Name:          "rad deploy - env does not exist invalid",
			Input:         []string{"app.bicep", "-e", "prod"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvDetails(gomock.Any(), "prod").
					Return(v20220315privatepreview.EnvironmentResource{}, radcli.Create404Error()).
					Times(1)

			},
		},
		{
			Name:          "rad deploy - valid with app and env",
			Input:         []string{"app.bicep", "-e", "prod", "-a", "my-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvDetails(gomock.Any(), "prod").
					Return(v20220315privatepreview.EnvironmentResource{}, nil).
					Times(1)
				mocks.ApplicationManagementClient.EXPECT().
					ShowApplication(gomock.Any(), "my-app").
					Return(v20220315privatepreview.ApplicationResource{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad deploy - app does not exist invalid",
			Input:         []string{"app.bicep", "-e", "prod", "-a", "my-app"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvDetails(gomock.Any(), "prod").
					Return(v20220315privatepreview.EnvironmentResource{}, nil).
					Times(1)
				mocks.ApplicationManagementClient.EXPECT().
					ShowApplication(gomock.Any(), "my-app").
					Return(v20220315privatepreview.ApplicationResource{}, radcli.Create404Error()).
					Times(1)
			},
		},
		{
			Name:          "rad deploy - app set by directory config",
			Input:         []string{"app.bicep", "-e", "prod"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
				DirectoryConfig: &config.DirectoryConfig{
					Workspace: config.DirectoryWorkspaceConfig{
						Application: "my-app",
					},
				},
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvDetails(gomock.Any(), "prod").
					Return(v20220315privatepreview.EnvironmentResource{}, nil).
					Times(1)
				mocks.ApplicationManagementClient.EXPECT().
					ShowApplication(gomock.Any(), "my-app").
					Return(v20220315privatepreview.ApplicationResource{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad deploy - fallback workspace invalid",
			Input:         []string{"app.bicep"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "rad deploy - too many args",
			Input:         []string{"app.bicep", "anotherfile.json"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Create Command with too many args",
			Input:         []string{"a", "b"},
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
	t.Run("Environment-scoped deployment", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bicep := bicep.NewMockInterface(ctrl)
		bicep.EXPECT().
			PrepareTemplate("app.bicep").
			Return(map[string]interface{}{}, nil).
			Times(1)

		options := deploy.Options{}

		deployMock := deploy.NewMockInterface(ctrl)
		deployMock.EXPECT().
			DeployWithProgress(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, o deploy.Options) (clients.DeploymentResult, error) {
				// Capture options for verification
				options = o
				return clients.DeploymentResult{}, nil
			}).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]interface{}{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name: "kind-kind",
		}
		outputSink := &output.MockOutput{}
		runner := &Runner{
			Bicep:  bicep,
			Deploy: deployMock,
			Output: outputSink,

			FilePath:        "app.bicep",
			EnvironmentID:   fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
			EnvironmentName: radcli.TestEnvironmentName,
			Parameters:      map[string]map[string]interface{}{},
			Workspace:       workspace,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		// Deployment is scoped to env
		require.Equal(t, "", options.ApplicationID)
		require.Equal(t, runner.EnvironmentID, options.EnvironmentID)

		// All of the output in this command is being done by functions that we mock for testing, so this
		// is always empty.
		require.Empty(t, outputSink.Writes)
	})

	t.Run("Application-scoped deployment", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bicep := bicep.NewMockInterface(ctrl)
		bicep.EXPECT().
			PrepareTemplate("app.bicep").
			Return(map[string]interface{}{}, nil).
			Times(1)

		options := deploy.Options{}

		deployMock := deploy.NewMockInterface(ctrl)
		deployMock.EXPECT().
			DeployWithProgress(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, o deploy.Options) (clients.DeploymentResult, error) {
				// Capture options for verification
				options = o
				return clients.DeploymentResult{}, nil
			}).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]interface{}{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name: "kind-kind",
		}
		outputSink := &output.MockOutput{}
		runner := &Runner{
			Bicep:  bicep,
			Deploy: deployMock,
			Output: outputSink,

			FilePath:        "app.bicep",
			ApplicationID:   fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/applications/%s", radcli.TestEnvironmentName, "test-application"),
			ApplicationName: "test-application",
			EnvironmentID:   fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
			EnvironmentName: radcli.TestEnvironmentName,
			Parameters:      map[string]map[string]interface{}{},
			Workspace:       workspace,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		// Deployment is scoped to app and env
		require.Equal(t, runner.ApplicationID, options.ApplicationID)
		require.Equal(t, runner.EnvironmentID, options.EnvironmentID)

		// All of the output in this command is being done by functions that we mock for testing, so this
		// is always empty.
		require.Empty(t, outputSink.Writes)
	})
}
