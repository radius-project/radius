// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package create

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd/api"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

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
			Name:          "valid create command with correct options but non existing radius",
			Input:         []string{"kubernetes", "-w", "ws", "-g", "rg1", "-e", "env1"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// We have a valid kubernetes context, but Radius is not installed
				mocks.Kubernetes.EXPECT().GetKubeContext().Return(getTestKubeConfig(), nil).Times(1)
				mocks.Helm.EXPECT().CheckRadiusInstall(gomock.Any()).Return(false, nil).Times(1)
			},
		},
		{
			Name:          "valid create command with correct options but non existing env resource group",
			Input:         []string{"kubernetes", "-w", "ws", "-g", "rg1", "-e", "env1"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "filePath",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// We have a valid kubernetes context with Radius installed
				mocks.Kubernetes.EXPECT().GetKubeContext().Return(getTestKubeConfig(), nil).Times(1)
				mocks.Helm.EXPECT().CheckRadiusInstall(gomock.Any()).Return(true, nil).Times(1)

				// Resource group does not exist
				mocks.ApplicationManagementClient.EXPECT().ShowUCPGroup(gomock.Any(), "radius", "local", "rg1").Return(ucp.ResourceGroupResource{}, errors.New("group does not exist")).Times(1)
			},
		},
		{
			Name:          "valid create command with correct options but non existing env",
			Input:         []string{"kubernetes", "-w", "ws", "-g", "rg1", "-e", "env1"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// We have a valid kubernetes context with Radius installed
				mocks.Kubernetes.EXPECT().GetKubeContext().Return(getTestKubeConfig(), nil).Times(1)
				mocks.Helm.EXPECT().CheckRadiusInstall(gomock.Any()).Return(true, nil).Times(1)

				// Resource group exists but environment does not
				mocks.ApplicationManagementClient.EXPECT().ShowUCPGroup(gomock.Any(), "radius", "local", "rg1").Return(ucp.ResourceGroupResource{}, nil).Times(1)
				mocks.ApplicationManagementClient.EXPECT().GetEnvDetails(gomock.Any(), "env1").Return(corerp.EnvironmentResource{}, errors.New("environment does not exist")).Times(1)
			},
		},
		{
			Name:          "valid working create command",
			Input:         []string{"kubernetes", "-w", "ws", "-g", "rg1", "-e", "env1"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// We have a valid kubernetes context with Radius installed
				mocks.Kubernetes.EXPECT().GetKubeContext().Return(getTestKubeConfig(), nil).Times(1)
				mocks.Helm.EXPECT().CheckRadiusInstall(gomock.Any()).Return(true, nil).Times(1)

				// Resource group and environment exist
				mocks.ApplicationManagementClient.EXPECT().ShowUCPGroup(gomock.Any(), "radius", "local", "rg1").Return(ucp.ResourceGroupResource{}, nil).Times(1)
				mocks.ApplicationManagementClient.EXPECT().GetEnvDetails(gomock.Any(), "env1").Return(corerp.EnvironmentResource{}, nil).Times(1)
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
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
		}

		configFileInterface := framework.NewMockConfigFileInterface(ctrl)
		configFileInterface.EXPECT().
			EditWorkspaces(context.Background(), gomock.Any(), workspace, gomock.Any()).
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

func getTestKubeConfig() *api.Config {
	kubeContexts := map[string]*api.Context{
		"docker-desktop": {Cluster: "docker-desktop"},
		"k3d-radius-dev": {Cluster: "k3d-radius-dev"},
		"kind-kind":      {Cluster: "kind-kind"},
	}
	return &api.Config{
		CurrentContext: "kind-kind",
		Contexts:       kubeContexts,
	}
}
