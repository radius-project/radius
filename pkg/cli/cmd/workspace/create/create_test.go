// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package create

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
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
	/*
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		helmMock := helm.NewMockInterface(ctrl)

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

		//Setup a non existant radius scenario
		helmMock.EXPECT().CheckRadiusInstall(gomock.Any()).Return(false, nil).Times(1)

		//Setup non existant group scenario
		helmMock.EXPECT().CheckRadiusInstall(gomock.Any()).Return(true, nil).Times(1)
		appManagementClient.EXPECT().ShowUCPGroup(gomock.Any(), "radius", "local", "rg1").Return(ucp.ResourceGroupResource{}, errors.New("af")).Times(1)

		//Setup existant group scenario and non existing env
		helmMock.EXPECT().CheckRadiusInstall(gomock.Any()).Return(true, nil).Times(1)
		appManagementClient.EXPECT().ShowUCPGroup(gomock.Any(), "radius", "local", "rg1").Return(ucp.ResourceGroupResource{}, nil).Times(1)
		appManagementClient.EXPECT().GetEnvDetails(gomock.Any(), "env1").Return(corerp.EnvironmentResource{}, errors.New("af")).Times(1)*/

	testcases := []radcli.ValidateInput{

		{
			Name:          "create command with workspace type not kubernetes",
			Input:         []string{"notkubernetes", "b"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		/*{
			Name:          "Create Command with too many args",
			Input:         []string{"kubernetes", "rg", "env", "ws"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "valid create command with correct options but non existing radius",
			Input:         []string{"kubernetes", "-w", "ws", "-g", "rg1", "-e", "env1"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			HelmInterface:     helmMock,
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
		},
		{
			Name:          "valid create command with correct options but non existing env",
			Input:         []string{"kubernetes", "-w", "ws", "-g", "rg1", "-e", "env1"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "filePath",
				Config:         configWithWorkspace,
			},
			HelmInterface:     helmMock,
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
		},
		{
			Name:          "valid create command with correct options but non existing env",
			Input:         []string{"kubernetes", "-w", "ws", "-g", "rg1", "-e", "env1"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			HelmInterface:     helmMock,
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
		},*/
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

		configFileInterface := framework.NewMockConfigFileInterface(ctrl)
		configFileInterface.EXPECT().
			EditWorkspaces(context.Background(), gomock.Any(), workspace, nil).
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
