// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package create

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
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
			Name:          "Create Command with incorrect args",
			Input:         []string{""},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Create Command with valid args and fallback workspace",
			Input:         []string{"rg"},
			ExpectedValid: true,
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
		{
			Name:          "Valid Create Command",
			Input:         []string{"rg"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Invalid regource group name",
			Input:         []string{"rg#1"},
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
	t.Run("Run rad group create", func(t *testing.T) {

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().CreateUCPGroup(gomock.Any(), gomock.Any(), gomock.Any(), "testrg", gomock.Any()).Return(true, nil).Times(2)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},

			Name: "kind-kind",
		}
		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory:    &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Workspace:            workspace,
			UCPResourceGroupName: "testrg",
			Output:               outputSink,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.LogOutput{
				Format: "creating resource group %q in workspace %q...\n",
				Params: []any{"testrg", "kind-kind"},
			},
			output.LogOutput{
				Format: "resource group %q created",
				Params: []any{"testrg"},
			},
		}
		require.Equal(t, expected, outputSink.Writes)

	})

}
