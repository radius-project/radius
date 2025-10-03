// ------------------------------------------------------------
// Copyright 2023 The Radius Authors.

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------.

package delete

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/radcli"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Delete Command with incorrect args",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete Command with correct args",
			Input:         []string{"groupname"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete Command with fallback workspace",
			Input:         []string{"groupname"},
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
	tests := []struct {
		name            string
		confirmation    bool // --yes flag
		resources       []generated.GenericResource
		listError       error
		deleteResult    bool
		deleteError     error
		promptResponse  string
		promptError     error
		expectedPrompt  string
		expectedOutputs []any
		expectedError   error
		skipPrompt      bool // for cases where prompt shouldn't be called
	}{
		{
			name:         "Success with --yes flag and empty group",
			confirmation: true,
			resources:    []generated.GenericResource{},
			deleteResult: true,
			skipPrompt:   true,
			expectedOutputs: []any{
				output.LogOutput{
					Format: "Deleting resource group %s...\n",
					Params: []any{"testrg"},
				},
				output.LogOutput{
					Format: "Resource group %s deleted.",
					Params: []any{"testrg"},
				},
			},
		},
		{
			name:         "Success with --yes flag and resources",
			confirmation: true,
			resources: []generated.GenericResource{
				{Name: to.Ptr("resource1"), Type: to.Ptr("Applications.Core/containers")},
				{Name: to.Ptr("resource2"), Type: to.Ptr("Applications.Core/gateways")},
			},
			deleteResult: true,
			skipPrompt:   true,
			expectedOutputs: []any{
				output.LogOutput{
					Format: "Deleting %d resource(s) in group %s...",
					Params: []any{2, "testrg"},
				},
				output.LogOutput{
					Format: "Deleting resource group %s...\n",
					Params: []any{"testrg"},
				},
				output.LogOutput{
					Format: "Resource group %s deleted.",
					Params: []any{"testrg"},
				},
			},
		},
		{
			name:         "Group already deleted with --yes flag",
			confirmation: true,
			resources:    []generated.GenericResource{},
			deleteResult: false, // indicates group doesn't exist
			skipPrompt:   true,
			expectedOutputs: []any{
				output.LogOutput{
					Format: "Deleting resource group %s...\n",
					Params: []any{"testrg"},
				},
				output.LogOutput{
					Format: "Resource group %s does not exist or has already been deleted.",
					Params: []any{"testrg"},
				},
			},
		},
		{
			name:           "Empty group - user confirms deletion",
			confirmation:   false,
			resources:      []generated.GenericResource{},
			promptResponse: prompt.ConfirmYes,
			expectedPrompt: "The resource group testrg is empty. Are you sure you want to delete the resource group?",
			deleteResult:   true,
			expectedOutputs: []any{
				output.LogOutput{
					Format: "Deleting resource group %s...\n",
					Params: []any{"testrg"},
				},
				output.LogOutput{
					Format: "Resource group %s deleted.",
					Params: []any{"testrg"},
				},
			},
		},
		{
			name:           "Empty group - user cancels deletion",
			confirmation:   false,
			resources:      []generated.GenericResource{},
			promptResponse: prompt.ConfirmNo,
			expectedPrompt: "The resource group testrg is empty. Are you sure you want to delete the resource group?",
			deleteResult:   false, // Won't be called
			expectedOutputs: []any{
				output.LogOutput{
					Format: "Resource group %q NOT deleted",
					Params: []any{"testrg"},
				},
			},
		},
		{
			name:         "Group with resources - user confirms deletion",
			confirmation: false,
			resources: []generated.GenericResource{
				{Name: to.Ptr("resource1"), Type: to.Ptr("Applications.Core/containers")},
				{Name: to.Ptr("resource2"), Type: to.Ptr("Applications.Core/gateways")},
			},
			promptResponse: prompt.ConfirmYes,
			expectedPrompt: "The resource group testrg contains deployed resources. Are you sure you want to delete the resource group and its resources?",
			deleteResult:   true,
			expectedOutputs: []any{
				output.LogOutput{
					Format: "Deleting %d resource(s) in group %s...",
					Params: []any{2, "testrg"},
				},
				output.LogOutput{
					Format: "Deleting resource group %s...\n",
					Params: []any{"testrg"},
				},
				output.LogOutput{
					Format: "Resource group %s deleted.",
					Params: []any{"testrg"},
				},
			},
		},
		{
			name:         "Group with resources - user cancels deletion",
			confirmation: false,
			resources: []generated.GenericResource{
				{Name: to.Ptr("resource1"), Type: to.Ptr("Applications.Core/containers")},
			},
			promptResponse: prompt.ConfirmNo,
			expectedPrompt: "The resource group testrg contains deployed resources. Are you sure you want to delete the resource group and its resources?",
			deleteResult:   false, // Won't be called
			expectedOutputs: []any{
				output.LogOutput{
					Format: "Resource group %q NOT deleted",
					Params: []any{"testrg"},
				},
			},
		},
		{
			name:            "List resources fails - should not proceed",
			confirmation:    false,
			listError:       fmt.Errorf("network error"),
			expectedError:   fmt.Errorf("unable to verify resource group contents: network error"),
			expectedOutputs: nil,  // No output expected, operation should fail
			skipPrompt:      true, // No prompt should be shown
		},
		{
			name:            "Exit console with interrupt signal",
			confirmation:    false,
			resources:       []generated.GenericResource{},
			promptError:     &prompt.ErrExitConsole{},
			expectedPrompt:  "The resource group testrg is empty. Are you sure you want to delete the resource group?",
			expectedError:   &prompt.ErrExitConsole{},
			expectedOutputs: nil, // No output expected
		},
		{
			name:          "Delete operation fails",
			confirmation:  true,
			resources:     []generated.GenericResource{},
			deleteError:   fmt.Errorf("deletion failed"),
			skipPrompt:    true,
			expectedError: fmt.Errorf("deletion failed"),
			expectedOutputs: []any{
				output.LogOutput{
					Format: "Deleting resource group %s...\n",
					Params: []any{"testrg"},
				},
			},
		},
		{
			name:           "List returns 404 - group doesn't exist",
			confirmation:   false,
			listError:      &azcore.ResponseError{StatusCode: http.StatusNotFound},
			promptResponse: prompt.ConfirmYes,
			expectedPrompt: "The resource group testrg is empty. Are you sure you want to delete the resource group?",
			deleteResult:   false,
			expectedOutputs: []any{
				output.LogOutput{
					Format: "Deleting resource group %s...\n",
					Params: []any{"testrg"},
				},
				output.LogOutput{
					Format: "Resource group %s does not exist or has already been deleted.",
					Params: []any{"testrg"},
				},
			},
		},
		{
			name:         "List returns 404 with --yes flag",
			confirmation: true,
			listError:    &azcore.ResponseError{StatusCode: http.StatusNotFound},
			skipPrompt:   true,
			deleteResult: false,
			expectedOutputs: []any{
				output.LogOutput{
					Format: "Deleting resource group %s...\n",
					Params: []any{"testrg"},
				},
				output.LogOutput{
					Format: "Resource group %s does not exist or has already been deleted.",
					Params: []any{"testrg"},
				},
			},
		},
		{
			name:            "List fails with --yes flag - should not proceed",
			confirmation:    true,
			listError:       fmt.Errorf("network error"),
			expectedError:   fmt.Errorf("unable to verify resource group contents: network error"),
			skipPrompt:      true,
			expectedOutputs: nil, // No output expected, operation should fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Setup mocks
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

			// Expect ListResourcesInResourceGroup call
			if tt.listError != nil {
				appManagementClient.EXPECT().
					ListResourcesInResourceGroup(gomock.Any(), "local", "testrg").
					Return(nil, tt.listError).Times(1)
			} else {
				appManagementClient.EXPECT().
					ListResourcesInResourceGroup(gomock.Any(), "local", "testrg").
					Return(tt.resources, nil).Times(1)
			}

			// Setup prompter mock if needed
			var prompter prompt.Interface
			if !tt.skipPrompt && !tt.confirmation {
				mockPrompter := prompt.NewMockInterface(ctrl)
				if tt.expectedPrompt != "" {
					if tt.promptError != nil {
						mockPrompter.EXPECT().
							GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, tt.expectedPrompt).
							Return("", tt.promptError).Times(1)
					} else {
						mockPrompter.EXPECT().
							GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, tt.expectedPrompt).
							Return(tt.promptResponse, nil).Times(1)
					}
				}
				prompter = mockPrompter
			}

			// Expect DeleteResourceGroup call if user confirms or --yes is provided
			// BUT not if we have a list error (other than 404)
			hasNonNotFoundListError := tt.listError != nil && !clients.Is404Error(tt.listError)
			shouldCallDelete := (tt.confirmation || tt.promptResponse == prompt.ConfirmYes) && !hasNonNotFoundListError
			if shouldCallDelete && tt.promptError == nil {
				if tt.deleteError != nil {
					appManagementClient.EXPECT().
						DeleteResourceGroup(gomock.Any(), "local", "testrg").
						Return(false, tt.deleteError).Times(1)
				} else {
					appManagementClient.EXPECT().
						DeleteResourceGroup(gomock.Any(), "local", "testrg").
						Return(tt.deleteResult, nil).Times(1)
				}
			}

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory:    &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Workspace:            &workspaces.Workspace{},
				UCPResourceGroupName: "testrg",
				Confirmation:         tt.confirmation,
				InputPrompter:        prompter,
				Output:               outputSink,
			}

			// Execute
			err := runner.Run(context.Background())

			// Verify results
			if tt.expectedError != nil {
				require.Error(t, err)
				// Check if the error contains the expected message (for wrapped errors)
				require.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.expectedOutputs, outputSink.Writes)
		})
	}
}
