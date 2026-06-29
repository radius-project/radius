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
	"net/http"
	"testing"

	azcore "github.com/Azure/azure-sdk-for-go/sdk/azcore"
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
					ctx context.Context, rootScope string, environmentName string,
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
					ctx context.Context, rootScope string, environmentName string,
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
					ctx context.Context, rootScope string, environmentName string,
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
		require.Empty(t, outputSink.Writes)
	})

	t.Run("get recipe pack returns 404 — returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appMgmtClient := clients.NewMockApplicationsManagementClient(ctrl)
		appMgmtClient.EXPECT().
			GetRecipePack(gomock.Any(), packName).
			Return(v20250801preview.RecipePackResource{}, &azcore.ResponseError{StatusCode: http.StatusNotFound})

		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appMgmtClient},
			Workspace:         workspace,
			Output:            outputSink,
			RecipePackName:    packName,
			Confirm:           true,
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), packName)
	})

	t.Run("get recipe pack returns error — propagates error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appMgmtClient := clients.NewMockApplicationsManagementClient(ctrl)
		appMgmtClient.EXPECT().
			GetRecipePack(gomock.Any(), packName).
			Return(v20250801preview.RecipePackResource{}, fmt.Errorf("test error"))

		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appMgmtClient},
			Workspace:         workspace,
			Output:            outputSink,
			RecipePackName:    packName,
			Confirm:           true,
		}

		err := runner.Run(context.Background())
		require.EqualError(t, err, "test error")
	})

	t.Run("env get returns non-404 error — returns error with context", func(t *testing.T) {
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

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, func() corerpfake.EnvironmentsServer {
			return corerpfake.EnvironmentsServer{
				Get: func(
					ctx context.Context, rootScope string, environmentName string,
					_ *v20250801preview.EnvironmentsClientGetOptions,
				) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
					errResp.SetError(fmt.Errorf("internal server error"))
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
		require.Error(t, err)
		require.Contains(t, err.Error(), "An error occurred while retrieving environment")
	})

	t.Run("env update returns error — returns error with context", func(t *testing.T) {
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

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, func() corerpfake.EnvironmentsServer {
			return corerpfake.EnvironmentsServer{
				Get: func(
					ctx context.Context, rootScope string, environmentName string,
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
					ctx context.Context, rootScope string, environmentName string,
					resource v20250801preview.EnvironmentResource,
					_ *v20250801preview.EnvironmentsClientCreateOrUpdateOptions,
				) (resp azfake.Responder[v20250801preview.EnvironmentsClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
					errResp.SetError(fmt.Errorf("update failed"))
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
		require.Error(t, err)
		require.Contains(t, err.Error(), "Failed to update environment")
	})

	t.Run("delete pack already deleted", func(t *testing.T) {
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
			Return(false, nil)

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
			output.LogOutput{Format: msgRecipePackNotFound, Params: []any{packName}},
		}, outputSink.Writes)
	})

	t.Run("multiple referenced envs in workspace scope reuse injected factory", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		envName2 := "test-env-2"
		envFullID2 := scope + "/providers/Radius.Core/environments/" + envName2

		appMgmtClient := clients.NewMockApplicationsManagementClient(ctrl)
		appMgmtClient.EXPECT().
			GetRecipePack(gomock.Any(), packName).
			Return(v20250801preview.RecipePackResource{
				ID:   to.Ptr(packFullID),
				Name: to.Ptr(packName),
				Properties: &v20250801preview.RecipePackProperties{
					Recipes:      map[string]*v20250801preview.RecipeDefinition{},
					ReferencedBy: []*string{to.Ptr(envFullID), to.Ptr(envFullID2)},
				},
			}, nil)
		appMgmtClient.EXPECT().
			DeleteRecipePack(gomock.Any(), packName).
			Return(true, nil)

		updatedEnvs := map[string]v20250801preview.EnvironmentResource{}
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, func() corerpfake.EnvironmentsServer {
			return corerpfake.EnvironmentsServer{
				Get: func(
					ctx context.Context, rootScope string, environmentName string,
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
					ctx context.Context, rootScope string, environmentName string,
					resource v20250801preview.EnvironmentResource,
					_ *v20250801preview.EnvironmentsClientCreateOrUpdateOptions,
				) (resp azfake.Responder[v20250801preview.EnvironmentsClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
					updatedEnvs[environmentName] = resource
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

		// Both referenced envs should have been updated using the injected factory,
		// with the deleted pack removed from each env's RecipePacks list.
		require.Len(t, updatedEnvs, 2)
		require.Contains(t, updatedEnvs, envName)
		require.Contains(t, updatedEnvs, envName2)
		require.Empty(t, updatedEnvs[envName].Properties.RecipePacks)
		require.Empty(t, updatedEnvs[envName2].Properties.RecipePacks)
	})

	t.Run("same-named pack in a different scope is preserved (matches by full ID, not suffix)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Another pack with the same name but in a different resource group must
		// not be removed from the environment when we delete this pack.
		otherScopePackID := "/planes/radius/local/resourceGroups/other-group/providers/Radius.Core/recipePacks/" + packName
		// Same ID as packFullID but with mixed-case segments — must still match.
		mixedCasePackID := "/planes/radius/local/resourceGroups/test-group/providers/RADIUS.CORE/recipePacks/" + packName

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
					ctx context.Context, rootScope string, environmentName string,
					_ *v20250801preview.EnvironmentsClientGetOptions,
				) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
					resp.SetResponse(http.StatusOK, v20250801preview.EnvironmentsClientGetResponse{
						EnvironmentResource: v20250801preview.EnvironmentResource{
							Name: to.Ptr(environmentName),
							Properties: &v20250801preview.EnvironmentProperties{
								RecipePacks: []*string{
									to.Ptr(mixedCasePackID),  // should be removed (case-insensitive match)
									to.Ptr(otherScopePackID), // must NOT be removed
								},
							},
						},
					}, nil)
					return
				},
				CreateOrUpdate: func(
					ctx context.Context, rootScope string, environmentName string,
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

		require.Len(t, capturedEnv.Properties.RecipePacks, 1)
		require.Equal(t, otherScopePackID, *capturedEnv.Properties.RecipePacks[0])
	})
}
