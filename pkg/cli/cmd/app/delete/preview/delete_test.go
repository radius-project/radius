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
	"fmt"
	"net/http"
	"testing"

	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/cli/clients"
	generated "github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Delete command with flag",
			Input:         []string{"-a", "test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Delete command with positional arg",
			Input:         []string{"test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Delete command with fallback workspace",
			Input:         []string{"--application", "test-app", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Delete command with incorrect args",
			Input:         []string{"foo", "bar"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Delete command with Bicep filename",
			Input:         []string{"app.bicep", "--yes"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

// mockManagementClientNoResources returns a mock that has no resources to delete.
func mockManagementClientNoResources(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
	mock := clients.NewMockApplicationsManagementClient(ctrl)
	mock.EXPECT().
		ListResourcesInApplication(gomock.Any(), gomock.Any()).
		Return([]generated.GenericResource{}, nil).
		AnyTimes()
	return mock
}

// mockManagementClientWithResources returns a mock that has resources owned by the app.
// The force flag controls the expected force argument on DeleteResource.
func mockManagementClientWithResources(ctrl *gomock.Controller, appID string, force bool) clients.ApplicationsManagementClient {
	mock := clients.NewMockApplicationsManagementClient(ctrl)

	resourceID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Datastores/redisCaches/my-redis"
	resourceType := "Applications.Datastores/redisCaches"

	mock.EXPECT().
		ListResourcesInApplication(gomock.Any(), appID).
		Return([]generated.GenericResource{
			{
				ID:   &resourceID,
				Type: &resourceType,
			},
		}, nil).
		Times(1)
	mock.EXPECT().
		DeleteResource(gomock.Any(), resourceType, resourceID, force).
		Return(true, nil).
		Times(1)
	return mock
}

func Test_Run(t *testing.T) {
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	t.Run("Success: application deleted (no resources)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		mockMgmt := mockManagementClientNoResources(ctrl)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               workspace,
			Output:                  outputSink,
			ApplicationName:         "test-app",
			Confirm:                 true,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
		require.Equal(t, []any{
			output.LogOutput{
				Format: msgDeletingApplicationPreview,
				Params: []any{"test-app"},
			},
			output.LogOutput{
				Format: msgApplicationDeletedPreview,
			},
		}, outputSink.Writes)
	})

	t.Run("Success: application deleted with cascade resource deletion", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		appID := workspace.Scope + "/providers/Radius.Core/applications/test-app"
		mockMgmt := mockManagementClientWithResources(ctrl, appID, false)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               workspace,
			Output:                  outputSink,
			ApplicationName:         "test-app",
			Confirm:                 true,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)

		// Verify resource count message and final deleted message are present
		require.GreaterOrEqual(t, len(outputSink.Writes), 3)

		// First output should be the resource count message
		firstLog, ok := outputSink.Writes[0].(output.LogOutput)
		require.True(t, ok)
		require.Equal(t, msgDeletingResources, firstLog.Format)

		// Last output should be the application deleted message
		lastLog, ok := outputSink.Writes[len(outputSink.Writes)-1].(output.LogOutput)
		require.True(t, ok)
		require.Equal(t, msgApplicationDeletedPreview, lastLog.Format)
	})

	t.Run("Success: application not found (404)", func(t *testing.T) {
		notFoundServer := func() fake.ApplicationsServer {
			return fake.ApplicationsServer{
				Get: func(
					ctx context.Context,
					rootScope string,
					applicationName string,
					options *corerpv20250801.ApplicationsClientGetOptions,
				) (resp azfake.Responder[corerpv20250801.ApplicationsClientGetResponse], errResp azfake.ErrorResponder) {
					errResp.SetResponseError(http.StatusNotFound, "NotFound")
					return
				},
			}
		}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, notFoundServer)
		require.NoError(t, err)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			Output:                  outputSink,
			ApplicationName:         "test-app",
			Confirm:                 true,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
		require.Equal(t, []any{
			output.LogOutput{
				Format: "Application '%s' does not exist or has already been deleted.",
				Params: []any{"test-app"},
			},
		}, outputSink.Writes)
	})

	t.Run("Success: user declines confirmation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		promptMock := prompt.NewMockInterface(ctrl)
		promptMock.EXPECT().
			GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf("Are you sure you want to delete application '%s'?", "test-app")).
			Return(prompt.ConfirmNo, nil).
			Times(1)

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			Output:                  outputSink,
			InputPrompter:           promptMock,
			ApplicationName:         "test-app",
			Confirm:                 false,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
		require.Equal(t, []any{
			output.LogOutput{
				Format: "Application %q NOT deleted",
				Params: []any{"test-app"},
			},
		}, outputSink.Writes)
	})

	t.Run("Success: --force passes force=true to child resource deletes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		appID := workspace.Scope + "/providers/Radius.Core/applications/test-app"
		mockMgmt := mockManagementClientWithResources(ctrl, appID, true)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               workspace,
			Output:                  outputSink,
			ApplicationName:         "test-app",
			Confirm:                 true,
			Force:                   true,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)

		// First log should be the force warning.
		require.NotEmpty(t, outputSink.Writes)
		firstLog, ok := outputSink.Writes[0].(output.LogOutput)
		require.True(t, ok)
		require.Contains(t, firstLog.Format, "Force deleting an application")
	})

	t.Run("Success: the force warning is shown before the confirmation prompt", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		appID := workspace.Scope + "/providers/Radius.Core/applications/test-app"
		mockMgmt := mockManagementClientWithResources(ctrl, appID, true)

		outputSink := &output.MockOutput{}
		var logsAtPromptTime []any

		// The warning explains that force can leave orphaned external resources, so it is only
		// useful if the user sees it before consenting to the delete.
		promptMock := prompt.NewMockInterface(ctrl)
		promptMock.EXPECT().
			GetListInput(gomock.Any(), gomock.Any()).
			DoAndReturn(func(choices []string, promptMsg string) (string, error) {
				logsAtPromptTime = append([]any{}, outputSink.Writes...)
				return prompt.ConfirmYes, nil
			}).
			Times(1)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               workspace,
			Output:                  outputSink,
			InputPrompter:           promptMock,
			ApplicationName:         "test-app",
			Confirm:                 false,
			Force:                   true,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)

		require.NotEmpty(t, logsAtPromptTime, "the force warning must be logged before the user is asked to confirm")
		firstLog, ok := logsAtPromptTime[0].(output.LogOutput)
		require.True(t, ok)
		require.Contains(t, firstLog.Format, "Force deleting an application")
	})

	t.Run("Failure: child resource delete failure surfaces error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		appID := workspace.Scope + "/providers/Radius.Core/applications/test-app"
		resourceID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Datastores/redisCaches/my-redis"
		resourceType := "Applications.Datastores/redisCaches"

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInApplication(gomock.Any(), appID).
			Return([]generated.GenericResource{{
				ID:   &resourceID,
				Type: &resourceType,
			}}, nil).
			Times(1)
		mockMgmt.EXPECT().
			DeleteResource(gomock.Any(), resourceType, resourceID, false).
			Return(false, fmt.Errorf("simulated delete failure")).
			Times(1)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               workspace,
			Output:                  &output.MockOutput{},
			ApplicationName:         "test-app",
			Confirm:                 true,
		}

		err = runner.Run(t.Context())
		require.Error(t, err)
		require.Contains(t, err.Error(), "Failed to delete resources for application 'test-app'")
	})

	t.Run("Failure: resource enumeration failure surfaces error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInApplication(gomock.Any(), gomock.Any()).
			Return(nil, fmt.Errorf("simulated list error")).
			Times(1)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               workspace,
			Output:                  &output.MockOutput{},
			ApplicationName:         "test-app",
			Confirm:                 true,
		}

		err = runner.Run(t.Context())
		require.Error(t, err)
		require.Contains(t, err.Error(), "simulated list error")
	})

	t.Run("Success: ownership query uses the fully qualified Radius.Core application ID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		// The management client filters on properties.application, so the runner must hand it a
		// Radius.Core ID. A bare name would be qualified as Applications.Core and match nothing.
		expectedAppID := workspace.Scope + "/providers/Radius.Core/applications/test-app"

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInApplication(gomock.Any(), expectedAppID).
			Return([]generated.GenericResource{}, nil).
			Times(1)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               workspace,
			Output:                  &output.MockOutput{},
			ApplicationName:         "test-app",
			Confirm:                 true,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
	})

	t.Run("Success: resources without an ID or type are skipped", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		appID := workspace.Scope + "/providers/Radius.Core/applications/test-app"
		resourceID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Datastores/redisCaches/my-redis"
		resourceType := "Applications.Datastores/redisCaches"

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInApplication(gomock.Any(), appID).
			Return([]generated.GenericResource{
				{ID: &resourceID, Type: &resourceType},
				{ID: nil, Type: &resourceType},
				{ID: &resourceID, Type: nil},
			}, nil).
			Times(1)
		// Only the well-formed resource is deleted.
		mockMgmt.EXPECT().
			DeleteResource(gomock.Any(), resourceType, resourceID, false).
			Return(true, nil).
			Times(1)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               workspace,
			Output:                  &output.MockOutput{},
			ApplicationName:         "test-app",
			Confirm:                 true,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
	})

	t.Run("Success: user accepts confirmation prompt", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		promptMock := prompt.NewMockInterface(ctrl)
		promptMock.EXPECT().
			GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf("Are you sure you want to delete application '%s'?", "test-app")).
			Return(prompt.ConfirmYes, nil).
			Times(1)

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		mockMgmt := mockManagementClientNoResources(ctrl)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               workspace,
			Output:                  outputSink,
			InputPrompter:           promptMock,
			ApplicationName:         "test-app",
			Confirm:                 false,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)

		// Must reach the final "Application deleted" log.
		lastLog, ok := outputSink.Writes[len(outputSink.Writes)-1].(output.LogOutput)
		require.True(t, ok)
		require.Equal(t, msgApplicationDeletedPreview, lastLog.Format)
	})
}
