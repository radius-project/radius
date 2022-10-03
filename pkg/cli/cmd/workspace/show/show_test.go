// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

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

	testcases := []radcli.ValidateInput{
		{
			Name:          "Show Command with incorrect args",
			Input:         []string{"foo", "bar"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with correct options",
			Input:         []string{"test-workspace"},
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

	t.Run("Validate rad workspace show", func(t *testing.T) {

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
			ShowWorkspace(gomock.Any(), gomock.Any(), workspace).
			Return(nil).Times(1)

		runner := &Runner{
			ConfigFileInterface: configFileInterface,
			ConfigHolder:        &framework.ConfigHolder{ConfigFilePath: "filePath"},
			Workspace:           workspace,
			Output:              outputSink,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

	})
}
