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
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/env/namespace"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	testResourceGroup := v20231001preview.ResourceGroupResource{
		Name: to.Ptr("test-resource-group"),
	}

	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid create command",
			Input:         []string{"testingenv"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Valid create command
				createMocksWithValidCommand(mocks.Namespace, mocks.ApplicationManagementClient, testResourceGroup)
			},
		},
		{
			Name:          "Create command with explicit resource group",
			Input:         []string{"testingenv", "-g", "test-resource-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Invalid resource group
				createMocksWithValidCommand(mocks.Namespace, mocks.ApplicationManagementClient, testResourceGroup)
			},
			ValidateCallback: func(t *testing.T, runner framework.Runner) {
				r := runner.(*Runner)
				require.Equal(t, "test-resource-group", r.ResourceGroupName)
				require.Equal(t, "/planes/radius/local/resourceGroups/test-resource-group", r.Workspace.Scope)
			},
		},
		{
			Name:          "Create command with invalid resource group",
			Input:         []string{"testingenv", "-g", "invalidresourcegroup"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Invalid resource group
				createShowUCPError(mocks.ApplicationManagementClient, testResourceGroup)
			},
		},
		{
			Name:          "Create command with invalid namespace",
			Input:         []string{"testingenv", "-n", "invalidnamespace"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Invalid create command with invalid namespace
				createMocksWithInvalidResourceGroup(mocks.Namespace, mocks.ApplicationManagementClient, testResourceGroup)
			},
		},
		{
			Name:          "Create command with fallback workspace",
			Input:         []string{"testingenv", "--group", *testResourceGroup.Name},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Valid create command
				createMocksWithValidCommand(mocks.Namespace, mocks.ApplicationManagementClient, testResourceGroup)
			},
		},
		{
			Name:          "Create command with fallback workspace - requires resource group",
			Input:         []string{"testingenv"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Create command with invalid environment",
			Input:         []string{"testingenv", "-e", "testingenv"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Create command with invalid workspace",
			Input:         []string{"testingenv", "-w", "invalidworkspace"},
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
	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

		namespaceClient := namespace.NewMockInterface(ctrl)
		testEnvProperties := &corerp.EnvironmentProperties{
			Compute: &corerp.KubernetesCompute{
				Namespace: to.Ptr("default"),
			},
		}
		appManagementClient.EXPECT().
			CreateEnvironment(context.Background(), "default", v1.LocationGlobal, testEnvProperties).
			Return(nil).Times(1)

		configFileInterface := framework.NewMockConfigFileInterface(ctrl)
		outputSink := &output.MockOutput{}
		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name:  "defaultWorkspace",
			Scope: "/planes/radius/local/resourceGroups/test-group",
		}

		runner := &Runner{
			ConnectionFactory:   &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			ConfigHolder:        &framework.ConfigHolder{ConfigFilePath: "filePath"},
			Output:              outputSink,
			Workspace:           workspace,
			EnvironmentName:     "default",
			Namespace:           "default",
			NamespaceInterface:  namespaceClient,
			ConfigFileInterface: configFileInterface,
			ResourceGroupName:   "test-group",
		}

		expectedOutput := []any{
			output.LogOutput{
				Format: "Creating Environment...",
			},
			output.LogOutput{
				Format: "Successfully created environment %q in resource group %q",
				Params: []interface{}{
					"default",
					"test-group",
				},
			},
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
		require.Equal(t, expectedOutput, outputSink.Writes)
	})

	t.Run("Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

		namespaceClient := namespace.NewMockInterface(ctrl)
		testEnvProperties := &corerp.EnvironmentProperties{
			Compute: &corerp.KubernetesCompute{
				Namespace: to.Ptr("default"),
			},
		}

		expectedError := errors.New("failed to create the environment")

		appManagementClient.EXPECT().
			CreateEnvironment(context.Background(), "default", v1.LocationGlobal, testEnvProperties).
			Return(expectedError).
			Times(1)

		configFileInterface := framework.NewMockConfigFileInterface(ctrl)
		outputSink := &output.MockOutput{}
		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name:  "defaultWorkspace",
			Scope: "/planes/radius/local/resourceGroups/test-group",
		}

		runner := &Runner{
			ConnectionFactory:   &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			ConfigHolder:        &framework.ConfigHolder{ConfigFilePath: "filePath"},
			Output:              outputSink,
			Workspace:           workspace,
			EnvironmentName:     "default",
			Namespace:           "default",
			NamespaceInterface:  namespaceClient,
			ConfigFileInterface: configFileInterface,
		}

		expectedOutput := []any{
			output.LogOutput{
				Format: "Creating Environment...",
			},
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Equal(t, expectedError, err)
		require.Equal(t, expectedOutput, outputSink.Writes)
	})
}

func createMocksWithValidCommand(namespaceClient *namespace.MockInterface, appManagementClient *clients.MockApplicationsManagementClient, testResourceGroup v20231001preview.ResourceGroupResource) {
	createShowUCPSuccess(appManagementClient, testResourceGroup)
	createValidateNamespaceSuccess(namespaceClient)
}

func createMocksWithInvalidResourceGroup(namespaceClient *namespace.MockInterface, appManagementClient *clients.MockApplicationsManagementClient, testResourceGroup v20231001preview.ResourceGroupResource) {
	createShowUCPSuccess(appManagementClient, testResourceGroup)
	createValidateNamespaceError(namespaceClient)
}

func createValidateNamespaceSuccess(namespaceClient *namespace.MockInterface) {
	namespaceClient.EXPECT().
		ValidateNamespace(gomock.Any(), "testingenv", gomock.Any()).
		Return(nil).Times(1)
}

func createValidateNamespaceError(namespaceClient *namespace.MockInterface) {
	namespaceClient.EXPECT().
		ValidateNamespace(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fmt.Errorf("failed to create namespace")).Times(1)
}

func createShowUCPSuccess(appManagementClient *clients.MockApplicationsManagementClient, testResourceGroup v20231001preview.ResourceGroupResource) {
	appManagementClient.EXPECT().
		ShowUCPGroup(gomock.Any(), gomock.Any(), gomock.Any(), "test-resource-group").
		Return(testResourceGroup, nil).Times(1)
}

func createShowUCPError(appManagementClient *clients.MockApplicationsManagementClient, testResourceGroup v20231001preview.ResourceGroupResource) {
	appManagementClient.EXPECT().
		ShowUCPGroup(gomock.Any(), gomock.Any(), gomock.Any(), "invalidresourcegroup").
		Return(testResourceGroup, clierrors.Message("The resource group %q could not be found.", "invalidresourcegroup")).Times(1)

}
