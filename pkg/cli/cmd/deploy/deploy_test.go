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

	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/config"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/deploy"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	corerpfake "github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/radcli"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(map[string]any{}, nil).
					Times(1)
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/test-environment").
					Return(v20231001preview.EnvironmentResource{
						ID: to.Ptr("/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/test-environment"),
					}, nil).
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
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(map[string]any{}, nil).
					Times(1)
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), radcli.TestEnvironmentID).
					Return(v20231001preview.EnvironmentResource{
						ID: to.Ptr(radcli.TestEnvironmentID),
					}, nil).
					Times(1)

			},
		},
		{
			Name:          "rad deploy - app set by directory config",
			Input:         []string{"app.bicep", "-e", "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/prod"},
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
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(map[string]any{}, nil).
					Times(1)
				// Since environment id indicates a applications core environment, only that will be fetched
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/prod").
					Return(v20231001preview.EnvironmentResource{
						ID: to.Ptr("/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/prod"),
					}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad deploy - fallback workspace",
			Input:         []string{"app.bicep", "--group", "my-group", "--environment", "/planes/radius/local/resourceGroups/my-group/providers/Applications.Core/environments/prod"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(map[string]any{}, nil).
					Times(1)
				// Since environment id indicates a applications core environment, only that will be fetched
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/my-group/providers/Applications.Core/environments/prod").
					Return(v20231001preview.EnvironmentResource{
						ID: to.Ptr("/planes/radius/local/resourceGroups/my-group/providers/Applications.Core/environments/prod"),
					}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad deploy - valid with app and env",
			Input:         []string{"app.bicep", "-e", "/planes/radius/local/resourceGroups/test-resource-group/providers/applications.core/environments/prod", "-a", "my-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(map[string]any{}, nil).
					Times(1)
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/applications.core/environments/prod").
					Return(v20231001preview.EnvironmentResource{
						ID: to.Ptr("/planes/radius/local/resourceGroups/test-resource-group/providers/applications.core/environments/prod"),
					}, nil).
					Times(1)
			},
			ValidateCallback: func(t *testing.T, obj framework.Runner) {
				runner := obj.(*Runner)
				scope := "/planes/radius/local/resourceGroups/test-resource-group"
				environmentID := scope + "/providers/applications.core/environments/prod"
				applicationID := scope + "/providers/applications.core/applications/my-app"
				require.Equal(t, scope, runner.Workspace.Scope)
				require.Equal(t, environmentID, runner.EnvironmentNameOrID)
				require.Equal(t, clients.RadiusProvider{ApplicationID: applicationID, EnvironmentID: environmentID}, *runner.Providers.Radius)
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
		{
			Name:          "rad deploy - no env in config, no env flag, no env in template invalid",
			Input:         []string{"app.bicep", "--group", "test-group"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(map[string]any{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad deploy - no env in config, env flag provided, no env in template valid",
			Input:         []string{"app.bicep", "-e", "/planes/radius/local/resourceGroups/test-resource-group/providers/applications.core/environments/prod", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(map[string]any{}, nil).
					Times(1)
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/applications.core/environments/prod").
					Return(v20231001preview.EnvironmentResource{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad deploy - no env in config, no env flag, env in template valid",
			Input:         []string{"app.bicep", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				templateWithEnv := map[string]any{
					"resources": map[string]any{
						"env": map[string]any{
							"type": "Radius.Core/environments@2023-10-01-preview",
						},
					},
				}
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(templateWithEnv, nil).
					Times(1)
			},
		},
		{
			Name:          "rad deploy - no env in config, env flag provided, env in template valid",
			Input:         []string{"app.bicep", "-e", "/planes/radius/local/resourceGroups/test-resource-group/providers/applications.core/environments/prod", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				templateWithEnv := map[string]any{
					"resources": map[string]any{
						"env": map[string]any{
							"type": "Applications.Core/environments@2023-10-01-preview",
						},
					},
				}
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(templateWithEnv, nil).
					Times(1)
				// When env flag is explicitly provided, we honor it and validate even if template creates environment
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/applications.core/environments/prod").
					Return(v20231001preview.EnvironmentResource{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad deploy - missing env and app succeeds with env in config",
			Input:         []string{"app.bicep", "--group", "new-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				templateWithEnv := map[string]any{
					"resources": map[string]any{
						"env": map[string]any{
							"type": "Applications.Core/environments@2023-10-01-preview",
						},
					},
				}
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(templateWithEnv, nil).
					Times(1)
				// Since workspace has default environment (full ID), we validate it even though template creates one
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/test-environment").
					Return(v20231001preview.EnvironmentResource{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad deploy fails - env required when not explicitly specified and template doesn't create env",
			Input:         []string{"app.bicep", "--group", "new-group"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(map[string]any{}, nil).
					Times(1)
			},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_ValidateRadiusCoreEnvProvider(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	// Test case: rad deploy with environment that returns 404 from Radius.Core
	t.Run("rad deploy - application.core env found, radius.core env not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create a fake env server that returns 404
		nonExistentEnvServer := corerpfake.EnvironmentsServer{
			Get: func(
				_ context.Context,
				_ string,
				_ *v20250801preview.EnvironmentsClientGetOptions,
			) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
				errResp.SetError(fmt.Errorf("Environment not found"))
				errResp.SetResponseError(404, "Not Found")
				return
			},
		}

		workspace := &workspaces.Workspace{
			Name:  "test-workspace",
			Scope: "/planes/radius/local/resourceGroups/test-resource-group",
		}

		// Create test client factory with fake env server factory function
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			workspace.Scope,
			func() corerpfake.EnvironmentsServer {
				return nonExistentEnvServer
			},
			nil,
		)
		require.NoError(t, err)

		// Set up Applications.Core mock to return successful environment
		// Since we're using `-e prod`, it should call GetEnvironment with the constructed Applications.Core ID for "prod"
		mockAppClient := clients.NewMockApplicationsManagementClient(ctrl)
		mockAppClient.EXPECT().
			GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/prod").
			Return(v20231001preview.EnvironmentResource{
				ID: to.Ptr("/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/prod"),
				Properties: &v20231001preview.EnvironmentProperties{
					Providers: &v20231001preview.Providers{
						Azure: &v20231001preview.ProvidersAzure{
							Scope: to.Ptr("/subscriptions/test-subId/resourceGroups/test-rg"),
						},
					},
				},
			}, nil).
			Times(1)

		mockBicep := bicep.NewMockInterface(ctrl)
		mockBicep.EXPECT().
			PrepareTemplate("app.bicep").
			Return(map[string]any{}, nil).
			Times(1)

		f := &framework.Impl{
			ConfigHolder: &framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			Output: &output.MockOutput{},
			Bicep:  mockBicep,
		}

		cmd, runner := NewCommand(f)
		r := runner.(*Runner)
		r.Workspace = workspace
		r.RadiusCoreClientFactory = factory
		r.ConnectionFactory = &connections.MockFactory{ApplicationsManagementClient: mockAppClient}

		// Parse the flags manually to set the environment flag
		cmd.SetArgs([]string{"app.bicep", "-e", "prod"})
		cmd.SetContext(context.Background())
		err = cmd.ParseFlags([]string{"-e", "prod"})
		require.NoError(t, err)

		// Now validate with the parsed arguments
		err = r.Validate(cmd, []string{"app.bicep"})
		require.NoError(t, err, "Deploy should succeed when environment returns 404 from Radius.Core but exists in Applications.Core")
	})

	t.Run("rad deploy - env specified with -e returns 404 from both Radius.Core and Applications.Core should fail", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create a fake env server that returns 404
		nonExistentEnvServer := corerpfake.EnvironmentsServer{
			Get: func(
				_ context.Context,
				_ string,
				_ *v20250801preview.EnvironmentsClientGetOptions,
			) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
				errResp.SetError(fmt.Errorf("Environment not found"))
				errResp.SetResponseError(404, "Not Found")
				return
			},
		}

		workspace := &workspaces.Workspace{
			Name:  "test-workspace",
			Scope: "/planes/radius/local/resourceGroups/test-resource-group",
		}

		// Create test client factory with fake env server
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			workspace.Scope,
			func() corerpfake.EnvironmentsServer {
				return nonExistentEnvServer
			},
			nil,
		)
		require.NoError(t, err)

		// Set up Applications.Core mock to also return 404
		mockAppClient := clients.NewMockApplicationsManagementClient(ctrl)
		mockAppClient.EXPECT().
			GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/nonexistent").
			Return(v20231001preview.EnvironmentResource{}, radcli.Create404Error()).
			Times(1)

		mockBicep := bicep.NewMockInterface(ctrl)
		mockBicep.EXPECT().
			PrepareTemplate("app.bicep").
			Return(map[string]any{}, nil).
			Times(1)

		f := &framework.Impl{
			ConfigHolder: &framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			Output: &output.MockOutput{},
			Bicep:  mockBicep,
		}

		cmd, runner := NewCommand(f)
		r := runner.(*Runner)
		r.Workspace = workspace
		r.RadiusCoreClientFactory = factory
		r.ConnectionFactory = &connections.MockFactory{ApplicationsManagementClient: mockAppClient}

		// Parse the flags manually to set the environment flag with a non-existent environment
		cmd.SetArgs([]string{"app.bicep", "-e", "nonexistent"})
		cmd.SetContext(context.Background())
		err = cmd.ParseFlags([]string{"-e", "nonexistent"})
		require.NoError(t, err)

		// This should fail because both providers return 404 and user specified environment name
		err = r.Validate(cmd, []string{"app.bicep"})
		require.Error(t, err, "Deploy should fail when both Radius.Core and Applications.Core return 404 for specified environment")
		require.Contains(t, err.Error(), "The environment \"nonexistent\" does not exist in scope", "Error should indicate environment doesn't exist")
		require.Contains(t, err.Error(), "Run `rad env create` first", "Error should suggest creating environment")
	})

	t.Run("rad deploy - env specified with -e returns conflict when both Radius.Core and Applications.Core environments exist", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create a fake env server that returns an environment (not 404)
		existingRadiusCoreEnvServer := corerpfake.EnvironmentsServer{
			Get: func(
				_ context.Context,
				_ string,
				_ *v20250801preview.EnvironmentsClientGetOptions,
			) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
				resp.SetResponse(200, v20250801preview.EnvironmentsClientGetResponse{
					EnvironmentResource: v20250801preview.EnvironmentResource{
						ID: to.Ptr("/planes/radius/local/resourceGroups/test-resource-group/providers/Radius.Core/environments/conflictenv"),
						Properties: &v20250801preview.EnvironmentProperties{
							Providers: &v20250801preview.Providers{
								Azure: &v20250801preview.ProvidersAzure{
									SubscriptionID:    to.Ptr("test-sub-id"),
									ResourceGroupName: to.Ptr("test-rg-name"),
								},
							},
						},
					},
				}, nil)
				return
			},
		}

		workspace := &workspaces.Workspace{
			Name:  "test-workspace",
			Scope: "/planes/radius/local/resourceGroups/test-resource-group",
		}

		// Create test client factory with fake env server that returns existing environment
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			workspace.Scope,
			func() corerpfake.EnvironmentsServer {
				return existingRadiusCoreEnvServer
			},
			nil,
		)
		require.NoError(t, err)

		// Set up Applications.Core mock to also return successful environment
		mockAppClient := clients.NewMockApplicationsManagementClient(ctrl)
		mockAppClient.EXPECT().
			GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/conflictenv").
			Return(v20231001preview.EnvironmentResource{
				ID: to.Ptr("/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/conflictenv"),
				Properties: &v20231001preview.EnvironmentProperties{
					Providers: &v20231001preview.Providers{
						Azure: &v20231001preview.ProvidersAzure{
							Scope: to.Ptr("/subscriptions/test-subId/resourceGroups/test-rg"),
						},
					},
				},
			}, nil).
			Times(1)

		mockBicep := bicep.NewMockInterface(ctrl)
		mockBicep.EXPECT().
			PrepareTemplate("app.bicep").
			Return(map[string]any{}, nil).
			Times(1)

		f := &framework.Impl{
			ConfigHolder: &framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			Output: &output.MockOutput{},
			Bicep:  mockBicep,
		}

		cmd, runner := NewCommand(f)
		r := runner.(*Runner)
		r.Workspace = workspace
		r.RadiusCoreClientFactory = factory
		r.ConnectionFactory = &connections.MockFactory{ApplicationsManagementClient: mockAppClient}

		// Parse the flags manually to set the environment flag with a conflicting environment name
		cmd.SetArgs([]string{"app.bicep", "-e", "conflictenv"})
		cmd.SetContext(context.Background())
		err = cmd.ParseFlags([]string{"-e", "conflictenv"})
		require.NoError(t, err)

		// This should fail because both providers return valid environments - conflict scenario
		err = r.Validate(cmd, []string{"app.bicep"})
		require.Error(t, err, "Deploy should fail when both Radius.Core and Applications.Core have environments with the same name")
		require.Contains(t, err.Error(), "Conflict detected: Environment 'conflictenv' exists in both Applications.Core and Radius.Core providers", "Error should indicate environment name conflict")
		require.Contains(t, err.Error(), "Please specify the full resource ID to disambiguate", "Error should suggest using full resource ID")
	})

}

func Test_Run(t *testing.T) {
	t.Run("Environment-scoped deployment with az provider", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bicep := bicep.NewMockInterface(ctrl)

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
				"Deployment In Progress...", filePath, radcli.TestEnvironmentID, workspace.Name)

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
			Bicep:               bicep,
			Deploy:              deployMock,
			Output:              outputSink,
			FilePath:            filePath,
			EnvironmentNameOrID: radcli.TestEnvironmentID,
			Parameters:          map[string]map[string]any{},
			Workspace:           workspace,
			Providers:           provider,
			Template:            map[string]any{},
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

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name:        "kind-kind",
			Environment: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
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
				"Deployment In Progress...", filePath, radcli.TestEnvironmentID, workspace.Name)

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
			Bicep:               bicep,
			Deploy:              deployMock,
			Output:              outputSink,
			Providers:           &ProviderConfig,
			FilePath:            filePath,
			EnvironmentNameOrID: radcli.TestEnvironmentID,
			Parameters:          map[string]map[string]any{},
			Workspace:           workspace,
			Template:            map[string]any{},
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
			Name:        "kind-kind",
			Environment: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
		}
		outputSink := &output.MockOutput{}
		providers := clients.Providers{
			Radius: &clients.RadiusProvider{
				EnvironmentID: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
				ApplicationID: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s/applications/test-application", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
			},
		}

		runner := &Runner{
			Bicep:               bicep,
			ConnectionFactory:   &connections.MockFactory{ApplicationsManagementClient: appManagmentMock},
			Deploy:              deployMock,
			Output:              outputSink,
			Providers:           &providers,
			FilePath:            "app.bicep",
			ApplicationName:     "test-application",
			EnvironmentNameOrID: radcli.TestEnvironmentName,
			Parameters:          map[string]map[string]any{},
			Workspace:           workspace,
			Template:            map[string]any{},
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

	t.Run("Deployment that doesn't need an app or env", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bicep := bicep.NewMockInterface(ctrl)

		appManagmentMock := clients.NewMockApplicationsManagementClient(ctrl)
		appManagmentMock.EXPECT().
			CreateApplicationIfNotFound(gomock.Any(), "appdoesntexist", gomock.Any()).
			Return(nil).
			Times(1)

		deployMock := deploy.NewMockInterface(ctrl)
		deployMock.EXPECT().
			DeployWithProgress(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, o deploy.Options) (clients.DeploymentResult, error) {
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
				ApplicationID: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/applications/test-application", radcli.TestEnvironmentName),
			},
		}

		runner := &Runner{
			Bicep:               bicep,
			ConnectionFactory:   &connections.MockFactory{ApplicationsManagementClient: appManagmentMock},
			Deploy:              deployMock,
			Output:              outputSink,
			Providers:           &providers,
			FilePath:            "app.bicep",
			ApplicationName:     "appdoesntexist",
			EnvironmentNameOrID: "envdoesntexist",
			Parameters:          map[string]map[string]any{},
			Workspace:           workspace,
			EnvResult:           nil,
			Template:            map[string]any{},
		}

		err := runner.Run(context.Background())

		// Even though GetEnvironment returns a 404 error (indicated by EnvCheckResult being nil), the deployment should still succeed
		require.NoError(t, err)

		// All of the output in this command is being done by functions that we mock for testing, so this
		// is always empty.
		require.Empty(t, outputSink.Writes)
	})

	t.Run("Deployment with missing parameters", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bicep := bicep.NewMockInterface(ctrl)

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
			},
		}

		runner := &Runner{
			Bicep:               bicep,
			ConnectionFactory:   &connections.MockFactory{},
			Output:              outputSink,
			Providers:           &providers,
			EnvironmentNameOrID: radcli.TestEnvironmentName,
			FilePath:            "app.bicep",
			Parameters:          map[string]map[string]any{},
			Workspace:           workspace,
			Template: map[string]any{
				"parameters": map[string]any{
					"application": map[string]any{},
					"environment": map[string]any{},
					"location":    map[string]any{},
					"size":        map[string]any{"defaultValue": "BIG!"},
				},
			},
		}

		err := runner.Run(context.Background())

		// Even though GetEnvironment returns a 404 error, the deployment should still succeed
		require.Error(t, err)

		expected := `The template "app.bicep" could not be deployed because of the following errors:

  - The template requires an application. Use --application to specify the application name.
  - The template requires a parameter "location". Use --parameters location=<value> to specify the value.`
		require.Equal(t, expected, err.Error())

		// All of the output in this command is being done by functions that we mock for testing, so this
		// is always empty.
		require.Empty(t, outputSink.Writes)
	})
}

func Test_injectAutomaticParameters(t *testing.T) {
	template := map[string]any{
		"parameters": map[string]any{
			"environment": map[string]any{},
			"application": map[string]any{},
		},
	}

	runner := Runner{
		Parameters: map[string]map[string]any{
			"a": {
				"value": "YO",
			},
		},
		Providers: &clients.Providers{
			Radius: &clients.RadiusProvider{
				ApplicationID: "test-app",
				EnvironmentID: "test-env",
			},
		},
	}
	err := runner.injectAutomaticParameters(template)
	require.NoError(t, err)

	expected := map[string]map[string]any{
		"environment": {
			"value": "test-env",
		},
		"application": {
			"value": "test-app",
		},
		"a": {
			"value": "YO",
		},
	}

	require.Equal(t, expected, runner.Parameters)
}

func Test_reportMissingParameters(t *testing.T) {
	template := map[string]any{
		"parameters": map[string]any{
			"a":                         map[string]any{},
			"b":                         map[string]any{},
			"parameterWithDefaultValue": map[string]any{"defaultValue": "!"},
		},
	}

	t.Run("Missing parameters", func(t *testing.T) {
		runner := Runner{
			FilePath: "app.bicep",
			Parameters: map[string]map[string]any{
				"b": {
					"value": "YO",
				},
			},
		}
		err := runner.reportMissingParameters(template)

		expected := `The template "app.bicep" could not be deployed because of the following errors:

  - The template requires a parameter "a". Use --parameters a=<value> to specify the value.`
		require.Equal(t, expected, err.Error())
	})

	t.Run("All parameters provided", func(t *testing.T) {
		runner := Runner{
			FilePath: "app.bicep",
			Parameters: map[string]map[string]any{
				"a": {
					"value": "YO",
				},
				"b": {
					"value": "YO",
				},
			},
		}
		err := runner.reportMissingParameters(template)
		require.NoError(t, err)
	})

	t.Run("All parameters provided (ignoring case)", func(t *testing.T) {
		runner := Runner{
			FilePath: "app.bicep",
			Parameters: map[string]map[string]any{
				"A": {
					"value": "YO",
				},
				"B": {
					"value": "YO",
				},
				"parameterWithDEfaultValue": {
					"value": "YO",
				},
			},
		}
		err := runner.reportMissingParameters(template)
		require.NoError(t, err)
	})
}

func Test_setupEnvironmentID(t *testing.T) {
	testcases := []struct {
		name                  string
		envID                 *string
		expectedEnvironmentID string
		expectedWorkspaceEnv  string
	}{
		{
			name:                  "Valid environment ID",
			envID:                 to.Ptr("/planes/radius/local/resourceGroups/test/providers/applications.core/environments/env1"),
			expectedEnvironmentID: "/planes/radius/local/resourceGroups/test/providers/applications.core/environments/env1",
			expectedWorkspaceEnv:  "/planes/radius/local/resourceGroups/test/providers/applications.core/environments/env1",
		},
		{
			name:                  "Nil environment ID",
			envID:                 nil,
			expectedEnvironmentID: "",
			expectedWorkspaceEnv:  "",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &Runner{
				Providers: &clients.Providers{
					Radius: &clients.RadiusProvider{},
				},
				Workspace: &workspaces.Workspace{},
			}

			runner.setupEnvironmentID(tc.envID)

			require.Equal(t, tc.expectedEnvironmentID, runner.Providers.Radius.EnvironmentID)
			require.Equal(t, tc.expectedWorkspaceEnv, runner.Workspace.Environment)
		})
	}
}

func Test_setupCloudProviders(t *testing.T) {
	testcases := []struct {
		name          string
		properties    interface{}
		expectedAWS   *clients.AWSProvider
		expectedAzure *clients.AzureProvider
	}{
		{
			name:          "Nil properties",
			properties:    nil,
			expectedAWS:   nil,
			expectedAzure: nil,
		},
		{
			name: "v20231001preview with both providers",
			properties: &v20231001preview.EnvironmentProperties{
				Providers: &v20231001preview.Providers{
					Aws: &v20231001preview.ProvidersAws{
						Scope: to.Ptr("test-aws-scope"),
					},
					Azure: &v20231001preview.ProvidersAzure{
						Scope: to.Ptr("test-azure-scope"),
					},
				},
			},
			expectedAWS: &clients.AWSProvider{
				Scope: "test-aws-scope",
			},
			expectedAzure: &clients.AzureProvider{
				Scope: "test-azure-scope",
			},
		},
		{
			name: "v20250801preview with both providers",
			properties: &v20250801preview.EnvironmentProperties{
				Providers: &v20250801preview.Providers{
					Aws: &v20250801preview.ProvidersAws{
						Scope: to.Ptr("/planes/aws/aws/account/account-id/regions/us-west-2"),
					},
					Azure: &v20250801preview.ProvidersAzure{
						SubscriptionID:    to.Ptr("test-subscription"),
						ResourceGroupName: to.Ptr("test-rg"),
					},
				},
			},
			expectedAWS: &clients.AWSProvider{
				Scope: "/planes/aws/aws/account/account-id/regions/us-west-2",
			},
			expectedAzure: &clients.AzureProvider{
				Scope: "/planes/azure/azure/Subscriptions/test-subscription/ResourceGroups/test-rg",
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &Runner{
				Providers: &clients.Providers{},
			}

			runner.setupCloudProviders(tc.properties)

			require.Equal(t, tc.expectedAWS, runner.Providers.AWS)
			require.Equal(t, tc.expectedAzure, runner.Providers.Azure)
		})
	}
}

func Test_handleEnvironmentError(t *testing.T) {
	testcases := []struct {
		name          string
		err           error
		command       *cobra.Command
		args          []string
		expectedError string
		shouldError   bool
	}{
		{
			name:          "Non-404 error",
			err:           fmt.Errorf("some other error"),
			command:       &cobra.Command{},
			args:          []string{},
			expectedError: "some other error",
			shouldError:   true,
		},
		{
			name:        "404 error with no environment specified",
			err:         radcli.Create404Error(),
			command:     &cobra.Command{},
			args:        []string{"template.bicep"},
			shouldError: false,
		},
		{
			name:          "404 error with environment specified via flag",
			err:           radcli.Create404Error(),
			command:       createCommandWithEnvironmentFlag("myenv"),
			args:          []string{"template.bicep"},
			expectedError: "The environment \"myenv\" does not exist in scope \"/planes/radius/local/resourceGroups/test-resource-group\". Run `rad env create` first. You could also provide the environment ID if the environment exists in a different group.",
			shouldError:   true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			envNameOrID := "myenv"
			if tc.name == "404 error with no environment specified" {
				envNameOrID = ""
			}
			runner := &Runner{
				EnvironmentNameOrID: envNameOrID,
				Workspace: &workspaces.Workspace{
					Scope: "/planes/radius/local/resourceGroups/test-resource-group",
				},
			}

			err := runner.handleEnvironmentError(tc.err, tc.command, tc.args)

			if tc.shouldError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_getApplicationsCoreEnvironment(t *testing.T) {
	testcases := []struct {
		name          string
		setupMocks    func(client *clients.MockApplicationsManagementClient)
		command       *cobra.Command
		args          []string
		expectedEnv   *v20231001preview.EnvironmentResource
		expectedError string
		shouldError   bool
	}{
		{
			name: "Successfully get environment",
			setupMocks: func(client *clients.MockApplicationsManagementClient) {
				env := v20231001preview.EnvironmentResource{
					ID: to.Ptr("/planes/radius/local/resourceGroups/test/providers/Applications.Core/environments/env1"),
					Properties: &v20231001preview.EnvironmentProperties{
						Providers: &v20231001preview.Providers{
							Azure: &v20231001preview.ProvidersAzure{
								Scope: to.Ptr("test-scope"),
							},
						},
					},
				}
				client.EXPECT().GetEnvironment(gomock.Any(), "myenv").Return(env, nil).Times(1)
			},
			command: &cobra.Command{},
			args:    []string{"template.bicep"},
			expectedEnv: &v20231001preview.EnvironmentResource{
				ID: to.Ptr("/planes/radius/local/resourceGroups/test/providers/Applications.Core/environments/env1"),
				Properties: &v20231001preview.EnvironmentProperties{
					Providers: &v20231001preview.Providers{
						Azure: &v20231001preview.ProvidersAzure{
							Scope: to.Ptr("test-scope"),
						},
					},
				},
			},
			shouldError: false,
		},
		{
			name: "Environment not found - returns error",
			setupMocks: func(client *clients.MockApplicationsManagementClient) {
				client.EXPECT().GetEnvironment(gomock.Any(), "myenv").Return(v20231001preview.EnvironmentResource{}, radcli.Create404Error()).Times(1)
			},
			command:       &cobra.Command{},
			args:          []string{"template.bicep"},
			expectedEnv:   nil,
			expectedError: "NotFound",
			shouldError:   true,
		},
		{
			name: "Environment not found - environment specified via flag (error)",
			setupMocks: func(client *clients.MockApplicationsManagementClient) {
				client.EXPECT().GetEnvironment(gomock.Any(), "myenv").Return(v20231001preview.EnvironmentResource{}, radcli.Create404Error()).Times(1)
			},
			command:       createCommandWithEnvironmentFlag("myenv"),
			args:          []string{"template.bicep"},
			expectedEnv:   nil,
			expectedError: "NotFound",
			shouldError:   true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := clients.NewMockApplicationsManagementClient(ctrl)
			tc.setupMocks(mockClient)

			runner := &Runner{
				EnvironmentNameOrID: "myenv",
				Workspace: &workspaces.Workspace{
					Scope: "/planes/radius/local/resourceGroups/test-resource-group",
				},
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: mockClient},
			}

			env, err := runner.getApplicationsCoreEnvironment(context.Background(), "myenv")

			if tc.shouldError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
				require.Nil(t, env)
			} else {
				require.NoError(t, err)
				if tc.expectedEnv == nil {
					require.Nil(t, env)
				} else {
					require.NotNil(t, env)
					require.Equal(t, tc.expectedEnv.ID, env.ID)
					require.Equal(t, tc.expectedEnv.Properties, env.Properties)
				}
			}
		})
	}
}

func Test_getRadiusCoreEnvironment(t *testing.T) {
	testcases := []struct {
		name            string
		environmentName string
		command         *cobra.Command
		args            []string
		expectedEnv     *v20250801preview.EnvironmentResource
		expectedError   string
		shouldError     bool
	}{
		{
			name:            "Successfully get environment by name",
			environmentName: "myenv",
			command:         &cobra.Command{},
			args:            []string{"template.bicep"},
			expectedEnv: &v20250801preview.EnvironmentResource{
				Name: to.Ptr("myenv"),
			},
			shouldError: false,
		},
		{
			name:            "Successfully get environment by resource ID",
			environmentName: "/planes/radius/local/resourceGroups/test/providers/Radius.Core/environments/myenv",
			command:         &cobra.Command{},
			args:            []string{"template.bicep"},
			expectedEnv: &v20250801preview.EnvironmentResource{
				Name: to.Ptr("/planes/radius/local/resourceGroups/test/providers/Radius.Core/environments/myenv"),
			},
			shouldError: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			scope := "/planes/radius/local/resourceGroups/test-resource-group"
			factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, test_client_factory.WithEnvironmentServerNoError, nil)
			require.NoError(t, err)

			runner := &Runner{
				EnvironmentNameOrID:     tc.environmentName,
				RadiusCoreClientFactory: factory,
				Workspace: &workspaces.Workspace{
					Scope: scope,
				},
			}

			env, err := runner.getRadiusCoreEnvironment(context.Background(), tc.environmentName)

			if tc.shouldError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
				require.Nil(t, env)
			} else {
				require.NoError(t, err)
				if tc.expectedEnv == nil {
					require.Nil(t, env)
				} else {
					require.NotNil(t, env)
					require.Equal(t, *tc.expectedEnv.Name, *env.Name)
				}
			}
		})
	}
}

