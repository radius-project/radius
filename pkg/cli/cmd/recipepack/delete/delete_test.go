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
	"net/http"
	"testing"

	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	v20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	corerpfake "github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/pkg/to"
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
	scope := "/planes/radius/local/resourceGroups/test-group"
	packName := "test-pack"
	packFullID := scope + "/providers/Radius.Core/recipePacks/" + packName
	envName := "test-env"
	envFullID := scope + "/providers/Radius.Core/environments/" + envName

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind":    "kubernetes",
			"context": "kind-kind",
		},
		Name:  "kind-kind",
		Scope: scope,
	}

	t.Run("delete removes pack from referenced env then deletes pack", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		var capturedEnv v20250801preview.EnvironmentResource

		appMgmtClient := clients.NewMockApplicationsManagementClient(ctrl)
		appMgmtClient.EXPECT().
			GetRecipePack(gomock.Any(), packName).
			Return(v20250801preview.RecipePackResource{
				ID:   to.Ptr(packFullID),
				Name: to.Ptr(packName),
				Properties: &v20250801preview.RecipePackProperties{
					Recipes:      map[string]*v20250801preview.RecipeDefinition{},
					ReferencedBy: []*string{to.Ptr(envFullID)},
				},
			}, nil)
		appMgmtClient.EXPECT().
			DeleteRecipePack(gomock.Any(), packName).
			Return(true, nil)

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, func() corerpfake.EnvironmentsServer {
			return corerpfake.EnvironmentsServer{
				Get: func(
					ctx context.Context, environmentName string,
					_ *v20250801preview.EnvironmentsClientGetOptions,
				) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
					resp.SetResponse(http.StatusOK, v20250801preview.EnvironmentsClientGetResponse{
						EnvironmentResource: v20250801preview.EnvironmentResource{
							Name: to.Ptr(environmentName),
							Properties: &v20250801preview.EnvironmentProperties{
								RecipePacks: []*string{to.Ptr(packFullID)},
							},
						},
					}, nil)
					return
				},
				CreateOrUpdate: func(
					ctx context.Context, environmentName string,
					resource v20250801preview.EnvironmentResource,
					_ *v20250801preview.EnvironmentsClientCreateOrUpdateOptions,
				) (resp azfake.Responder[v20250801preview.EnvironmentsClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
					capturedEnv = resource
					resp.SetResponse(http.StatusOK, v20250801preview.EnvironmentsClientCreateOrUpdateResponse{EnvironmentResource: resource}, nil)
					return
				},
			}
		}, nil)
		require.NoError(t, err)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: appMgmtClient},
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			Output:                  outputSink,
			RecipePackName:          packName,
			Confirm:                 true,
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)

		// Pack must be removed from the environment's RecipePacks list.
		require.Empty(t, capturedEnv.Properties.RecipePacks)

		require.Equal(t, []any{
			output.LogOutput{Format: msgDeletingRecipePack, Params: []any{packName}},
			output.LogOutput{Format: msgRecipePackDeleted, Params: []any{packName}},
		}, outputSink.Writes)
	})

	t.Run("delete with no referenced envs deletes pack directly", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appMgmtClient := clients.NewMockApplicationsManagementClient(ctrl)
		appMgmtClient.EXPECT().
			GetRecipePack(gomock.Any(), packName).
			Return(v20250801preview.RecipePackResource{
				ID:   to.Ptr(packFullID),
				Name: to.Ptr(packName),
				Properties: &v20250801preview.RecipePackProperties{
					Recipes:      map[string]*v20250801preview.RecipeDefinition{},
					ReferencedBy: nil,
				},
			}, nil)
		appMgmtClient.EXPECT().
			DeleteRecipePack(gomock.Any(), packName).
			Return(true, nil)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appMgmtClient},
			Workspace:         workspace,
			Output:            outputSink,
			RecipePackName:    packName,
			Confirm:           true,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
		require.Equal(t, []any{
			output.LogOutput{Format: msgDeletingRecipePack, Params: []any{packName}},
			output.LogOutput{Format: msgRecipePackDeleted, Params: []any{packName}},
		}, outputSink.Writes)
	})

	t.Run("delete skips referenced env that is already deleted", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appMgmtClient := clients.NewMockApplicationsManagementClient(ctrl)
		appMgmtClient.EXPECT().
			GetRecipePack(gomock.Any(), packName).
			Return(v20250801preview.RecipePackResource{
				ID:   to.Ptr(packFullID),
				Name: to.Ptr(packName),
				Properties: &v20250801preview.RecipePackProperties{
					Recipes:      map[string]*v20250801preview.RecipeDefinition{},
					ReferencedBy: []*string{to.Ptr(envFullID)},
				},
			}, nil)
		appMgmtClient.EXPECT().
			DeleteRecipePack(gomock.Any(), packName).
			Return(true, nil)

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, func() corerpfake.EnvironmentsServer {
			return corerpfake.EnvironmentsServer{
				Get: func(
					ctx context.Context, environmentName string,
					_ *v20250801preview.EnvironmentsClientGetOptions,
				) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
					errResp.SetResponseError(404, "Not Found")
					return
				},
			}
		}, nil)
		require.NoError(t, err)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: appMgmtClient},
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			Output:                  outputSink,
			RecipePackName:          packName,
			Confirm:                 true,
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)
		require.Equal(t, []any{
			output.LogOutput{Format: msgDeletingRecipePack, Params: []any{packName}},
			output.LogOutput{Format: msgRecipePackDeleted, Params: []any{packName}},
		}, outputSink.Writes)
	})

	t.Run("user declines confirmation — pack not deleted", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appMgmtClient := clients.NewMockApplicationsManagementClient(ctrl)
		promptMock := prompt.NewMockInterface(ctrl)

		promptMock.EXPECT().
			GetListInput(gomock.Any(), gomock.Any()).
			Return(prompt.ConfirmNo, nil)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appMgmtClient},
			InputPrompter:     promptMock,
			Workspace:         workspace,
			Output:            outputSink,
			RecipePackName:    packName,
			Confirm:           false,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
		require.Equal(t, []any{
			output.LogOutput{Format: msgRecipePackNotDeleted, Params: []any{packName}},
		}, outputSink.Writes)
	})
}
