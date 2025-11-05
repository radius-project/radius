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

package delete

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Missing recipe pack name",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid with workspace flag",
			Input:         []string{"my-pack", "-w", radcli.TestWorkspaceName},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid with group fallback",
			Input:         []string{"--group", "test-group", "my-pack"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run_DeleteConfirmed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	appMgmtClient := clients.NewMockApplicationsManagementClient(ctrl)
	appMgmtClient.EXPECT().DeleteRecipePack(gomock.Any(), "sample-pack").Return(true, nil)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appMgmtClient},
		Workspace:         &workspaces.Workspace{Name: "test", Scope: "/planes/radius/local/resourceGroups/test-group"},
		Output:            outputSink,
		InputPrompter:     prompt.NewMockInterface(ctrl),
		RecipePackName:    "sample-pack",
		Confirm:           true,
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	expected := []any{
		output.LogOutput{Format: msgDeletingRecipePack, Params: []any{"sample-pack"}},
		output.LogOutput{Format: msgRecipePackDeleted, Params: []any{"sample-pack"}},
	}
	require.Equal(t, expected, outputSink.Writes)
}

func Test_Run_DeleteWithPromptConfirmed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	promptMock := prompt.NewMockInterface(ctrl)
	promptMock.EXPECT().GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmationMsg, "sample-pack")).Return(prompt.ConfirmYes, nil)

	appMgmtClient := clients.NewMockApplicationsManagementClient(ctrl)
	appMgmtClient.EXPECT().DeleteRecipePack(gomock.Any(), "sample-pack").Return(true, nil)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appMgmtClient},
		Workspace:         &workspaces.Workspace{Name: "test", Scope: "/planes/radius/local/resourceGroups/test-group"},
		Output:            outputSink,
		InputPrompter:     promptMock,
		RecipePackName:    "sample-pack",
		Confirm:           false,
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	expected := []any{
		output.LogOutput{Format: msgDeletingRecipePack, Params: []any{"sample-pack"}},
		output.LogOutput{Format: msgRecipePackDeleted, Params: []any{"sample-pack"}},
	}
	require.Equal(t, expected, outputSink.Writes)
}

func Test_Run_DeleteWithPromptDeclined(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	promptMock := prompt.NewMockInterface(ctrl)
	promptMock.EXPECT().GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmationMsg, "sample-pack")).Return(prompt.ConfirmNo, nil)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: clients.NewMockApplicationsManagementClient(ctrl)},
		Workspace:         &workspaces.Workspace{Name: "test", Scope: "/planes/radius/local/resourceGroups/test-group"},
		Output:            outputSink,
		InputPrompter:     promptMock,
		RecipePackName:    "sample-pack",
		Confirm:           false,
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	expected := []any{
		output.LogOutput{Format: msgRecipePackNotDeleted, Params: []any{"sample-pack"}},
	}
	require.Equal(t, expected, outputSink.Writes)
}

func Test_Run_DeleteNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	appMgmtClient := clients.NewMockApplicationsManagementClient(ctrl)
	appMgmtClient.EXPECT().DeleteRecipePack(gomock.Any(), "sample-pack").Return(false, nil)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appMgmtClient},
		Workspace:         &workspaces.Workspace{Name: "test", Scope: "/planes/radius/local"},
		Output:            outputSink,
		InputPrompter:     prompt.NewMockInterface(ctrl),
		RecipePackName:    "sample-pack",
		Confirm:           true,
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	expected := []any{
		output.LogOutput{Format: msgDeletingRecipePack, Params: []any{"sample-pack"}},
		output.LogOutput{Format: msgRecipePackNotFound, Params: []any{"sample-pack"}},
	}
	require.Equal(t, expected, outputSink.Writes)
}

func Test_Run_DeleteError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	deleteErr := fmt.Errorf("boom")

	appMgmtClient := clients.NewMockApplicationsManagementClient(ctrl)
	appMgmtClient.EXPECT().DeleteRecipePack(gomock.Any(), "sample-pack").Return(false, deleteErr)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appMgmtClient},
		Workspace:         &workspaces.Workspace{Name: "test", Scope: "/planes/radius/local"},
		Output:            outputSink,
		InputPrompter:     prompt.NewMockInterface(ctrl),
		RecipePackName:    "sample-pack",
		Confirm:           true,
	}

	err := runner.Run(context.Background())
	require.Error(t, err)
	require.Equal(t, deleteErr, err)

	expected := []any{
		output.LogOutput{Format: msgDeletingRecipePack, Params: []any{"sample-pack"}},
	}
	require.Equal(t, expected, outputSink.Writes)
}

func Test_Run_DeleteError404(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	notFoundErr := &azcore.ResponseError{StatusCode: 404, ErrorCode: v1.CodeNotFound}

	appMgmtClient := clients.NewMockApplicationsManagementClient(ctrl)
	appMgmtClient.EXPECT().DeleteRecipePack(gomock.Any(), "sample-pack").Return(false, notFoundErr)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appMgmtClient},
		Workspace:         &workspaces.Workspace{Name: "test", Scope: "/planes/radius/local"},
		Output:            outputSink,
		InputPrompter:     prompt.NewMockInterface(ctrl),
		RecipePackName:    "sample-pack",
		Confirm:           true,
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	expected := []any{
		output.LogOutput{Format: msgDeletingRecipePack, Params: []any{"sample-pack"}},
		output.LogOutput{Format: msgRecipePackNotFound, Params: []any{"sample-pack"}},
	}
	require.Equal(t, expected, outputSink.Writes)
}
