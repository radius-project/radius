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
	"net/http"
	"testing"

	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	v20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	corerpfake "github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
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
			Name:          "Update Env Command without any flags",
			Input:         []string{"default"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Update Env Command without env arg",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Update Env Command with invalid Azure subscriptionId arg",
			Input:         []string{"default", "--azure-subscription-id", "subscriptionName", "--azure-resource-group", "testResourceGroup"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Update Env Command with single provider set",
			Input:         []string{"default", "--azure-subscription-id", "00000000-0000-0000-0000-000000000000", "--azure-resource-group", "testResourceGroup"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name: "Update Env Command with all providers set",
			Input: []string{"default", "--azure-subscription-id", "00000000-0000-0000-0000-000000000000", "--azure-resource-group", "testResourceGroup",
				"--aws-region", "us-west-2", "--aws-account-id", "testAWSAccount", "--kubernetes-namespace", "testNamespace",
			},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	// envServerWithID returns an environment that includes the full resource ID,
	// which is required for the referencedBy sync in env update.
	envServerWithID := func() corerpfake.EnvironmentsServer {
		return corerpfake.EnvironmentsServer{
			CreateOrUpdate: test_client_factory.WithEnvironmentServerNoError().CreateOrUpdate,
			Get: func(
				ctx context.Context,
				environmentName string,
				options *v20250801preview.EnvironmentsClientGetOptions,
			) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
				result := v20250801preview.EnvironmentsClientGetResponse{
					EnvironmentResource: v20250801preview.EnvironmentResource{
						ID:   to.Ptr(workspace.Scope + "/providers/Radius.Core/environments/" + environmentName),
						Name: to.Ptr(environmentName),
						Properties: &v20250801preview.EnvironmentProperties{
							Providers: &v20250801preview.Providers{
								Azure: &v20250801preview.ProvidersAzure{
									SubscriptionID:    to.Ptr("test-subscription-id"),
									ResourceGroupName: to.Ptr("test-resource-group"),
								},
								Aws: &v20250801preview.ProvidersAws{
									AccountID: to.Ptr("test-account-id"),
									Region:    to.Ptr("test-region"),
								},
								Kubernetes: &v20250801preview.ProvidersKubernetes{
									Namespace: to.Ptr("test-namespace"),
								},
							},
							RecipePacks: []*string{
								to.Ptr(workspace.Scope + "/providers/Radius.Core/recipePacks/old-pack"),
							},
						},
					},
				}
				resp.SetResponse(http.StatusOK, result, nil)
				return
			},
		}
	}

	// recipePackServerWithCreateOrUpdate handles both Get (validation) and
	// CreateOrUpdate (referencedBy sync) calls made during env update.
	recipePackServerWithCreateOrUpdate := func() corerpfake.RecipePacksServer {
		return corerpfake.RecipePacksServer{
			Get:            test_client_factory.WithRecipePackServerNoError().Get,
			CreateOrUpdate: recipePackCreateOrUpdateNoError(),
		}
	}

	testcases := []struct {
		name           string
		envName        string
		expectedOutput []any
	}{
		{
			name:    "update environment with azure provider and recipe packs",
			envName: "test-env",
			expectedOutput: []any{
				output.LogOutput{
					Format: "Updating Environment...",
				},
				output.FormattedOutput{
					Format: "table",
					Obj: environmentForDisplay{
						Name:        "test-env",
						RecipePacks: 2, // rp1 and rp2 replace the old pack
						Providers:   3,
					},
					Options: environmentFormat(),
				},
				output.LogOutput{
					Format: "Successfully updated environment %q.",
					Params: []any{"test-env"},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
				workspace.Scope,
				envServerWithID,
				recipePackServerWithCreateOrUpdate,
			)
			require.NoError(t, err)

			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConfigHolder:            &framework.ConfigHolder{},
				Output:                  outputSink,
				Workspace:               workspace,
				EnvironmentName:         tc.envName,
				RadiusCoreClientFactory: factory,
				recipePacks:             []string{"rp1", "rp2"},
				providers: &v20250801preview.Providers{
					Azure: &v20250801preview.ProvidersAzure{
						SubscriptionID:    to.Ptr("00000000-0000-0000-0000-000000000000"),
						ResourceGroupName: to.Ptr("testResourceGroup"),
					},
					Aws: &v20250801preview.ProvidersAws{
						Region:    to.Ptr("us-west-2"),
						AccountID: to.Ptr("testAWSAccount"),
					},
					Kubernetes: &v20250801preview.ProvidersKubernetes{
						Namespace: to.Ptr("test-namespace"),
					},
				},
			}

			err = runner.Run(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.expectedOutput, outputSink.Writes)
		})
	}
}