func Test_constructApplicationsCoreEnvironmentID(t *testing.T) {
	runner := &Runner{
		Workspace: &workspaces.Workspace{
			Scope: "/planes/radius/local/resourceGroups/test-rg",
		},
	}

	result := runner.constructApplicationsCoreEnvironmentID("myenv")
	expected := "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/myenv"
	require.Equal(t, expected, result)
}

func Test_constructRadiusCoreEnvironmentID(t *testing.T) {
	runner := &Runner{
		Workspace: &workspaces.Workspace{
			Scope: "/planes/radius/local/resourceGroups/test-rg",
		},
	}

	result := runner.constructRadiusCoreEnvironmentID("myenv")
	expected := "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/environments/myenv"
	require.Equal(t, expected, result)
}

// Helper function to create a command with environment flag set
func createCommandWithEnvironmentFlag(envName string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("environment", "", "Environment name")
	_ = cmd.Flags().Set("environment", envName)
	return cmd
}

func Test_FetchEnvironment(t *testing.T) {
	testcases := []struct {
		name                    string
		envNameOrID             string
		setupApplicationsCore   func(*clients.MockApplicationsManagementClient)
		setupRadiusCoreFactory  bool
		expectedUseApplications bool
		expectedEnvironmentID   string
		expectedError           string
		shouldError             bool
		shouldReturnNil         bool
	}{
		{
			name:        "Fetch Applications.Core environment by full ID",
			envNameOrID: "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/myenv",
			setupApplicationsCore: func(client *clients.MockApplicationsManagementClient) {
				env := v20231001preview.EnvironmentResource{
					ID: to.Ptr("/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/myenv"),
					Properties: &v20231001preview.EnvironmentProperties{
						Providers: &v20231001preview.Providers{
							Azure: &v20231001preview.ProvidersAzure{
								Scope: to.Ptr("/subscriptions/test-sub/resourceGroups/test-rg"),
							},
						},
					},
				}
				client.EXPECT().GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/myenv").Return(env, nil).Times(1)
			},
			setupRadiusCoreFactory:  false,
			expectedUseApplications: true,
			expectedEnvironmentID:   "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/myenv",
			shouldError:             false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			scope := "/planes/radius/local/resourceGroups/test-rg"

			mockClient := clients.NewMockApplicationsManagementClient(ctrl)
			tc.setupApplicationsCore(mockClient)

			runner := &Runner{
				EnvironmentNameOrID: tc.envNameOrID,
				Workspace: &workspaces.Workspace{
					Connection: map[string]any{
						"kind":    "kubernetes",
						"context": "kind-kind",
					},
					Scope: scope,
				},
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: mockClient},
			}

			if tc.setupRadiusCoreFactory {
				factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, test_client_factory.WithEnvironmentServerNoError, nil)
				require.NoError(t, err)
				runner.RadiusCoreClientFactory = factory
			}

			result, err := runner.FetchEnvironment(context.Background(), tc.envNameOrID)

			if tc.shouldError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
				require.Nil(t, result)
			} else if tc.shouldReturnNil {
				require.NoError(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tc.expectedUseApplications, result.UseApplicationsCore)
				require.Equal(t, tc.expectedEnvironmentID, runner.EnvironmentNameOrID)
			}
		})
	}
}

