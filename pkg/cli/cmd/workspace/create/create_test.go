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

package create

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20220315privatepreview"
	ucp "github.com/radius-project/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/radius-project/radius/test/radcli"
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
				mocks.Helm.EXPECT().CheckRadiusInstall(gomock.Any()).Return(helm.InstallState{Installed: false}, nil).Times(1)
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
				mocks.Helm.EXPECT().CheckRadiusInstall(gomock.Any()).Return(helm.InstallState{Installed: true}, nil).Times(1)

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
				mocks.Helm.EXPECT().CheckRadiusInstall(gomock.Any()).Return(helm.InstallState{Installed: true}, nil).Times(1)

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
				mocks.Helm.EXPECT().CheckRadiusInstall(gomock.Any()).Return(helm.InstallState{Installed: true}, nil).Times(1)

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
			EditWorkspaces(context.Background(), gomock.Any(), workspace).
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
