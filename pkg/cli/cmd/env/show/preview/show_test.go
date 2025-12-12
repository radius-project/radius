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
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
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
			Name:          "Show Command with default environment",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with flag",
			Input:         []string{"-e", "test-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with positional arg",
			Input:         []string{"test-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with fallback workspace",
			Input:         []string{"--environment", "test-env", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Show Command with incorrect args",
			Input:         []string{"foo", "bar"},
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
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	testcases := []struct {
		name              string
		envFactory        func() fake.EnvironmentsServer
		recipePackFactory func() fake.RecipePacksServer
		environmentName   string
		expectedOutput    []any
	}{
		{
			name:              "environment with recipe packs",
			envFactory:        test_client_factory.WithEnvironmentServerNoError,
			recipePackFactory: test_client_factory.WithRecipePackServerNoError,
			environmentName:   "env1",
			expectedOutput: []any{
				output.FormattedOutput{
					Format: "table",
					Obj: corerpv20250801.EnvironmentResource{
						Name: to.Ptr("env1"),
						Properties: &corerpv20250801.EnvironmentProperties{
							RecipePacks: []*string{
								to.Ptr("/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/recipePacks/test-recipe-pack"),
							},
						},
					},
					Options: objectformats.GetResourceTableFormat(),
				},
				output.LogOutput{
					Format: "",
				},
				output.FormattedOutput{
					Format: "table",
					Obj: []EnvRecipes{
						{
							RecipePack:     "test-recipe-pack",
							ResourceType:   "test-recipe1",
							RecipeKind:     string(corerpv20250801.RecipeKindTerraform),
							RecipeLocation: "https://example.com/recipe1?ref=v0.1",
						},
						{
							RecipePack:     "test-recipe-pack",
							ResourceType:   "test-recipe2",
							RecipeKind:     string(corerpv20250801.RecipeKindTerraform),
							RecipeLocation: "https://example.com/recipe2?ref=v0.1",
						},
					},
					Options: objectformats.GetRecipesForEnvironmentTableFormat(),
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, tc.envFactory, tc.recipePackFactory)
			require.NoError(t, err)

			outputSink := &output.MockOutput{}
			runner := &Runner{
				RadiusCoreClientFactory: factory,
				Workspace:               workspace,
				EnvironmentName:         tc.environmentName,
				Format:                  "table",
				Output:                  outputSink,
			}

			err = runner.Run(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.expectedOutput, outputSink.Writes)
		})
	}
}

func Test_Run_RecipeSortOrder(t *testing.T) {
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	// Create environment server with multiple recipe packs
	envServer := func() fake.EnvironmentsServer {
		return fake.EnvironmentsServer{
			Get: func(
				ctx context.Context,
				environmentName string,
				options *corerpv20250801.EnvironmentsClientGetOptions,
			) (resp azfake.Responder[corerpv20250801.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
				result := corerpv20250801.EnvironmentsClientGetResponse{
					EnvironmentResource: corerpv20250801.EnvironmentResource{
						Name: to.Ptr(environmentName),
						Properties: &corerpv20250801.EnvironmentProperties{
							RecipePacks: []*string{
								to.Ptr("/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/recipePacks/pack-b"),
								to.Ptr("/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/recipePacks/pack-a"),
							},
						},
					},
				}
				resp.SetResponse(http.StatusOK, result, nil)
				return
			},
		}
	}

	// Create recipe pack server with recipes in non-alphabetical order
	recipePackServer := func() fake.RecipePacksServer {
		return fake.RecipePacksServer{
			Get: func(ctx context.Context, recipePackName string, options *corerpv20250801.RecipePacksClientGetOptions) (resp azfake.Responder[corerpv20250801.RecipePacksClientGetResponse], errResp azfake.ErrorResponder) {
				var recipes map[string]*corerpv20250801.RecipeDefinition
				if recipePackName == "pack-a" {
					recipes = map[string]*corerpv20250801.RecipeDefinition{
						"Applications.Datastores/sqlDatabases": {
							RecipeLocation: to.Ptr("ghcr.io/radius-project/recipes/sql"),
							RecipeKind:     to.Ptr(corerpv20250801.RecipeKindTerraform),
						},
						"Applications.Datastores/redisCaches": {
							RecipeLocation: to.Ptr("ghcr.io/radius-project/recipes/redis"),
							RecipeKind:     to.Ptr(corerpv20250801.RecipeKindTerraform),
						},
					}
				} else {
					recipes = map[string]*corerpv20250801.RecipeDefinition{
						"Applications.Messaging/rabbitMQQueues": {
							RecipeLocation: to.Ptr("ghcr.io/radius-project/recipes/rabbitmq"),
							RecipeKind:     to.Ptr(corerpv20250801.RecipeKindBicep),
						},
						"Applications.Dapr/stateStores": {
							RecipeLocation: to.Ptr("ghcr.io/radius-project/recipes/dapr-state"),
							RecipeKind:     to.Ptr(corerpv20250801.RecipeKindBicep),
						},
					}
				}
				result := corerpv20250801.RecipePacksClientGetResponse{
					RecipePackResource: corerpv20250801.RecipePackResource{
						Name: to.Ptr(recipePackName),
						Properties: &corerpv20250801.RecipePackProperties{
							Recipes: recipes,
						},
					},
				}
				resp.SetResponse(http.StatusOK, result, nil)
				return
			},
		}
	}

	factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, envServer, recipePackServer)
	require.NoError(t, err)

	outputSink := &output.MockOutput{}
	runner := &Runner{
		RadiusCoreClientFactory: factory,
		Workspace:               workspace,
		EnvironmentName:         "test-env",
		Format:                  "table",
		Output:                  outputSink,
	}

	err = runner.Run(context.Background())
	require.NoError(t, err)

	// Verify the recipes are sorted by RecipePack first, then by ResourceType
	expectedRecipes := []EnvRecipes{
		{RecipePack: "pack-a", ResourceType: "Applications.Datastores/redisCaches", RecipeKind: "terraform", RecipeLocation: "ghcr.io/radius-project/recipes/redis"},
		{RecipePack: "pack-a", ResourceType: "Applications.Datastores/sqlDatabases", RecipeKind: "terraform", RecipeLocation: "ghcr.io/radius-project/recipes/sql"},
		{RecipePack: "pack-b", ResourceType: "Applications.Dapr/stateStores", RecipeKind: "bicep", RecipeLocation: "ghcr.io/radius-project/recipes/dapr-state"},
		{RecipePack: "pack-b", ResourceType: "Applications.Messaging/rabbitMQQueues", RecipeKind: "bicep", RecipeLocation: "ghcr.io/radius-project/recipes/rabbitmq"},
	}

	// The third output should be the recipes table
	require.Len(t, outputSink.Writes, 3)
	formattedOutput, ok := outputSink.Writes[2].(output.FormattedOutput)
	require.True(t, ok, "expected FormattedOutput")
	require.Equal(t, expectedRecipes, formattedOutput.Obj)
}