func Test_ConfigureProviders(t *testing.T) {
	testcases := []struct {
		name                  string
		envResult             *EnvironmentCheckResult
		applicationName       string
		expectedEnvironmentID string
		expectedApplicationID string
		expectedAzureScope    string
		expectedAWSScope      string
	}{
		{
			name: "Configure with Applications.Core environment",
			envResult: &EnvironmentCheckResult{
				UseApplicationsCore: true,
				ApplicationsCoreEnv: &v20231001preview.EnvironmentResource{
					ID: to.Ptr("/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/myenv"),
					Properties: &v20231001preview.EnvironmentProperties{
						Providers: &v20231001preview.Providers{
							Azure: &v20231001preview.ProvidersAzure{
								Scope: to.Ptr("/subscriptions/test-sub/resourceGroups/test-rg"),
							},
							Aws: &v20231001preview.ProvidersAws{
								Scope: to.Ptr("/planes/aws/aws/accounts/123456789012/regions/us-west-2"),
							},
						},
					},
				},
			},
			applicationName:       "myapp",
			expectedEnvironmentID: "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/myenv",
			expectedApplicationID: "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/applications/myapp",
			expectedAzureScope:    "/subscriptions/test-sub/resourceGroups/test-rg",
			expectedAWSScope:      "/planes/aws/aws/accounts/123456789012/regions/us-west-2",
		},
		{
			name: "Configure with Radius.Core environment",
			envResult: &EnvironmentCheckResult{
				UseApplicationsCore: false,
				RadiusCoreEnv: &v20250801preview.EnvironmentResource{
					ID: to.Ptr("/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/environments/myenv"),
					Properties: &v20250801preview.EnvironmentProperties{
						Providers: &v20250801preview.Providers{
							Azure: &v20250801preview.ProvidersAzure{
								SubscriptionID:    to.Ptr("test-sub-id"),
								ResourceGroupName: to.Ptr("test-rg-name"),
							},
							Aws: &v20250801preview.ProvidersAws{
								Scope: to.Ptr("/planes/aws/aws/accounts/123456789012/regions/us-west-2"),
							},
						},
					},
				},
			},
			applicationName:       "myapp",
			expectedEnvironmentID: "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/environments/myenv",
			expectedApplicationID: "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/applications/myapp",
			expectedAzureScope:    "/planes/azure/azure/Subscriptions/test-sub-id/ResourceGroups/test-rg-name",
			expectedAWSScope:      "/planes/aws/aws/accounts/123456789012/regions/us-west-2",
		},
		{
			name: "Configure without cloud providers",
			envResult: &EnvironmentCheckResult{
				UseApplicationsCore: true,
				ApplicationsCoreEnv: &v20231001preview.EnvironmentResource{
					ID:         to.Ptr("/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/myenv"),
					Properties: &v20231001preview.EnvironmentProperties{},
				},
			},
			applicationName:       "",
			expectedEnvironmentID: "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/myenv",
			expectedApplicationID: "",
			expectedAzureScope:    "",
			expectedAWSScope:      "",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &Runner{
				EnvResult:       tc.envResult,
				ApplicationName: tc.applicationName,
				Workspace: &workspaces.Workspace{
					Scope: "/planes/radius/local/resourceGroups/test-rg",
				},
				Providers: &clients.Providers{
					Radius: &clients.RadiusProvider{},
				},
			}

			err := runner.configureProviders()
			require.NoError(t, err)

			require.Equal(t, tc.expectedEnvironmentID, runner.Providers.Radius.EnvironmentID)
			require.Equal(t, tc.expectedApplicationID, runner.Providers.Radius.ApplicationID)

			if tc.expectedAzureScope != "" {
				require.NotNil(t, runner.Providers.Azure)
				require.Equal(t, tc.expectedAzureScope, runner.Providers.Azure.Scope)
			} else {
				require.Nil(t, runner.Providers.Azure)
			}

			if tc.expectedAWSScope != "" {
				require.NotNil(t, runner.Providers.AWS)
				require.Equal(t, tc.expectedAWSScope, runner.Providers.AWS.Scope)
			} else {
				require.Nil(t, runner.Providers.AWS)
			}
		})
	}
}
