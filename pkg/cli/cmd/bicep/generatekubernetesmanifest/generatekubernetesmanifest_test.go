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

package bicep

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/test/radcli"
	"github.com/spf13/afero"

	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
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
			Name:          "rad bicep generate-kubernetes-manifest - valid",
			Input:         []string{"app.bicep"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/test-environment").
					Return(v20231001preview.EnvironmentResource{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with parameters",
			Input:         []string{"app.bicep", "-p", "foo=bar", "--parameters", "a=b"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), radcli.TestEnvironmentID).
					Return(v20231001preview.EnvironmentResource{}, nil).
					Times(1)

			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with environment",
			Input:         []string{"app.bicep", "-e", "prod"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "prod").
					Return(v20231001preview.EnvironmentResource{
						Properties: &v20231001preview.EnvironmentProperties{
							Providers: &v20231001preview.Providers{
								Azure: &v20231001preview.ProvidersAzure{
									Scope: to.Ptr("/subscriptions/test-subId/resourceGroups/test-rg"),
								},
							},
						},
					}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - env does not exist invalid",
			Input:         []string{"app.bicep", "-e", "prod"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "prod").
					Return(v20231001preview.EnvironmentResource{}, radcli.Create404Error()).
					Times(1)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with env ID",
			Input:         []string{"app.bicep", "-e", "/planes/radius/local/resourceGroups/test-resource-group/providers/applications.core/environments/prod"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
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
				require.Equal(t, scope, runner.Workspace.Scope)
				require.Equal(t, environmentID, runner.Workspace.Environment)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - fallback workspace",
			Input:         []string{"app.bicep", "--group", "my-group", "--environment", "prod"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "prod").
					Return(v20231001preview.EnvironmentResource{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - fallback workspace requires resource group",
			Input:         []string{"app.bicep", "--environment", "prod"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - too many args",
			Input:         []string{"app.bicep", "anotherfile.json"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with outfile",
			Input:         []string{"app.bicep", "--outfile", "test.yaml"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/test-environment").
					Return(v20231001preview.EnvironmentResource{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - invalid outfile",
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
	t.Run("Create basic DeploymentTemplate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bicep := bicep.NewMockInterface(ctrl)
		bicep.EXPECT().
			PrepareTemplate("basic.bicep").
			Return(map[string]any{}, nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Scope: "/planes/radius/local/resourceGroups/test-resource-group",
			Name:  "kind-kind",
		}
		provider := &clients.Providers{
			Radius: &clients.RadiusProvider{
				EnvironmentID: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
			},
		}

		filePath := "basic.bicep"

		outputSink := &output.MockOutput{}
		runner := &Runner{
			Bicep:      bicep,
			Output:     outputSink,
			FilePath:   filePath,
			Parameters: map[string]map[string]any{},
			Workspace:  workspace,
			Providers:  provider,
			FileSystem: afero.NewMemMapFs(),
		}

		fileExists, err := afero.Exists(runner.FileSystem, "basic.yaml")
		require.NoError(t, err)
		require.False(t, fileExists)

		err = runner.Run(context.Background())
		require.NoError(t, err)

		fileExists, err = afero.Exists(runner.FileSystem, "basic.yaml")
		require.NoError(t, err)
		require.True(t, fileExists)

		require.Equal(t, "basic.yaml", runner.OutFile)

		expected, err := os.ReadFile(filepath.Join("testdata", "basic.yaml"))
		require.NoError(t, err)

		// assert that the file contents are as expected
		actual, err := afero.ReadFile(runner.FileSystem, "basic.yaml")
		require.NoError(t, err)
		require.Equal(t, string(expected), string(actual))
	})

	t.Run("Create DeploymentTemplate with template content", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bicep := bicep.NewMockInterface(ctrl)
		bicep.EXPECT().
			PrepareTemplate("value.bicep").
			Return(map[string]any{
				"resources": []map[string]any{
					{
						"some-key": "some-value",
					},
				},
				"parameters": map[string]any{
					"kubernetesNamespace": map[string]any{},
				},
			}, nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Scope: "/planes/radius/local/resourceGroups/test-resource-group",
			Name:  "kind-kind",
		}
		provider := &clients.Providers{
			Radius: &clients.RadiusProvider{
				EnvironmentID: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
			},
		}

		parameters := map[string]map[string]any{
			"kubernetesNamespace": {
				"value": "test-namespace",
			},
		}

		filePath := "value.bicep"

		outputSink := &output.MockOutput{}
		runner := &Runner{
			Bicep:      bicep,
			Output:     outputSink,
			FilePath:   filePath,
			Parameters: parameters,
			Workspace:  workspace,
			Providers:  provider,
			FileSystem: afero.NewMemMapFs(),
		}

		fileExists, err := afero.Exists(runner.FileSystem, "value.yaml")
		require.NoError(t, err)
		require.False(t, fileExists)

		err = runner.Run(context.Background())
		require.NoError(t, err)

		fileExists, err = afero.Exists(runner.FileSystem, "value.yaml")
		require.NoError(t, err)
		require.True(t, fileExists)

		require.Equal(t, "value.yaml", runner.OutFile)

		expected, err := os.ReadFile(filepath.Join("testdata", "value.yaml"))
		require.NoError(t, err)

		// assert that the file contents are as expected
		actual, err := afero.ReadFile(runner.FileSystem, "value.yaml")
		require.NoError(t, err)
		require.Equal(t, string(expected), string(actual))
	})

	t.Run("Create DeploymentTemplate with Azure scope", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bicep := bicep.NewMockInterface(ctrl)
		bicep.EXPECT().
			PrepareTemplate("azure.bicep").
			Return(map[string]any{}, nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Scope: "/planes/radius/local/resourceGroups/test-resource-group",
			Name:  "kind-kind",
		}
		provider := &clients.Providers{
			Radius: &clients.RadiusProvider{
				EnvironmentID: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
			},
			Azure: &clients.AzureProvider{
				Scope: "/subscriptions/test-subId/resourceGroups/test-rg",
			},
		}

		filePath := "azure.bicep"

		outputSink := &output.MockOutput{}
		runner := &Runner{
			Bicep:      bicep,
			Output:     outputSink,
			FilePath:   filePath,
			Parameters: map[string]map[string]any{},
			Workspace:  workspace,
			Providers:  provider,
			FileSystem: afero.NewMemMapFs(),
		}

		fileExists, err := afero.Exists(runner.FileSystem, "azure.yaml")
		require.NoError(t, err)
		require.False(t, fileExists)

		err = runner.Run(context.Background())
		require.NoError(t, err)

		fileExists, err = afero.Exists(runner.FileSystem, "azure.yaml")
		require.NoError(t, err)
		require.True(t, fileExists)

		require.Equal(t, "azure.yaml", runner.OutFile)

		expected, err := os.ReadFile(filepath.Join("testdata", "azure.yaml"))
		require.NoError(t, err)

		// assert that the file contents are as expected
		actual, err := afero.ReadFile(runner.FileSystem, "azure.yaml")
		require.NoError(t, err)
		require.Equal(t, string(expected), string(actual))
	})

	t.Run("Create DeploymentTemplate with AWS scope", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bicep := bicep.NewMockInterface(ctrl)
		bicep.EXPECT().
			PrepareTemplate("aws.bicep").
			Return(map[string]any{}, nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Scope: "/planes/radius/local/resourceGroups/test-resource-group",
			Name:  "kind-kind",
		}
		provider := &clients.Providers{
			Radius: &clients.RadiusProvider{
				EnvironmentID: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
			},
			AWS: &clients.AWSProvider{
				Scope: "awsscope",
			},
		}

		filePath := "aws.bicep"

		outputSink := &output.MockOutput{}
		runner := &Runner{
			Bicep:      bicep,
			Output:     outputSink,
			FilePath:   filePath,
			Parameters: map[string]map[string]any{},
			Workspace:  workspace,
			Providers:  provider,
			FileSystem: afero.NewMemMapFs(),
		}

		fileExists, err := afero.Exists(runner.FileSystem, "aws.yaml")
		require.NoError(t, err)
		require.False(t, fileExists)

		err = runner.Run(context.Background())
		require.NoError(t, err)

		fileExists, err = afero.Exists(runner.FileSystem, "aws.yaml")
		require.NoError(t, err)
		require.True(t, fileExists)

		require.Equal(t, "aws.yaml", runner.OutFile)

		expected, err := os.ReadFile(filepath.Join("testdata", "aws.yaml"))
		require.NoError(t, err)

		// assert that the file contents are as expected
		actual, err := afero.ReadFile(runner.FileSystem, "aws.yaml")
		require.NoError(t, err)
		require.Equal(t, string(expected), string(actual))
	})
}