func Test_syncRecipePackReferences(t *testing.T) {
	scope := "/planes/radius/local/resourceGroups/test-group"
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: scope,
	}
	envID := scope + "/providers/Radius.Core/environments/test-env"

	pack1FullID := scope + "/providers/Radius.Core/recipePacks/pack1"
	pack2FullID := scope + "/providers/Radius.Core/recipePacks/pack2"
	pack3FullID := scope + "/providers/Radius.Core/recipePacks/pack3"

	t.Run("newly added packs get env added to referencedBy", func(t *testing.T) {
		var capturedReferencedBy []*string

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, nil, func() corerpfake.RecipePacksServer {
			return corerpfake.RecipePacksServer{
				Get: test_client_factory.WithRecipePackServerNoError().Get,
				CreateOrUpdate: func(
					ctx context.Context, name string,
					resource v20250801preview.RecipePackResource,
					_ *v20250801preview.RecipePacksClientCreateOrUpdateOptions,
				) (resp azfake.Responder[v20250801preview.RecipePacksClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
					capturedReferencedBy = resource.Properties.ReferencedBy
					resp.SetResponse(http.StatusOK, v20250801preview.RecipePacksClientCreateOrUpdateResponse{RecipePackResource: resource}, nil)
					return
				},
			}
		})
		require.NoError(t, err)

		id1, err := resources.Parse(pack1FullID)
		require.NoError(t, err)

		newPacks := []resolvedPack{
			{
				id:     id1,
				client: factory.NewRecipePacksClient(),
				pack: v20250801preview.RecipePackResource{
					Name: to.Ptr("pack1"),
					Properties: &v20250801preview.RecipePackProperties{
						Recipes:      map[string]*v20250801preview.RecipeDefinition{},
						ReferencedBy: []*string{},
					},
				},
			},
		}

		newPackIDs, err := syncRecipePackReferences(context.Background(), envID, nil, newPacks, workspace, factory)
		require.NoError(t, err)
		require.Len(t, newPackIDs, 1)
		require.Equal(t, pack1FullID, *newPackIDs[0])
		require.Len(t, capturedReferencedBy, 1)
		require.Equal(t, envID, *capturedReferencedBy[0])
	})

	t.Run("dropped packs get env removed from referencedBy", func(t *testing.T) {
		var capturedReferencedBy []*string

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, nil, func() corerpfake.RecipePacksServer {
			return corerpfake.RecipePacksServer{
				Get: func(
					ctx context.Context, name string,
					_ *v20250801preview.RecipePacksClientGetOptions,
				) (resp azfake.Responder[v20250801preview.RecipePacksClientGetResponse], errResp azfake.ErrorResponder) {
					// Pack has both this env and another env in referencedBy.
					result := v20250801preview.RecipePacksClientGetResponse{
						RecipePackResource: v20250801preview.RecipePackResource{
							Name: to.Ptr(name),
							Properties: &v20250801preview.RecipePackProperties{
								Recipes:      map[string]*v20250801preview.RecipeDefinition{},
								ReferencedBy: []*string{to.Ptr("some-other-env"), to.Ptr(envID)},
							},
						},
					}
					resp.SetResponse(http.StatusOK, result, nil)
					return
				},
				CreateOrUpdate: func(
					ctx context.Context, name string,
					resource v20250801preview.RecipePackResource,
					_ *v20250801preview.RecipePacksClientCreateOrUpdateOptions,
				) (resp azfake.Responder[v20250801preview.RecipePacksClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
					capturedReferencedBy = resource.Properties.ReferencedBy
					resp.SetResponse(http.StatusOK, v20250801preview.RecipePacksClientCreateOrUpdateResponse{RecipePackResource: resource}, nil)
					return
				},
			}
		})
		require.NoError(t, err)

		oldPackIDs := []*string{to.Ptr(pack1FullID)}

		newPackIDs, err := syncRecipePackReferences(context.Background(), envID, oldPackIDs, nil, workspace, factory)
		require.NoError(t, err)
		require.Empty(t, newPackIDs)
		// This env is removed; the other env remains.
		require.Len(t, capturedReferencedBy, 1)
		require.Equal(t, "some-other-env", *capturedReferencedBy[0])
	})

	t.Run("unchanged packs are not updated", func(t *testing.T) {
		createOrUpdateCalled := map[string]bool{}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, nil, func() corerpfake.RecipePacksServer {
			return corerpfake.RecipePacksServer{
				Get: func(
					ctx context.Context, name string,
					_ *v20250801preview.RecipePacksClientGetOptions,
				) (resp azfake.Responder[v20250801preview.RecipePacksClientGetResponse], errResp azfake.ErrorResponder) {
					result := v20250801preview.RecipePacksClientGetResponse{
						RecipePackResource: v20250801preview.RecipePackResource{
							Name: to.Ptr(name),
							Properties: &v20250801preview.RecipePackProperties{
								Recipes:      map[string]*v20250801preview.RecipeDefinition{},
								ReferencedBy: []*string{to.Ptr(envID)},
							},
						},
					}
					resp.SetResponse(http.StatusOK, result, nil)
					return
				},
				CreateOrUpdate: func(
					ctx context.Context, name string,
					resource v20250801preview.RecipePackResource,
					_ *v20250801preview.RecipePacksClientCreateOrUpdateOptions,
				) (resp azfake.Responder[v20250801preview.RecipePacksClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
					createOrUpdateCalled[name] = true
					resp.SetResponse(http.StatusOK, v20250801preview.RecipePacksClientCreateOrUpdateResponse{RecipePackResource: resource}, nil)
					return
				},
			}
		})
		require.NoError(t, err)

		// old: [pack1, pack2], new: [pack1, pack3]
		// pack1 unchanged — no update; pack2 dropped — updated; pack3 added — updated.
		id1, err := resources.Parse(pack1FullID)
		require.NoError(t, err)
		id3, err := resources.Parse(pack3FullID)
		require.NoError(t, err)

		oldPackIDs := []*string{to.Ptr(pack1FullID), to.Ptr(pack2FullID)}
		newPacks := []resolvedPack{
			{
				id:     id1,
				client: factory.NewRecipePacksClient(),
				pack: v20250801preview.RecipePackResource{
					Name: to.Ptr("pack1"),
					Properties: &v20250801preview.RecipePackProperties{
						Recipes:      map[string]*v20250801preview.RecipeDefinition{},
						ReferencedBy: []*string{to.Ptr(envID)},
					},
				},
			},
			{
				id:     id3,
				client: factory.NewRecipePacksClient(),
				pack: v20250801preview.RecipePackResource{
					Name: to.Ptr("pack3"),
					Properties: &v20250801preview.RecipePackProperties{
						Recipes:      map[string]*v20250801preview.RecipeDefinition{},
						ReferencedBy: []*string{},
					},
				},
			},
		}

		newPackIDs, err := syncRecipePackReferences(context.Background(), envID, oldPackIDs, newPacks, workspace, factory)
		require.NoError(t, err)
		require.Len(t, newPackIDs, 2)

		require.False(t, createOrUpdateCalled["pack1"], "unchanged pack should not be updated")
		require.True(t, createOrUpdateCalled["pack2"], "dropped pack should be updated")
		require.True(t, createOrUpdateCalled["pack3"], "newly added pack should be updated")
	})

	t.Run("dropped pack already deleted is skipped without error", func(t *testing.T) {
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, nil, func() corerpfake.RecipePacksServer {
			return corerpfake.RecipePacksServer{
				Get: func(
					ctx context.Context, name string,
					_ *v20250801preview.RecipePacksClientGetOptions,
				) (resp azfake.Responder[v20250801preview.RecipePacksClientGetResponse], errResp azfake.ErrorResponder) {
					errResp.SetResponseError(404, "Not Found")
					return
				},
			}
		})
		require.NoError(t, err)

		oldPackIDs := []*string{to.Ptr(pack1FullID)}

		newPackIDs, err := syncRecipePackReferences(context.Background(), envID, oldPackIDs, nil, workspace, factory)
		require.NoError(t, err)
		require.Empty(t, newPackIDs)
	})
}

// recipePackCreateOrUpdateNoError returns a CreateOrUpdate handler that echoes the resource back.
func recipePackCreateOrUpdateNoError() func(
	ctx context.Context,
	recipePackName string,
	resource v20250801preview.RecipePackResource,
	options *v20250801preview.RecipePacksClientCreateOrUpdateOptions,
) (azfake.Responder[v20250801preview.RecipePacksClientCreateOrUpdateResponse], azfake.ErrorResponder) {
	return func(
		ctx context.Context,
		recipePackName string,
		resource v20250801preview.RecipePackResource,
		options *v20250801preview.RecipePacksClientCreateOrUpdateOptions,
	) (resp azfake.Responder[v20250801preview.RecipePacksClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
		resp.SetResponse(http.StatusOK, v20250801preview.RecipePacksClientCreateOrUpdateResponse{RecipePackResource: resource}, nil)
		return
	}
}
