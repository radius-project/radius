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

package preview

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/radius-project/radius/pkg/cli/clients"
	workspace_create "github.com/radius-project/radius/pkg/cli/cmd/workspace/create"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	ucp "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "preview create command with workspace type not kubernetes",
			Input:         []string{"notkubernetes", "b"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "preview create command with too many args",
			Input:         []string{"kubernetes", "rg", "env", "ws"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "preview create command with radius not installed",
			Input:         []string{"kubernetes", "-w", "ws", "-g", "rg1"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.Kubernetes.EXPECT().GetKubeContext().Return(getTestKubeConfig(), nil).Times(1)
				mocks.Helm.EXPECT().CheckRadiusInstall(gomock.Any()).Return(helm.InstallState{RadiusInstalled: false}, nil).Times(1)
			},
		},
		{
			Name:          "preview create command with non-existing resource group",
			Input:         []string{"kubernetes", "-w", "ws", "-g", "rg1"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "filePath",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.Kubernetes.EXPECT().GetKubeContext().Return(getTestKubeConfig(), nil).Times(1)
				mocks.Helm.EXPECT().CheckRadiusInstall(gomock.Any()).Return(helm.InstallState{RadiusInstalled: true}, nil).Times(1)
				mocks.ApplicationManagementClient.EXPECT().GetResourceGroup(gomock.Any(), "local", "rg1").Return(ucp.ResourceGroupResource{}, radcli.Create404Error()).Times(1)
			},
		},
		{
			Name:          "preview create command with valid resource group and no environment",
			Input:         []string{"kubernetes", "-w", "ws", "-g", "rg1"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.Kubernetes.EXPECT().GetKubeContext().Return(getTestKubeConfig(), nil).Times(1)
				mocks.Helm.EXPECT().CheckRadiusInstall(gomock.Any()).Return(helm.InstallState{RadiusInstalled: true}, nil).Times(1)
				mocks.ApplicationManagementClient.EXPECT().GetResourceGroup(gomock.Any(), "local", "rg1").Return(ucp.ResourceGroupResource{}, nil).Times(1)
			},
		},
		{
			Name:          "preview create command with environment but empty scope",
			Input:         []string{"kubernetes", "-w", "ws", "-e", "env1"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.Kubernetes.EXPECT().GetKubeContext().Return(getTestKubeConfig(), nil).Times(1)
				mocks.Helm.EXPECT().CheckRadiusInstall(gomock.Any()).Return(helm.InstallState{RadiusInstalled: true}, nil).Times(1)
			},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Validate_Environment(t *testing.T) {
	scope := "/planes/radius/local/resourceGroups/rg1"

	t.Run("environment exists", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, nil, nil)
		require.NoError(t, err)

		runner, cmd := newRunnerForEnvValidation(t, ctrl, factory)
		require.NoError(t, cmd.ParseFlags([]string{"kubernetes", "-w", "ws", "-g", "rg1", "-e", "env1"}))

		err = runner.Validate(cmd, []string{"kubernetes"})
		require.NoError(t, err)
		require.Equal(t, "/planes/radius/local/resourceGroups/rg1/providers/Radius.Core/environments/env1", runner.Workspace.Environment)
	})

	t.Run("environment not found returns clierror referencing Radius.Core", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			scope,
			test_client_factory.WithEnvironmentServer404OnGet,
			nil,
		)
		require.NoError(t, err)

		runner, cmd := newRunnerForEnvValidation(t, ctrl, factory)
		require.NoError(t, cmd.ParseFlags([]string{"kubernetes", "-w", "ws", "-g", "rg1", "-e", "env1"}))

		err = runner.Validate(cmd, []string{"kubernetes"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "Radius.Core/environments/env1")
		require.Contains(t, err.Error(), "rad env create --preview")
	})

	t.Run("non-404 error is propagated and not masked as not-found", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			scope,
			test_client_factory.WithEnvironmentServer500OnGet,
			nil,
		)
		require.NoError(t, err)

		runner, cmd := newRunnerForEnvValidation(t, ctrl, factory)
		require.NoError(t, cmd.ParseFlags([]string{"kubernetes", "-w", "ws", "-g", "rg1", "-e", "env1"}))

		err = runner.Validate(cmd, []string{"kubernetes"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "Failed to get environment")
		require.Contains(t, err.Error(), "Radius.Core/environments/env1")
		require.NotContains(t, err.Error(), "does not exist")
	})
}

func Test_Run(t *testing.T) {
	t.Run("Workspace Create Preview", func(t *testing.T) {
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
			Runner: &workspace_create.Runner{
				ConfigFileInterface: configFileInterface,
				ConfigHolder:        &framework.ConfigHolder{ConfigFilePath: "filePath"},
				Workspace:           workspace,
				Force:               true,
				Output:              outputSink,
			},
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
	})
}

// newRunnerForEnvValidation builds a Runner and cobra command pre-wired with mocks for
// the kubernetes/helm/resource-group calls so tests can focus on the Radius.Core
// environment validation path.
func newRunnerForEnvValidation(t *testing.T, ctrl *gomock.Controller, factory *corerpv20250801.ClientFactory) (*Runner, *cobra.Command) {
	t.Helper()

	configHolder := &framework.ConfigHolder{Config: radcli.LoadConfigWithWorkspace(t)}

	kubeMock := kubernetes.NewMockInterface(ctrl)
	kubeMock.EXPECT().GetKubeContext().Return(getTestKubeConfig(), nil).Times(1)

	helmMock := helm.NewMockInterface(ctrl)
	helmMock.EXPECT().CheckRadiusInstall(gomock.Any()).Return(helm.InstallState{RadiusInstalled: true}, nil).Times(1)

	mgmtMock := clients.NewMockApplicationsManagementClient(ctrl)
	mgmtMock.EXPECT().GetResourceGroup(gomock.Any(), "local", "rg1").Return(ucp.ResourceGroupResource{}, nil).Times(1)

	fw := &framework.Impl{
		ConfigHolder:        configHolder,
		Output:              &output.MockOutput{},
		KubernetesInterface: kubeMock,
		HelmInterface:       helmMock,
		ConnectionFactory:   &connections.MockFactory{ApplicationsManagementClient: mgmtMock},
	}

	cmd, runnerIface := NewCommand(fw)
	cmd.SetContext(context.Background())
	runner := runnerIface.(*Runner)
	runner.RadiusCoreClientFactory = factory

	return runner, cmd
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
