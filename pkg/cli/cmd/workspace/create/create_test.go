// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package create

import (
	"testing"

	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	configWithoutWorkspace := radcli.LoadConfigWithoutWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Workspace create command with incorrect args",
			Input:         []string{""},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Create Command with  valid args but no workspace",
			Input:         []string{"kubernetes", "ws"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithoutWorkspace,
			},
		},
		{
			Name:          "Create Command with wrong type of workspace",
			Input:         []string{"a", "b"},
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
			Name:          "valid create command",
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

}
