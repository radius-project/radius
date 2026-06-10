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
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	corerpfake "github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/radcli"
	"github.com/spf13/cobra"
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

func Test_ValidateRecipePackParsing(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	testcases := []struct {
		name          string
		input         []string
		expectedPacks []string
	}{
		{
			name:          "single recipe pack",
			input:         []string{"test-env", "--recipe-packs", "pack1"},
			expectedPacks: []string{"pack1"},
		},
		{
			name:          "comma-separated recipe packs",
			input:         []string{"test-env", "--recipe-packs", "pack1,pack2"},
			expectedPacks: []string{"pack1", "pack2"},
		},
		{
			name:          "comma-separated with spaces",
			input:         []string{"test-env", "--recipe-packs", "pack1, pack2 , pack3"},
			expectedPacks: []string{"pack1", "pack2", "pack3"},
		},
		{
			name:          "mixed simple names and full resource IDs",
			input:         []string{"test-env", "--recipe-packs", "demorecipepack,/planes/radius/local/resourcegroups/kind-radius/providers/Radius.Core/recipePacks/kuberecipepack2"},
			expectedPacks: []string{"demorecipepack", "/planes/radius/local/resourcegroups/kind-radius/providers/Radius.Core/recipePacks/kuberecipepack2"},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			// Create command
			cmd := &cobra.Command{}
			// Validate uses cli.RequireWorkspace / RequireScope,
			// which expect these flags to be defined on the command.
			commonflags.AddWorkspaceFlag(cmd)
			commonflags.AddResourceGroupFlag(cmd)
			commonflags.AddEnvironmentNameFlag(cmd)
			cmd.Flags().Bool(commonflags.ClearEnvAzureFlag, false, "")
			cmd.Flags().Bool(commonflags.ClearEnvAWSFlag, false, "")
			cmd.Flags().Bool(commonflags.ClearEnvKubernetesFlag, false, "")
			cmd.Flags().StringSliceP("recipe-packs", "", []string{}, "Specify recipe packs to be added to the environment (--preview)")

			// Parse flags
			err := cmd.Flags().Parse(tc.input)
			require.NoError(t, err)

			// Create and validate runner
			runner := &Runner{
				ConfigHolder: &framework.ConfigHolder{
					ConfigFilePath: "",
					Config:         configWithWorkspace,
				},
				Output:    &output.MockOutput{},
				Workspace: workspace,
			}

			err = runner.Validate(cmd, []string{"test-env"})
			require.NoError(t, err)

			require.Equal(t, tc.expectedPacks, runner.recipePacks, "recipe packs mismatch for test: %s", tc.name)
		})
	}
}

