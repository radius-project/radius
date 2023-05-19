// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/project-radius/radius/pkg/cli/clients"
	types "github.com/project-radius/radius/pkg/cli/cmd/recipe"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/test/radcli"
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
			Input:         []string{"recipeName", "--link-type", "link-type"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with incorrect fallback workspace",
			Input:         []string{"-e", "my-env", "-g", "my-env", "recipeName", "--link-type", "link-type"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Show Command with too many positional args",
			Input:         []string{"recipeName", "arg2", "--link-type", "link-type"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with fallback workspace",
			Input:         []string{"-e", "my-env", "-w", "test-workspace", "recipeName", "--link-type", "link-type"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command without LinkType",
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
	t.Run("Show recipes details - Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		envRecipe := v20220315privatepreview.EnvironmentRecipeProperties{
			TemplateKind: to.Ptr(types.TemplateKindBicep),
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
			LinkType:     linkrp.MongoDatabasesResourceType,
			TemplateKind: types.TemplateKindBicep,
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
			LinkType:          linkrp.MongoDatabasesResourceType,
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

	t.Run("Show recipe details - empty template kind", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		envRecipe := v20220315privatepreview.EnvironmentRecipeProperties{
			TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
			Parameters: map[string]any{
				"throughput": map[string]any{
					"type":     "float64",
					"maxValue": float64(800),
				},
			},
		}
		recipe := types.EnvironmentRecipe{
			Name:         "cosmosDB",
			LinkType:     linkrp.MongoDatabasesResourceType,
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
			LinkType:          linkrp.MongoDatabasesResourceType,
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
