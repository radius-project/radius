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

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
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
			Name:          "Valid Delete Command",
			Input:         []string{"Applications.Core/containers", "foo"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete Command with fallback workspace",
			Input:         []string{"Applications.Core/containers", "foo", "-g", "my-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Delete Command with invalid resource type",
			Input:         []string{"invalidResourceType", "foo"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete Command with insufficient args",
			Input:         []string{"Applications.Core/containers"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete Command with too many args",
			Input:         []string{"Applications.Core/containers", "a", "b"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "List Command with ambiguous args",
			Input:         []string{"Applications.Core/secretStores"},
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
	t.Run("Delete resource", func(t *testing.T) {
		t.Run("Success (non-existent)", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(generated.GenericResource{
					Properties: map[string]interface{}{
						"environment": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/my-test-env",
						"application": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/my-test-app",
					},
				}, nil).
				Times(1)

			appManagementClient.EXPECT().
				DeleteResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(false, nil).
				Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{},
				ResourceType:      "Applications.Core/containers",
				ResourceName:      "test-container",
				Format:            "table",
				Confirm:           true,
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "Resource '%s' of type '%s' does not exist or has already been deleted",
					Params: []any{"test-container", "Applications.Core/containers"},
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})

		t.Run("Success (deleted)", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(generated.GenericResource{
					Properties: map[string]interface{}{
						"environment": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/my-test-env",
						"application": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/my-test-app",
					},
				}, nil).
				Times(1)
			appManagementClient.EXPECT().
				DeleteResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(true, nil).
				Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{},
				ResourceType:      "Applications.Core/containers",
				ResourceName:      "test-container",
				Format:            "table",
				Confirm:           true,
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "Resource deleted",
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})

		t.Run("Success: Prompt Confirmed (case 1: application-scoped standard resource)", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			promptMock := prompt.NewMockInterface(ctrl)
			promptMock.EXPECT().
				GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmationWithApplication, "test-container", "Applications.Core/containers", "my-test-app", "my-test-env")).
				Return(prompt.ConfirmYes, nil).
				Times(1)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(generated.GenericResource{
					Properties: map[string]interface{}{
						"environment": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/my-test-env",
						"application": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/my-test-app",
					},
				}, nil).
				Times(1)
			appManagementClient.EXPECT().
				DeleteResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(true, nil).
				Times(1)

			workspace := &workspaces.Workspace{
				Connection: map[string]any{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name:  "kind-kind",
				Scope: "/planes/radius/local/resourceGroups/test-group",
			}
			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         workspace,
				ResourceType:      "Applications.Core/containers",
				ResourceName:      "test-container",
				Format:            "table",
				InputPrompter:     promptMock,
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "Resource deleted",
				},
			}

			require.Equal(t, expected, outputSink.Writes)
		})

		t.Run("Success: Prompt Confirmed (case 2: environment-scoped standard resource)", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			promptMock := prompt.NewMockInterface(ctrl)
			promptMock.EXPECT().
				GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmationWithoutApplication, "test-container", "Applications.Core/containers", "my-test-env")).
				Return(prompt.ConfirmYes, nil).
				Times(1)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(generated.GenericResource{
					Properties: map[string]interface{}{
						"environment": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/my-test-env",
					},
				}, nil).
				Times(1)
			appManagementClient.EXPECT().
				DeleteResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(true, nil).
				Times(1)

			workspace := &workspaces.Workspace{
				Connection: map[string]any{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name:  "kind-kind",
				Scope: "/planes/radius/local/resourceGroups/test-group",
			}

			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         workspace,
				ResourceType:      "Applications.Core/containers",
				ResourceName:      "test-container",
				Format:            "table",
				InputPrompter:     promptMock,
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "Resource deleted",
				},
			}

			require.Equal(t, expected, outputSink.Writes)
		})

		// NOTE: this case requires an extra lookup to get the environment name.
		t.Run("Success: Prompt Confirmed (case 3: application-scoped core resource)", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			promptMock := prompt.NewMockInterface(ctrl)
			promptMock.EXPECT().
				GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmationWithApplication, "test-container", "Applications.Core/containers", "my-test-app", "my-test-env")).
				Return(prompt.ConfirmYes, nil).
				Times(1)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(generated.GenericResource{
					Properties: map[string]interface{}{
						"application": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/my-test-app",
					},
				}, nil).
				Times(1)

			appManagementClient.EXPECT().
				GetApplication(gomock.Any(), "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/my-test-app").
				Return(v20231001preview.ApplicationResource{
					Properties: &v20231001preview.ApplicationProperties{
						Environment: to.Ptr("/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/my-test-env"),
					},
				}, nil).
				Times(1)
			appManagementClient.EXPECT().
				DeleteResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(true, nil).
				Times(1)

			workspace := &workspaces.Workspace{
				Connection: map[string]any{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name:  "kind-kind",
				Scope: "/planes/radius/local/resourceGroups/test-group",
			}

			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         workspace,
				ResourceType:      "Applications.Core/containers",
				ResourceName:      "test-container",
				Format:            "table",
				InputPrompter:     promptMock,
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "Resource deleted",
				},
			}

			require.Equal(t, expected, outputSink.Writes)
		})

		t.Run("Success: Prompt Confirmed (case 4: no application or environment)", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			promptMock := prompt.NewMockInterface(ctrl)
			promptMock.EXPECT().
				GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmationWithoutApplicationOrEnvironment, "test-container", "Applications.Core/containers")).
				Return(prompt.ConfirmYes, nil).
				Times(1)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(generated.GenericResource{
					Properties: map[string]interface{}{},
				}, nil).
				Times(1)

			appManagementClient.EXPECT().
				DeleteResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(true, nil).
				Times(1)

			workspace := &workspaces.Workspace{
				Connection: map[string]any{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name:  "kind-kind",
				Scope: "/planes/radius/local/resourceGroups/test-group",
			}
			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         workspace,
				ResourceType:      "Applications.Core/containers",
				ResourceName:      "test-container",
				Format:            "table",
				InputPrompter:     promptMock,
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "Resource deleted",
				},
			}

			require.Equal(t, expected, outputSink.Writes)
		})

		t.Run("Success: Prompt Cancelled", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(generated.GenericResource{
					Properties: map[string]interface{}{
						"environment": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/my-test-env",
						"application": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/my-test-app",
					},
				}, nil).
				Times(1)

			promptMock := prompt.NewMockInterface(ctrl)
			promptMock.EXPECT().
				GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmationWithApplication, "test-container", "Applications.Core/containers", "my-test-app", "my-test-env")).
				Return(prompt.ConfirmNo, nil).
				Times(1)

			workspace := &workspaces.Workspace{
				Connection: map[string]any{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name:  "kind-kind",
				Scope: "/planes/radius/local/resourceGroups/test-group",
			}

			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				InputPrompter:     promptMock,
				Workspace:         workspace,
				Format:            "table",
				Output:            outputSink,
				ResourceType:      "Applications.Core/containers",
				ResourceName:      "test-container",
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "resource %q of type %q NOT deleted",
					Params: []any{"test-container", "Applications.Core/containers"},
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})
	})
}
