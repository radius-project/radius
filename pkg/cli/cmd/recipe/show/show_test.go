// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"context"
	"testing"

	az_to "github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
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
	t.Run("List recipes linked to the environment", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			envRecipes := v20220315privatepreview.EnvironmentRecipeProperties{
				LinkType:     to.StringPtr("Applications.Link/mongoDatabases"),
				TemplatePath: to.StringPtr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
				Parameters: map[string]interface{}{
					"throughput": "int (max: 800)",
					"sku":        "string",
				},
			}

			envResource := v20220315privatepreview.EnvironmentResource{
				ID:       az_to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
				Name:     az_to.Ptr("kind-kind"),
				Type:     az_to.Ptr("applications.core/environments"),
				Location: az_to.Ptr(v1.LocationGlobal),
				Properties: &v20220315privatepreview.EnvironmentProperties{
					UseDevRecipes: az_to.Ptr(true),
					Recipes: map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
						"cosmosDB": &envRecipes,
					},
					Compute: &v20220315privatepreview.KubernetesCompute{
						Namespace: az_to.Ptr("default"),
					},
				},
			}

			recipes := []EnvironmentRecipe{
				{
					RecipeName:       "cosmosDB",
					LinkType:         "Applications.Link/mongoDatabases",
					TemplatePath:     "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
					ParameterName:    "sku",
					ParameterDetails: "string",
				},
				{
					ParameterName:    "throughput",
					ParameterDetails: "int (max: 800)",
				},
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetEnvDetails(gomock.Any(), gomock.Any()).
				Return(envResource, nil).Times(1)
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

			expected := []any{
				output.FormattedOutput{
					Format:  "table",
					Obj:     recipes,
					Options: objectformats.GetRecipeParamsTableFormats(),
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})
	})
}
