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

func Test_Run(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		confirm        bool
		promptChoice   string
		expectDelete   bool
		deleteReturn   bool
		deleteErr      error
		workspaceScope string
		expectedLogs   []output.LogOutput
		expectedErrMsg string
	}{
		{
			name:           "confirm flag bypasses prompt",
			confirm:        true,
			expectDelete:   true,
			deleteReturn:   true,
			workspaceScope: "/planes/radius/local/resourceGroups/test-group",
			expectedLogs: []output.LogOutput{
				{Format: msgDeletingRecipePack, Params: []any{"sample-pack"}},
				{Format: msgRecipePackDeleted, Params: []any{"sample-pack"}},
			},
		},
		{
			name:           "prompt confirms deletion",
			confirm:        false,
			promptChoice:   prompt.ConfirmYes,
			expectDelete:   true,
			deleteReturn:   true,
			workspaceScope: "/planes/radius/local/resourceGroups/test-group",
			expectedLogs: []output.LogOutput{
				{Format: msgDeletingRecipePack, Params: []any{"sample-pack"}},
				{Format: msgRecipePackDeleted, Params: []any{"sample-pack"}},
			},
		},
		{
			name:           "prompt declines deletion",
			confirm:        false,
			promptChoice:   prompt.ConfirmNo,
			expectDelete:   false,
			workspaceScope: "/planes/radius/local/resourceGroups/test-group",
			expectedLogs: []output.LogOutput{
				{Format: msgRecipePackNotDeleted, Params: []any{"sample-pack"}},
			},
		},
		{
			name:           "delete returns not found",
			confirm:        true,
			expectDelete:   true,
			deleteReturn:   false,
			workspaceScope: "/planes/radius/local",
			expectedLogs: []output.LogOutput{
				{Format: msgDeletingRecipePack, Params: []any{"sample-pack"}},
				{Format: msgRecipePackNotFound, Params: []any{"sample-pack"}},
			},
		},
		{
			name:           "delete returns error",
			confirm:        true,
			expectDelete:   true,
			deleteReturn:   false,
			deleteErr:      fmt.Errorf("error"),
			workspaceScope: "/planes/radius/local",
			expectedErrMsg: "failed to delete resource group sample-pack: error",
			expectedLogs: []output.LogOutput{
				{Format: msgDeletingRecipePack, Params: []any{"sample-pack"}},
			},
		},
		{
			name:           "delete returns 404",
			confirm:        true,
			expectDelete:   true,
			deleteReturn:   false,
			deleteErr:      &azcore.ResponseError{StatusCode: 404, ErrorCode: v1.CodeNotFound},
			workspaceScope: "/planes/radius/local",
			expectedLogs: []output.LogOutput{
				{Format: msgDeletingRecipePack, Params: []any{"sample-pack"}},
				{Format: msgRecipePackNotFound, Params: []any{"sample-pack"}},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			promptMock := prompt.NewMockInterface(ctrl)
			if !tc.confirm {
				promptMock.EXPECT().GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmationMsg, "sample-pack")).Return(tc.promptChoice, nil)
			}

			appMgmtClient := clients.NewMockApplicationsManagementClient(ctrl)
			if tc.expectDelete {
				appMgmtClient.EXPECT().DeleteRecipePack(gomock.Any(), "sample-pack").Return(tc.deleteReturn, tc.deleteErr)
			}

			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appMgmtClient},
				Workspace:         &workspaces.Workspace{Name: "test", Scope: tc.workspaceScope},
				Output:            outputSink,
				InputPrompter:     promptMock,
				RecipePackName:    "sample-pack",
				Confirm:           tc.confirm,
			}

			err := runner.Run(context.Background())
			if tc.expectedErrMsg != "" {
				require.Error(t, err)
				require.EqualError(t, err, tc.expectedErrMsg)
			} else {
				require.NoError(t, err)
			}

			expected := make([]any, len(tc.expectedLogs))
			for i, log := range tc.expectedLogs {
				expected[i] = log
			}
			require.Equal(t, expected, outputSink.Writes)
		})
	}
}
