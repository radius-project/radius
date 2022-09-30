// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package create

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/configFile"
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
	configWithoutWorkspace := radcli.LoadConfigWithoutWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "create command with incorrect args",
			Input:         []string{""},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "create command with  valid args",
			Input:         []string{"kubernetes", "ws"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithoutWorkspace,
			},
		},
		{
			Name:          "create command with workspace type not kubernetes",
			Input:         []string{"notkubernetes", "b"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Create Command with too many args",
			Input:         []string{"kubernetes", "rg", "env", "ws"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "valid create command with options",
			Input:         []string{"kubernetes", "ws", "-g", "rg1", "-e", "env1", "--context", "kind-kind"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {

	t.Run("Workspace Create", func(t *testing.T) {

		ctrl := gomock.NewController(t)
		outputSink := &output.MockOutput{}
		workspace := &workspaces.Workspace{
			Name: "defaultWorkspace",
			Connection: map[string]interface{}{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
		}

		configFileInterface := configFile.NewMockInterface(ctrl)
		configFileInterface.EXPECT().
			UpdateWorkspaces(context.Background(), "filePath", workspace).
			Return(nil).Times(1)

		runner := &Runner{
			ConfigFileInterface: configFileInterface,
			ConfigHolder:        &framework.ConfigHolder{ConfigFilePath: "filePath"},
			Workspace:           workspace,
			Force:               true,
			Output:              outputSink,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
	})

}
