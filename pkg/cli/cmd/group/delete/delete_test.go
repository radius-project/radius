// ------------------------------------------------------------
// Copyright 2023 The Radius Authors.

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------.

package delete

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/workspaces"
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
			Name:          "Delete Command with incorrect args",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete Command with correct args",
			Input:         []string{"groupname"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete Command with fallback workspace",
			Input:         []string{"groupname"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {

	t.Run("Delete resource group", func(t *testing.T) {
		t.Run("Success (non-existent)", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().DeleteUCPGroup(gomock.Any(), gomock.Any(), gomock.Any(), "testrg").Return(true, nil).Times(2)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory:    &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Workspace:            &workspaces.Workspace{},
				UCPResourceGroupName: "testrg",
				Confirmation:         true,
				Output:               outputSink,
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "deleting resource group %q ...\n",
					Params: []any{"testrg"},
				},
				output.LogOutput{
					Format: "resource group %q deleted",
					Params: []any{"testrg"},
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})

		t.Run("Success (deleted)", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().DeleteUCPGroup(gomock.Any(), gomock.Any(), gomock.Any(), "testrg").Return(false, nil).Times(2)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory:    &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Workspace:            &workspaces.Workspace{},
				UCPResourceGroupName: "testrg",
				Confirmation:         true,
				Output:               outputSink,
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "deleting resource group %q ...\n",
					Params: []any{"testrg"},
				},
				output.LogOutput{
					Format: "resource group %q does not exist or has already been deleted",
					Params: []any{"testrg"},
				},
			}
			require.Equal(t, expected, outputSink.Writes)

		})

		t.Run("Answer no on confirmation", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			outputSink := &output.MockOutput{}

			prompter := prompt.NewMockInterface(ctrl)
			prompter.EXPECT().
				GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, "Are you sure you want to delete the resource group 'testrg'? A resource group can be deleted only when empty").
				Return(prompt.ConfirmNo, nil).
				Times(1)

			runner := &Runner{
				ConnectionFactory:    &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Workspace:            &workspaces.Workspace{},
				UCPResourceGroupName: "testrg",
				Confirmation:         false,
				InputPrompter:        prompter,
				Output:               outputSink,
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "resource group %q NOT deleted",
					Params: []any{"testrg"},
				},
			}
			require.Equal(t, expected, outputSink.Writes)

		})
	})

	t.Run("Exit console with interrupt signal", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		outputSink := &output.MockOutput{}
		prompter := prompt.NewMockInterface(ctrl)
		prompter.EXPECT().
			GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, "Are you sure you want to delete the resource group 'testrg'? A resource group can be deleted only when empty").
			Return("", &prompt.ErrExitConsole{}).
			Times(1)

		runner := &Runner{
			UCPResourceGroupName: "testrg",
			Confirmation:         false,
			InputPrompter:        prompter,
			Output:               outputSink,
		}

		err := runner.Run(context.Background())
		require.Equal(t, err, &prompt.ErrExitConsole{})
		require.Empty(t, outputSink.Writes)

	})

}
