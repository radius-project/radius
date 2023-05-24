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

package deploy

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/bicep"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/config"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/deploy"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/to"
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
					Return(v20220315privatepreview.EnvironmentResource{
						Properties: &v20220315privatepreview.EnvironmentProperties{
							Providers: &v20220315privatepreview.Providers{
								Azure: &v20220315privatepreview.ProvidersAzure{
									Scope: to.Ptr("/subscriptions/test-subId/resourceGroups/test-rg"),
								},
							},
						},
					}, nil).
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
			},
		},
		{
			Name:          "rad deploy - fallback workspace",
			Input:         []string{"app.bicep", "--group", "my-group", "--environment", "prod"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvDetails(gomock.Any(), "prod").
					Return(v20220315privatepreview.EnvironmentResource{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad deploy - fallback workspace requires resource group",
			Input:         []string{"app.bicep", "--environment", "prod"},
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
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Environment-scoped deployment with az provider", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bicep := bicep.NewMockInterface(ctrl)
		bicep.EXPECT().
			PrepareTemplate("app.bicep").
			Return(map[string]any{}, nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},

			Name: "kind-kind",
		}
		provider :=
			&clients.Providers{
				Azure: &clients.AzureProvider{
					Scope: "test-scope",
				},
				Radius: &clients.RadiusProvider{
					EnvironmentID: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
				},
			}

		filePath := "app.bicep"
		progressText := fmt.Sprintf(
			"Deploying template '%v' into environment '%v' from workspace '%v'...\n\n"+
				"Deployment In Progress...", filePath, radcli.TestEnvironmentName, workspace.Name)

		options := deploy.Options{
			Workspace:      *workspace,
			Parameters:     map[string]map[string]any{},
			CompletionText: "Deployment Complete",
			ProgressText:   progressText,
			Template:       map[string]any{},
			Providers:      provider,
		}

		deployMock := deploy.NewMockInterface(ctrl)
		deployMock.EXPECT().
			DeployWithProgress(gomock.Any(), options).
			DoAndReturn(func(ctx context.Context, o deploy.Options) (clients.DeploymentResult, error) {
				// Capture options for verification
				options = o
				return clients.DeploymentResult{}, nil
			}).
			Times(1)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			Bicep:           bicep,
			Deploy:          deployMock,
			Output:          outputSink,
			FilePath:        filePath,
			EnvironmentName: radcli.TestEnvironmentName,
			Parameters:      map[string]map[string]any{},
			Workspace:       workspace,
			Providers:       provider,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		// Deployment is scoped to env
		require.Equal(t, "", options.Providers.Radius.ApplicationID)
		require.Equal(t, runner.Providers.Radius.EnvironmentID, options.Providers.Radius.EnvironmentID)

		// All of the output in this command is being done by functions that we mock for testing, so this
		// is always empty.
		require.Empty(t, outputSink.Writes)
	})

	t.Run("Environment-scoped deployment with aws provider", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bicep := bicep.NewMockInterface(ctrl)
		bicep.EXPECT().
			PrepareTemplate("app.bicep").
			Return(map[string]any{}, nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name: "kind-kind",
		}
		ProviderConfig := clients.Providers{
			AWS: &clients.AWSProvider{
				Scope: "test-scope",
			},
			Radius: &clients.RadiusProvider{
				EnvironmentID: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
			},
		}

		filePath := "app.bicep"
		progressText := fmt.Sprintf(
			"Deploying template '%v' into environment '%v' from workspace '%v'...\n\n"+
				"Deployment In Progress...", filePath, radcli.TestEnvironmentName, workspace.Name)

		options := deploy.Options{
			Workspace:      *workspace,
			Parameters:     map[string]map[string]any{},
			CompletionText: "Deployment Complete",
			ProgressText:   progressText,
			Template:       map[string]any{},
			Providers:      &ProviderConfig,
		}

		deployMock := deploy.NewMockInterface(ctrl)
		deployMock.EXPECT().
			DeployWithProgress(gomock.Any(), options).
			DoAndReturn(func(ctx context.Context, o deploy.Options) (clients.DeploymentResult, error) {
				// Capture options for verification
				options = o
				return clients.DeploymentResult{}, nil
			}).
			Times(1)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			Bicep:           bicep,
			Deploy:          deployMock,
			Output:          outputSink,
			Providers:       &ProviderConfig,
			FilePath:        filePath,
			EnvironmentName: radcli.TestEnvironmentName,
			Parameters:      map[string]map[string]any{},
			Workspace:       workspace,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		// Deployment is scoped to env
		require.Equal(t, "", options.Providers.Radius.ApplicationID)
		require.Equal(t, runner.Providers.Radius.EnvironmentID, options.Providers.Radius.EnvironmentID)

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
			Return(map[string]any{}, nil).
			Times(1)

		options := deploy.Options{}

		appManagmentMock := clients.NewMockApplicationsManagementClient(ctrl)
		appManagmentMock.EXPECT().
			CreateApplicationIfNotFound(gomock.Any(), "test-application", gomock.Any()).
			Return(nil).
			Times(1)

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
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name: "kind-kind",
		}
		outputSink := &output.MockOutput{}
		providers := clients.Providers{
			Radius: &clients.RadiusProvider{
				EnvironmentID: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
				ApplicationID: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s/applications/test-application", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
			},
		}

		runner := &Runner{
			Bicep:             bicep,
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagmentMock},
			Deploy:            deployMock,
			Output:            outputSink,
			Providers:         &providers,
			FilePath:          "app.bicep",
			ApplicationName:   "test-application",
			EnvironmentName:   radcli.TestEnvironmentName,
			Parameters:        map[string]map[string]any{},
			Workspace:         workspace,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		// Deployment is scoped to app and env
		require.Equal(t, runner.Providers.Radius.ApplicationID, options.Providers.Radius.ApplicationID)
		require.Equal(t, runner.Providers.Radius.EnvironmentID, options.Providers.Radius.EnvironmentID)

		// All of the output in this command is being done by functions that we mock for testing, so this
		// is always empty.
		require.Empty(t, outputSink.Writes)
	})
}
