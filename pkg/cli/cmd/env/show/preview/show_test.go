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
	"testing"

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
