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

package list

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/config"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	testcases := []radcli.ValidateInput{
		{
			Name:          "Delete Command with default application",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadConfigWithWorkspace(t),
				DirectoryConfig: &config.DirectoryConfig{
					Workspace: config.DirectoryWorkspaceConfig{
						Application: "test-application",
					},
				},
			},
		},
		{
			Name:          "Delete Command with flag",
			Input:         []string{"-a", "test-application"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadConfigWithWorkspace(t),
			},
		},
		{
			Name:          "Delete Command with positional arg",
			Input:         []string{"test-application"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadConfigWithWorkspace(t),
			},
		},
		{
			Name:          "Delete Command with confirm",
			Input:         []string{"--yes"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadConfigWithWorkspace(t),
				DirectoryConfig: &config.DirectoryConfig{
					Workspace: config.DirectoryWorkspaceConfig{
						Application: "test-application",
					},
				},
			},
		},
		{
			Name:          "Delete Command with fallback workspace",
			Input:         []string{"--application", "test-application", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Delete Command with incorrect args",
			Input:         []string{"foo", "bar"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadConfigWithWorkspace(t),
			},
		},
		{
			Name:          "Delete Command with Bicep filename",
			Input:         []string{"app.bicep", "--yes"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadConfigWithWorkspace(t),
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Show(t *testing.T) {
	t.Run("Success: Application Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			DeleteApplication(gomock.Any(), "test-app").
			Return(true, nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name:        "kind-kind",
			Scope:       "/planes/radius/local/resourceGroups/test-group",
			Environment: "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default",
		}
		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Workspace:         workspace,
			Output:            outputSink,
			ApplicationName:   "test-app",
			Confirm:           true,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.LogOutput{
				Format: "Application %s deleted",
				Params: []any{"test-app"},
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Success: Prompt Confirmed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name:        "kind-kind",
			Scope:       "/planes/radius/local/resourceGroups/test-group",
			Environment: "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default",
		}

		promptMock := prompt.NewMockInterface(ctrl)
		promptMock.EXPECT().
			GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmation, "test-app", "default")).
			Return(prompt.ConfirmYes, nil).
			Times(1)

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			DeleteApplication(gomock.Any(), "test-app").
			Return(true, nil).
			Times(1)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			InputPrompter:     promptMock,
			Workspace:         workspace,
			Output:            outputSink,
			ApplicationName:   "test-app",
			EnvironmentName:   "default",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.LogOutput{
				Format: "Application %s deleted",
				Params: []any{"test-app"},
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Success: Prompt Cancelled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name:        "kind-kind",
			Scope:       "/planes/radius/local/resourceGroups/test-group",
			Environment: "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default",
		}

		promptMock := prompt.NewMockInterface(ctrl)
		promptMock.EXPECT().
			GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmation, "test-app", "default")).
			Return(prompt.ConfirmNo, nil).
			Times(1)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			InputPrompter:   promptMock,
			Workspace:       workspace,
			Output:          outputSink,
			ApplicationName: "test-app",
			EnvironmentName: "default",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
		require.Empty(t, outputSink.Writes)
	})

	// YES, this is a success case. Delete means "make it be gone", so if the application is already
	// gone that counts as a success.
	//
	// We print a different message which is why it has a separate test
	t.Run("Success: Application Not Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			DeleteApplication(gomock.Any(), "test-app").
			Return(false, nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name:        "kind-kind",
			Scope:       "/planes/radius/local/resourceGroups/test-group",
			Environment: "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default",
		}
		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Workspace:         workspace,
			Output:            outputSink,
			ApplicationName:   "test-app",
			EnvironmentName:   "default",
			Confirm:           true,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.LogOutput{
				Format: "Application '%s' does not exist or has already been deleted.",
				Params: []any{"test-app"},
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})

	// This is a success scenario because the user intended for the interrupt
	t.Run("Success: Console Interrupt", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name:        "kind-kind",
			Scope:       "/planes/radius/local/resourceGroups/test-group",
			Environment: "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default",
		}

		promptMock := prompt.NewMockInterface(ctrl)
		promptMock.EXPECT().
			GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmation, "test-app", "default")).
			Return("", &prompt.ErrExitConsole{}).
			Times(1)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			InputPrompter:   promptMock,
			Output:          outputSink,
			Workspace:       workspace,
			ApplicationName: "test-app",
			EnvironmentName: "default",
		}

		err := runner.Run(context.Background())
		require.Equal(t, &prompt.ErrExitConsole{}, err)
		require.Empty(t, outputSink.Writes)
	})
}
