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
	// testResourceID mirrors the fully qualified IDs the API returns. The runner needs a real ID to
	// address a resource for deletion, and reports any resource that lacks one.
	testResourceID := func(resourceType string, name string) string {
		return "/planes/radius/local/resourceGroups/testrg/providers/" + resourceType + "/" + name
	}

	testResource := func(resourceType string, name string) generated.GenericResource {
		return generated.GenericResource{
			ID:   new(testResourceID(resourceType, name)),
			Name: new(name),
			Type: new(resourceType),
		}
	}

	deletingOutput := func(resourceType string, name string) output.LogOutput {
		return output.LogOutput{
			Format: "  Deleting %s...",
			Params: []any{testResourceID(resourceType, name)},
		}
	}

	groupDeletedOutput := output.LogOutput{
		Format: "System.Resources/resourceGroups/%s deleted",
		Params: []any{"testrg"},
	}

	groupNotFoundOutput := output.LogOutput{
		Format: "System.Resources/resourceGroups/%s not found",
		Params: []any{"testrg"},
	}

	tests := []struct {
		name                string
		confirmation        bool // --yes flag
		resources           []generated.GenericResource
		listError           error
		remainingResources  []generated.GenericResource // returned by the post-delete verification list
		verifyListError     error
		deleteResult        bool
		deleteError         error
		resourceDeleteError error
		promptResponse      string
		promptError         error
		expectedPrompt      string
		expectedOutputs     []any
		expectedError       error
		skipPrompt          bool // for cases where prompt shouldn't be called
	}{
		{
			name:            "Success with --yes flag and empty group",
			confirmation:    true,
			resources:       []generated.GenericResource{},
			deleteResult:    true,
			skipPrompt:      true,
			expectedOutputs: []any{groupDeletedOutput},
		},
		{
			name:         "Success with --yes flag and resources",
			confirmation: true,
			resources: []generated.GenericResource{
				testResource("Applications.Core/containers", "resource1"),
				testResource("Applications.Core/gateways", "resource2"),
			},
			deleteResult: true,
			skipPrompt:   true,
			expectedOutputs: []any{
				deletingOutput("Applications.Core/containers", "resource1"),
				deletingOutput("Applications.Core/gateways", "resource2"),
				groupDeletedOutput,
			},
		},
		{
			name:            "Group already deleted with --yes flag",
			confirmation:    true,
			resources:       []generated.GenericResource{},
			deleteResult:    false, // indicates group doesn't exist
			skipPrompt:      true,
			expectedOutputs: []any{groupNotFoundOutput},
		},
		{
			name:            "Empty group - user confirms deletion",
			confirmation:    false,
			resources:       []generated.GenericResource{},
			promptResponse:  prompt.ConfirmYes,
			expectedPrompt:  "The resource group testrg is empty. Are you sure you want to delete the resource group?",
			deleteResult:    true,
			expectedOutputs: []any{groupDeletedOutput},
		},
		{
			name:            "Empty group - user cancels deletion",
			confirmation:    false,
			resources:       []generated.GenericResource{},
			promptResponse:  prompt.ConfirmNo,
			expectedPrompt:  "The resource group testrg is empty. Are you sure you want to delete the resource group?",
			deleteResult:    false, // Won't be called
			expectedOutputs: nil,
		},
		{
			name:         "Group with resources - user confirms deletion",
			confirmation: false,
			resources: []generated.GenericResource{
				testResource("Applications.Core/containers", "resource1"),
				testResource("Applications.Core/gateways", "resource2"),
			},
			promptResponse: prompt.ConfirmYes,
			expectedPrompt: "The resource group testrg contains deployed resources. Are you sure you want to delete the resource group and its resources?",
			deleteResult:   true,
			expectedOutputs: []any{
				deletingOutput("Applications.Core/containers", "resource1"),
				deletingOutput("Applications.Core/gateways", "resource2"),
				groupDeletedOutput,
			},
		},
		{
			name:         "Group with resources - user cancels deletion",
			confirmation: false,
			resources: []generated.GenericResource{
				testResource("Applications.Core/containers", "resource1"),
			},
			promptResponse:  prompt.ConfirmNo,
			expectedPrompt:  "The resource group testrg contains deployed resources. Are you sure you want to delete the resource group and its resources?",
			deleteResult:    false, // Won't be called
			expectedOutputs: nil,
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
			name:            "Delete operation fails",
			confirmation:    true,
			resources:       []generated.GenericResource{},
			deleteError:     fmt.Errorf("deletion failed"),
			skipPrompt:      true,
			expectedError:   fmt.Errorf("deletion failed"),
			expectedOutputs: nil,
		},
		{
			// The group record must outlive a failed resource delete. Deleting the record while a
			// resource is still present orphans that resource: the record survives in the
			// datastore but becomes unreachable, because UCP resolves the resource group before
			// the resource beneath it. This is the defect that issue #12469 reports, so the case
			// deliberately registers no DeleteResourceGroupRecord expectation -- gomock fails the
			// test if the runner deletes the record anyway.
			name:         "Resource delete fails - group record is not deleted",
			confirmation: true,
			resources: []generated.GenericResource{
				testResource("Applications.Core/containers", "resource1"),
			},
			resourceDeleteError: fmt.Errorf("recipe delete failed"),
			skipPrompt:          true,
			expectedError:       fmt.Errorf("failed to delete resources in resource group testrg"),
			expectedOutputs: []any{
				deletingOutput("Applications.Core/containers", "resource1"),
			},
		},
		{
			// A missing group is reported without prompting: there is nothing to confirm, and
			// asking whether to delete a group that does not exist is misleading.
			name:            "List returns 404 - group doesn't exist",
			confirmation:    false,
			listError:       &azcore.ResponseError{StatusCode: http.StatusNotFound},
			skipPrompt:      true,
			deleteResult:    false,
			expectedOutputs: []any{groupNotFoundOutput},
		},
		{
			name:            "List returns 404 with --yes flag",
			confirmation:    true,
			listError:       &azcore.ResponseError{StatusCode: http.StatusNotFound},
			skipPrompt:      true,
			deleteResult:    false,
			expectedOutputs: []any{groupNotFoundOutput},
		},
		{
			name:            "List fails with --yes flag - should not proceed",
			confirmation:    true,
			listError:       fmt.Errorf("network error"),
			expectedError:   fmt.Errorf("unable to verify resource group contents: network error"),
			skipPrompt:      true,
			expectedOutputs: nil, // No output expected, operation should fail
		},
		{
			// The group is enumerated before the prompt, so a resource deployed into it while the
			// prompt was open or while the tiers were being deleted is not in that snapshot and is
			// not deleted. Deleting the group record around it would orphan it, so the runner
			// re-enumerates the group and keeps the record. No DeleteResourceGroupRecord
			// expectation is registered, so gomock fails the test if the runner deletes it anyway.
			name:         "Resource added during deletion - group record is not deleted",
			confirmation: true,
			resources: []generated.GenericResource{
				testResource("Applications.Core/containers", "resource1"),
			},
			remainingResources: []generated.GenericResource{
				testResource("Applications.Core/containers", "latecomer"),
			},
			skipPrompt: true,
			expectedError: fmt.Errorf("resource group testrg still contains 1 resource: %s after deleting its contents",
				testResourceID("Applications.Core/containers", "latecomer")),
			expectedOutputs: []any{
				deletingOutput("Applications.Core/containers", "resource1"),
			},
		},
		{
			// The same protection has to apply to a group that was empty when the user confirmed:
			// an empty group is the case where a concurrent deployment is most likely to be
			// missed, because the user is told there is nothing to lose.
			name:           "Empty group gains a resource during the prompt - group record is not deleted",
			confirmation:   false,
			resources:      []generated.GenericResource{},
			promptResponse: prompt.ConfirmYes,
			expectedPrompt: "The resource group testrg is empty. Are you sure you want to delete the resource group?",
			remainingResources: []generated.GenericResource{
				testResource("Applications.Core/containers", "latecomer"),
			},
			expectedError: fmt.Errorf("resource group testrg still contains 1 resource: %s after deleting its contents",
				testResourceID("Applications.Core/containers", "latecomer")),
			expectedOutputs: nil,
		},
		{
			// Emptiness that cannot be established is treated the same way as a resource that is
			// known to be left behind: the record is kept, because deleting it would orphan
			// anything that did survive.
			name:         "Verification list fails - group record is not deleted",
			confirmation: true,
			resources: []generated.GenericResource{
				testResource("Applications.Core/containers", "resource1"),
			},
			verifyListError: fmt.Errorf("network error"),
			skipPrompt:      true,
			expectedError:   fmt.Errorf("unable to verify that resource group testrg is empty after deleting its resources: network error"),
			expectedOutputs: []any{
				deletingOutput("Applications.Core/containers", "resource1"),
			},
		},
		{
			// A 404 from the verification list means the group itself is already gone, so there is
			// nothing left to orphan. The record delete still runs and reports the group as not
			// found, which is the same outcome as deleting an already-deleted group.
			name:         "Verification list returns 404 - group already gone",
			confirmation: true,
			resources: []generated.GenericResource{
				testResource("Applications.Core/containers", "resource1"),
			},
			verifyListError: &azcore.ResponseError{StatusCode: http.StatusNotFound},
			deleteResult:    false,
			skipPrompt:      true,
			expectedOutputs: []any{
				deletingOutput("Applications.Core/containers", "resource1"),
				groupNotFoundOutput,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Setup mocks
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

			// Expect ListResourcesInResourceGroup call
			var firstList *gomock.Call
			if tt.listError != nil {
				firstList = appManagementClient.EXPECT().
					ListResourcesInResourceGroup(gomock.Any(), "local", "testrg").
					Return(nil, tt.listError).Times(1)
			} else {
				firstList = appManagementClient.EXPECT().
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

			// Expect the group record deletion if the user confirms or --yes is provided, and the
			// deletion of each resource in the group beforehand. Neither happens if the group could
			// not be enumerated, because its contents would be unknown.
			//
			// Calls are chained with After so that the test fails if the runner reorders them. The
			// deletes within a tier run concurrently and so are only ordered against the calls on
			// either side of them, not against each other.
			listFailed := tt.listError != nil
			shouldCallDelete := (tt.confirmation || tt.promptResponse == prompt.ConfirmYes) && !listFailed
			if shouldCallDelete && tt.promptError == nil {
				deletes := make([]*gomock.Call, 0, len(tt.resources))
				for _, resource := range tt.resources {
					deletes = append(deletes, appManagementClient.EXPECT().
						DeleteResource(gomock.Any(), *resource.Type, *resource.ID, false).
						Return(tt.resourceDeleteError == nil, tt.resourceDeleteError).
						Times(1).
						After(firstList))
				}

				// A failed resource delete must stop the run before the group is re-enumerated and
				// before the group record is deleted, so no expectation is registered for either:
				// gomock then fails the test if the runner calls them anyway.
				if tt.resourceDeleteError == nil {
					// The group is re-enumerated once its contents have been deleted. The record is
					// only deleted when that enumeration proves the group is empty, so a case that
					// leaves resources behind or cannot enumerate the group registers no
					// DeleteResourceGroupRecord expectation.
					verifyList := appManagementClient.EXPECT().
						ListResourcesInResourceGroup(gomock.Any(), "local", "testrg").
						Return(tt.remainingResources, tt.verifyListError).
						Times(1)
					for _, deleteCall := range deletes {
						verifyList.After(deleteCall)
					}

					verified := tt.verifyListError == nil || clients.Is404Error(tt.verifyListError)
					if verified && len(tt.remainingResources) == 0 {
						if tt.deleteError != nil {
							appManagementClient.EXPECT().
								DeleteResourceGroupRecord(gomock.Any(), "local", "testrg").
								Return(false, tt.deleteError).Times(1).After(verifyList)
						} else {
							appManagementClient.EXPECT().
								DeleteResourceGroupRecord(gomock.Any(), "local", "testrg").
								Return(tt.deleteResult, nil).Times(1).After(verifyList)
						}
					}
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
			err := runner.Run(t.Context())

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
