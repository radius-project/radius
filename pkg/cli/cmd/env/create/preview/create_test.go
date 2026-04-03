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
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid create command",
			Input:         []string{"testingenv"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupSuccess(mocks.ApplicationManagementClient, "test-resource-group")
			},
			ValidateCallback: func(t *testing.T, runner framework.Runner) {
				r := runner.(*Runner)
				require.Equal(t, "testingenv", r.EnvironmentName)
				require.Equal(t, "test-resource-group", r.ResourceGroupName)
			},
		},
		{
			Name:          "Create command with explicit resource group",
			Input:         []string{"testingenv", "-g", "explicit-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupSuccess(mocks.ApplicationManagementClient, "explicit-group")
			},
			ValidateCallback: func(t *testing.T, runner framework.Runner) {
				r := runner.(*Runner)
				require.Equal(t, "explicit-group", r.ResourceGroupName)
				require.Equal(t, "/planes/radius/local/resourceGroups/explicit-group", r.Workspace.Scope)
			},
		},
		{
			Name:          "Create command with invalid resource group",
			Input:         []string{"testingenv", "-g", "missing-group"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupNotFound(mocks.ApplicationManagementClient, "missing-group")
			},
		},
		{
			Name:          "Create command with resource group lookup error",
			Input:         []string{"testingenv"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupError(mocks.ApplicationManagementClient, "test-resource-group", errors.New("lookup failed"))
			},
		},
		{
			Name:          "Create command with fallback workspace",
			Input:         []string{"testingenv", "--group", "test-resource-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupSuccess(mocks.ApplicationManagementClient, "test-resource-group")
			},
		},
		{
			Name:          "Create command with fallback workspace - requires resource group",
			Input:         []string{"testingenv"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Create command with invalid environment",
			Input:         []string{"testingenv", "-e", "testingenv"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Create command with invalid workspace",
			Input:         []string{"testingenv", "-w", "invalidworkspace"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Create command with extra positional arg",
			Input:         []string{"a", "b"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Success: environment created", func(t *testing.T) {
		workspace := &workspaces.Workspace{
			Name:  "test-workspace",
			Scope: "/planes/radius/local/resourceGroups/test-resource-group",
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
		}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, test_client_factory.WithEnvironmentServerNoError, nil)
		require.NoError(t, err)
		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Output:                  outputSink,
			Workspace:               workspace,
			EnvironmentName:         "testenv",
			ResourceGroupName:       "test-resource-group",
		}

		expectedOutput := []any{
			output.LogOutput{
				Format: "Creating Radius Core Environment...",
			},
			output.LogOutput{
				Format: "Successfully created environment %q in resource group %q",
				Params: []interface{}{
					"testenv",
					"test-resource-group",
				},
			},
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)
		require.Equal(t, expectedOutput, outputSink.Writes)
	})
}

func expectResourceGroupSuccess(client *clients.MockApplicationsManagementClient, name string) {
	resource := radcli.CreateResourceGroup(name)
	client.EXPECT().
		GetResourceGroup(gomock.Any(), "local", name).
		Return(resource, nil).
		Times(1)
}

func expectResourceGroupNotFound(client *clients.MockApplicationsManagementClient, name string) {
	client.EXPECT().
		GetResourceGroup(gomock.Any(), "local", name).
		Return(v20231001preview.ResourceGroupResource{}, radcli.Create404Error()).
		Times(1)
}

func expectResourceGroupError(client *clients.MockApplicationsManagementClient, name string, err error) {
	client.EXPECT().
		GetResourceGroup(gomock.Any(), "local", name).
		Return(v20231001preview.ResourceGroupResource{}, err).
		Times(1)
}