func Test_normalizeRecipePacks(t *testing.T) {
	testcases := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: []string{},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "single value",
			input:    []string{"pack1"},
			expected: []string{"pack1"},
		},
		{
			name:     "comma-separated values",
			input:    []string{"pack1,pack2,pack3"},
			expected: []string{"pack1", "pack2", "pack3"},
		},
		{
			name:     "trims whitespace",
			input:    []string{" pack1 , pack2 ,  pack3"},
			expected: []string{"pack1", "pack2", "pack3"},
		},
		{
			name:     "drops empty entries",
			input:    []string{"pack1,,pack2", "", " , "},
			expected: []string{"pack1", "pack2"},
		},
		{
			name:     "deduplicates repeated flags",
			input:    []string{"pack1", "pack1"},
			expected: []string{"pack1"},
		},
		{
			name:     "deduplicates within comma list",
			input:    []string{"pack1,pack1,pack2"},
			expected: []string{"pack1", "pack2"},
		},
		{
			name:     "deduplicates across mixed sources preserving order",
			input:    []string{"pack2", "pack1,pack2", " pack1 ", "pack3"},
			expected: []string{"pack2", "pack1", "pack3"},
		},
		{
			name:     "treats whitespace-only difference as duplicate",
			input:    []string{"pack1", " pack1 "},
			expected: []string{"pack1"},
		},
		{
			name:     "preserves full resource ID and dedupes",
			input:    []string{"/planes/radius/local/resourcegroups/g/providers/Radius.Core/recipePacks/p1,/planes/radius/local/resourcegroups/g/providers/Radius.Core/recipePacks/p1"},
			expected: []string{"/planes/radius/local/resourcegroups/g/providers/Radius.Core/recipePacks/p1"},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, normalizeRecipePacks(tc.input))
		})
	}
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
					Format: "WARNING: The existing recipe pack list will be replaced with the specified packs.",
				},
				output.LogOutput{
					Format: "Radius.Core/environments/%s updated",
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

func Test_Run_RecipePacksReplaced(t *testing.T) {
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	// Track the resource sent to CreateOrUpdate so we can assert on its recipe packs.
	var capturedEnv v20250801preview.EnvironmentResource

	existingPackID := "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/recipePacks/old-pack"

	envServer := func() fake.EnvironmentsServer {
		return fake.EnvironmentsServer{
			Get: func(
				_ context.Context,
				environmentName string,
				_ *v20250801preview.EnvironmentsClientGetOptions,
			) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
				result := v20250801preview.EnvironmentsClientGetResponse{
					EnvironmentResource: v20250801preview.EnvironmentResource{
						ID:   to.Ptr(workspace.Scope + "/providers/Radius.Core/environments/" + environmentName),
						Name: to.Ptr(environmentName),
						Properties: &v20250801preview.EnvironmentProperties{
							RecipePacks: []*string{to.Ptr(existingPackID)},
						},
					},
				}
				resp.SetResponse(http.StatusOK, result, nil)
				return
			},
			CreateOrUpdate: func(
				_ context.Context,
				_ string,
				resource v20250801preview.EnvironmentResource,
				_ *v20250801preview.EnvironmentsClientCreateOrUpdateOptions,
			) (resp azfake.Responder[v20250801preview.EnvironmentsClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
				capturedEnv = resource
				result := v20250801preview.EnvironmentsClientCreateOrUpdateResponse{
					EnvironmentResource: resource,
				}
				resp.SetResponse(http.StatusOK, result, nil)
				return
			},
		}
	}

	factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
		workspace.Scope,
		envServer,
		nil, // uses default RecipePacksServer (accepts any name)
	)
	require.NoError(t, err)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		ConfigHolder:            &framework.ConfigHolder{},
		Output:                  outputSink,
		Workspace:               workspace,
		EnvironmentName:         "test-env",
		RadiusCoreClientFactory: factory,
		recipePacks:             []string{"new-pack-a", "new-pack-b"},
		providers:               &v20250801preview.Providers{},
	}

	err = runner.Run(context.Background())
	require.NoError(t, err)

	// The old pack should be gone — only the two new packs should remain.
	require.Len(t, capturedEnv.Properties.RecipePacks, 2, "recipe packs list should be replaced, not appended")
	packIDs := []string{*capturedEnv.Properties.RecipePacks[0], *capturedEnv.Properties.RecipePacks[1]}
	require.NotContains(t, packIDs, existingPackID, "old pack should not be in the updated list")

	// Verify the replacement warning was emitted.
	foundWarning := false
	for _, w := range outputSink.Writes {
		if logOut, ok := w.(output.LogOutput); ok && logOut.Format == "WARNING: The existing recipe pack list will be replaced with the specified packs." {
			foundWarning = true
			break
		}
	}
	require.True(t, foundWarning, "expected replacement warning in output")
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

		newPackIDs := []*string{to.Ptr(pack1FullID)}

		err = syncRecipePackReferences(context.Background(), envID, nil, newPackIDs, workspace, factory)
		require.NoError(t, err)
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

		err = syncRecipePackReferences(context.Background(), envID, oldPackIDs, nil, workspace, factory)
		require.NoError(t, err)
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
		oldPackIDs := []*string{to.Ptr(pack1FullID), to.Ptr(pack2FullID)}
		newPackIDs := []*string{to.Ptr(pack1FullID), to.Ptr(pack3FullID)}

		err = syncRecipePackReferences(context.Background(), envID, oldPackIDs, newPackIDs, workspace, factory)
		require.NoError(t, err)

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

		err = syncRecipePackReferences(context.Background(), envID, oldPackIDs, nil, workspace, factory)
		require.NoError(t, err)
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
