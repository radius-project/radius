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

package show

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/radius-project/radius/pkg/cli/clients"
	types "github.com/radius-project/radius/pkg/cli/cmd/recipe"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/to"
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
			Name:          "Valid Show Command",
			Input:         []string{"recipeName", "--resource-type", "resource-type"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with incorrect fallback workspace",
			Input:         []string{"-e", "my-env", "-g", "my-env", "recipeName", "--resource-type", "resource-type"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Show Command with too many positional args",
			Input:         []string{"recipeName", "arg2", "--resource-type", "resource-type"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with fallback workspace",
			Input:         []string{"-e", "my-env", "-w", "test-workspace", "recipeName", "--resource-type", "resource-type"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command without ResourceType",
			Input:         []string{"recipeName"},
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
	t.Run("Show bicep recipe details - Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		envRecipe := v20220315privatepreview.RecipeGetMetadataResponse{
			TemplateKind: to.Ptr(recipes.TemplateKindBicep),
			TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
			Parameters: map[string]any{
				"throughput": map[string]any{
					"type":     "float64",
					"maxValue": float64(800),
				},
				"sku": map[string]any{
					"type": "string",
				},
			},
		}
		recipe := types.EnvironmentRecipe{
			Name:         "cosmosDB",
			ResourceType: portableresources.MongoDatabasesResourceType,
			TemplateKind: recipes.TemplateKindBicep,
			TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
		}
		recipeParams := []RecipeParameter{
			{
				Name:         "throughput",
				Type:         "float64",
				MaxValue:     "800",
				MinValue:     "-",
				DefaultValue: "-",
			},
			{
				Name:         "sku",
				Type:         "string",
				MaxValue:     "-",
				MinValue:     "-",
				DefaultValue: "-",
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			ShowRecipe(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(envRecipe, nil).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{},
			Format:            "table",
			RecipeName:        "cosmosDB",
			ResourceType:      portableresources.MongoDatabasesResourceType,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipe,
				Options: objectformats.GetEnvironmentRecipesTableFormat(),
			},
			output.LogOutput{
				Format: "",
			},
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipeParams,
				Options: objectformats.GetRecipeParamsTableFormat(),
			},
		}
		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Show terraformn recipe details - Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		envRecipe := v20220315privatepreview.RecipeGetMetadataResponse{
			TemplateKind:    to.Ptr(recipes.TemplateKindTerraform),
			TemplatePath:    to.Ptr("Azure/cosmosdb/azurerm"),
			TemplateVersion: to.Ptr("1.1.0"),
			Parameters: map[string]any{
				"throughput": map[string]any{
					"type":     "float64",
					"maxValue": float64(800),
				},
				"sku": map[string]any{
					"type": "string",
				},
			},
		}
		recipe := types.EnvironmentRecipe{
			Name:            "cosmosDB",
			ResourceType:    portableresources.MongoDatabasesResourceType,
			TemplateKind:    recipes.TemplateKindTerraform,
			TemplatePath:    "Azure/cosmosdb/azurerm",
			TemplateVersion: "1.1.0",
		}
		recipeParams := []RecipeParameter{
			{
				Name:         "throughput",
				Type:         "float64",
				MaxValue:     "800",
				MinValue:     "-",
				DefaultValue: "-",
			},
			{
				Name:         "sku",
				Type:         "string",
				MaxValue:     "-",
				MinValue:     "-",
				DefaultValue: "-",
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			ShowRecipe(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(envRecipe, nil).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{},
			Format:            "table",
			RecipeName:        "cosmosDB",
			ResourceType:      portableresources.MongoDatabasesResourceType,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipe,
				Options: objectformats.GetEnvironmentRecipesTableFormat(),
			},
			output.LogOutput{
				Format: "",
			},
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipeParams,
				Options: objectformats.GetRecipeParamsTableFormat(),
			},
		}
		require.Equal(t, expected, outputSink.Writes)
	})
}
