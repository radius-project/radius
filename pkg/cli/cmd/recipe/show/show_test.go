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
	"fmt"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/cli/clients"
	types "github.com/radius-project/radius/pkg/cli/cmd/recipe"
	"github.com/radius-project/radius/pkg/cli/cmd/recipe/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	datastoresrp "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller"
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
			Input:         []string{"recipeName", "--resource-type", datastoresrp.RedisCachesResourceType},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with incorrect fallback workspace",
			Input:         []string{"-e", "my-env", "-g", "my-env", "recipeName", "--resource-type", datastoresrp.RedisCachesResourceType},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Show Command with too many positional args",
			Input:         []string{"recipeName", "arg2", "--resource-type", datastoresrp.RedisCachesResourceType},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with fallback workspace",
			Input:         []string{"-e", "my-env", "-w", "test-workspace", "recipeName", "--resource-type", datastoresrp.RedisCachesResourceType},
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
		envRecipe := v20231001preview.RecipeGetMetadataResponse{
			TemplateKind: to.Ptr(recipes.TemplateKindBicep),
			TemplatePath: to.Ptr("ghcr.io/testpublicrecipe/bicep/modules/mongodatabases:v1"),
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
			ResourceType: datastoresrp.MongoDatabasesResourceType,
			TemplateKind: recipes.TemplateKindBicep,
			TemplatePath: "ghcr.io/testpublicrecipe/bicep/modules/mongodatabases:v1",
		}
		recipeParams := []types.RecipeParameter{
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
			GetRecipeMetadata(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(envRecipe, nil).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{},
			Format:            "table",
			RecipeName:        "cosmosDB",
			ResourceType:      datastoresrp.MongoDatabasesResourceType,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipe,
				Options: common.RecipeFormat(),
			},
			output.LogOutput{
				Format: "",
			},
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipeParams,
				Options: common.RecipeParametersFormat(),
			},
		}
		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Show bicep recipe details - Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		envRecipe := v20231001preview.RecipeGetMetadataResponse{
			TemplateKind: to.Ptr(recipes.TemplateKindBicep),
			TemplatePath: to.Ptr("localhost:8000/mongodatabases:v1"),
			PlainHTTP:    to.Ptr(true),
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
			ResourceType: datastoresrp.MongoDatabasesResourceType,
			TemplateKind: recipes.TemplateKindBicep,
			TemplatePath: "localhost:8000/mongodatabases:v1",
			PlainHTTP:    true,
		}
		recipeParams := []types.RecipeParameter{
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
			GetRecipeMetadata(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(envRecipe, nil).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{},
			Format:            "table",
			RecipeName:        "cosmosDB",
			ResourceType:      datastoresrp.MongoDatabasesResourceType,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipe,
				Options: common.RecipeFormat(),
			},
			output.LogOutput{
				Format: "",
			},
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipeParams,
				Options: common.RecipeParametersFormat(),
			},
		}
		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Show terraformn recipe details - Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		envRecipe := v20231001preview.RecipeGetMetadataResponse{
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
			ResourceType:    datastoresrp.MongoDatabasesResourceType,
			TemplateKind:    recipes.TemplateKindTerraform,
			TemplatePath:    "Azure/cosmosdb/azurerm",
			TemplateVersion: "1.1.0",
		}
		recipeParams := []types.RecipeParameter{
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
			GetRecipeMetadata(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(envRecipe, nil).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{},
			Format:            "table",
			RecipeName:        "cosmosDB",
			ResourceType:      datastoresrp.MongoDatabasesResourceType,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipe,
				Options: common.RecipeFormat(),
			},
			output.LogOutput{
				Format: "",
			},
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipeParams,
				Options: common.RecipeParametersFormat(),
			},
		}
		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Show recipe details with environment parameter values - Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		envRecipe := v20231001preview.RecipeGetMetadataResponse{
			TemplateKind: to.Ptr(recipes.TemplateKindBicep),
			TemplatePath: to.Ptr("ghcr.io/testpublicrecipe/bicep/modules/openai:v1"),
			Parameters: map[string]any{
				"location": map[string]any{
					"type":         "string",
					"defaultValue": "null",
				},
				"resource_group_name": map[string]any{
					"type":         "string", 
					"defaultValue": "null",
				},
			},
		}
		recipe := types.EnvironmentRecipe{
			Name:         "default",
			ResourceType: "Radius.Resources/openAI",
			TemplateKind: recipes.TemplateKindBicep,
			TemplatePath: "ghcr.io/testpublicrecipe/bicep/modules/openai:v1",
		}

		// Environment has configured parameter values
		envResource := v20231001preview.EnvironmentResource{
			Properties: &v20231001preview.EnvironmentProperties{
				Recipes: map[string]map[string]v20231001preview.RecipePropertiesClassification{
					"Radius.Resources/openAI": {
						"default": &v20231001preview.BicepRecipeProperties{
							TemplateKind: to.Ptr(recipes.TemplateKindBicep),
							TemplatePath: to.Ptr("ghcr.io/testpublicrecipe/bicep/modules/openai:v1"),
							Parameters: map[string]any{
								"location":            "eastus",
								"resource_group_name": "my-rg",
							},
						},
					},
				},
			},
		}

		// Expected parameters should show environment values, not template defaults
		recipeParams := []types.RecipeParameter{
			{
				Name:         "resource_group_name",
				Type:         "string",
				MaxValue:     "-",
				MinValue:     "-",
				DefaultValue: "my-rg", // Environment value overrides template default
			},
			{
				Name:         "location",
				Type:         "string",
				MaxValue:     "-",
				MinValue:     "-",
				DefaultValue: "eastus", // Environment value overrides template default
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetRecipeMetadata(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(envRecipe, nil).Times(1)

		appManagementClient.EXPECT().
			GetEnvironment(gomock.Any(), gomock.Any()).
			Return(envResource, nil).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{},
			Format:            "table",
			RecipeName:        "default",
			ResourceType:      "Radius.Resources/openAI",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipe,
				Options: common.RecipeFormat(),
			},
			output.LogOutput{
				Format: "",
			},
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipeParams,
				Options: common.RecipeParametersFormat(),
			},
		}
		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Show recipe details when GetEnvironment fails - Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		envRecipe := v20231001preview.RecipeGetMetadataResponse{
			TemplateKind: to.Ptr(recipes.TemplateKindBicep),
			TemplatePath: to.Ptr("ghcr.io/testpublicrecipe/bicep/modules/openai:v1"),
			Parameters: map[string]any{
				"location": map[string]any{
					"type":         "string",
					"defaultValue": "westus",
				},
				"resource_group_name": map[string]any{
					"type":         "string", 
					"defaultValue": "null",
				},
			},
		}
		recipe := types.EnvironmentRecipe{
			Name:         "default",
			ResourceType: "Radius.Resources/openAI",
			TemplateKind: recipes.TemplateKindBicep,
			TemplatePath: "ghcr.io/testpublicrecipe/bicep/modules/openai:v1",
		}

		// Expected parameters should show template defaults since environment call fails
		recipeParams := []types.RecipeParameter{
			{
				Name:         "resource_group_name",
				Type:         "string",
				MaxValue:     "-",
				MinValue:     "-",
				DefaultValue: "null", // Template default since environment fails
			},
			{
				Name:         "location",
				Type:         "string",
				MaxValue:     "-",
				MinValue:     "-",
				DefaultValue: "westus", // Template default since environment fails
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetRecipeMetadata(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(envRecipe, nil).Times(1)

		// Mock GetEnvironment to fail - this should be handled gracefully
		// The environment will be nil, so we get original behavior
		appManagementClient.EXPECT().
			GetEnvironment(gomock.Any(), gomock.Any()).
			Return(v20231001preview.EnvironmentResource{}, fmt.Errorf("environment not found")).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{},
			Format:            "table",
			RecipeName:        "default",
			ResourceType:      "Radius.Resources/openAI",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipe,
				Options: common.RecipeFormat(),
			},
			output.LogOutput{
				Format: "",
			},
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipeParams,
				Options: common.RecipeParametersFormat(),
			},
		}
		require.Equal(t, expected, outputSink.Writes)
	})
}
