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
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/corerp/api/v20230415preview"
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
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "List Command with incorrect fallback workspace",
			Input:         []string{"-e", "my-env", "-g", "my-env", "--name", "recipeName"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "List Command with just recipe name",
			Input:         []string{"recipeName"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "List Command with fallback workspace",
			Input:         []string{"-e", "my-env", "-w", "test-workspace", "--name", "recipeName"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "List Command with fallback workspace and name without flag",
			Input:         []string{"-e", "my-env", "-w", "test-workspace", "recipeName"},
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
	t.Run("Show recipes details", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			envRecipes := v20230415preview.EnvironmentRecipeProperties{
				LinkType:     to.Ptr("Applications.Link/mongoDatabases"),
				TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
				Parameters: map[string]any{
					"throughput": map[string]any{
						"type": "float64",
						"max":  float64(800),
					},
					"sku": map[string]any{
						"type": "string",
					},
				},
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				ShowRecipe(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(envRecipes, nil).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{},
				Format:            "table",
				RecipeName:        "cosmosDB",
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)
			output := outputSink.Writes[0].(output.FormattedOutput)
			skuType := false
			throughputType := false
			throughputMax := false
			outputParams := output.Obj.([]EnvironmentRecipe)
			require.Equal(t, 3, len(outputParams))
			for i, envRecipeObj := range output.Obj.([]EnvironmentRecipe) {
				if i == 0 {
					require.Equal(t, "cosmosDB", envRecipeObj.RecipeName)
					require.Equal(t, "Applications.Link/mongoDatabases", envRecipeObj.LinkType)
					require.Equal(t, "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1", envRecipeObj.TemplatePath)
				} else {
					require.Equal(t, "", envRecipeObj.RecipeName)
					require.Equal(t, "", envRecipeObj.LinkType)
					require.Equal(t, "", envRecipeObj.TemplatePath)
				}
				if envRecipeObj.ParameterName == "sku" && envRecipeObj.ParameterDetailName == "type" {
					require.Equal(t, "string", envRecipeObj.ParameterDetailValue)
					skuType = true
				}

				if envRecipeObj.ParameterName == "throughput" {
					if envRecipeObj.ParameterDetailName == "type" {
						require.Equal(t, "float64", envRecipeObj.ParameterDetailValue)
						throughputType = true
					}
					if envRecipeObj.ParameterDetailName == "max" {
						require.Equal(t, float64(800), envRecipeObj.ParameterDetailValue)
						throughputMax = true
					}
				}

				if envRecipeObj.ParameterName == "" {
					if envRecipeObj.ParameterDetailName == "type" && !throughputType {
						require.Equal(t, "float64", envRecipeObj.ParameterDetailValue)
						throughputType = true
					}
					if envRecipeObj.ParameterDetailName == "max" && !throughputMax {
						require.Equal(t, float64(800), envRecipeObj.ParameterDetailValue)
						throughputMax = true
					}
				}
			}
			require.True(t, skuType)
			require.True(t, throughputType)
			require.True(t, throughputMax)
		})
	})
}
