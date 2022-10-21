// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package delete

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
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
	configWithoutWorkspace := radcli.LoadConfigWithoutWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid Delete Command",
			Input:         []string{"--name", "test_recipe"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete Command without name",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithoutWorkspace,
			},
		},
		{
			Name:          "Delete Command with too many args",
			Input:         []string{"foo", "bar", "foo1"},
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
	t.Run("Delete recipe from the environment", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			envResource := v20220315privatepreview.EnvironmentResource{
				ID:       to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
				Name:     to.Ptr("kind-kind"),
				Type:     to.Ptr("applications.core/environments"),
				Location: to.Ptr("global"),
				Properties: &v20220315privatepreview.EnvironmentProperties{
					UseRadiusOwnedRecipes: to.Ptr(true),
					Recipes: map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
						"cosmosDB": {
							ConnectorType: to.Ptr("Applications.Connector/mongoDatabases"),
							TemplatePath:  to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
						},
					},
				},
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetEnvDetails(gomock.Any(), gomock.Any()).
				Return(envResource, nil).Times(1)
			appManagementClient.EXPECT().
				CreateEnvironment(context.Background(), "kind-kind", "global", "default", "Kubernetes", gomock.Any(), map[string]*v20220315privatepreview.EnvironmentRecipeProperties{}, gomock.Any(), gomock.Any()).
				Return(true, nil).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
				RecipeName:        "cosmosDB",
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)
		})
		t.Run("Delete recipe that doesn't exist in the environment", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			envResource := v20220315privatepreview.EnvironmentResource{
				ID:       to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
				Name:     to.Ptr("kind-kind"),
				Type:     to.Ptr("applications.core/environments"),
				Location: to.Ptr("global"),
				Properties: &v20220315privatepreview.EnvironmentProperties{
					UseRadiusOwnedRecipes: to.Ptr(true),
					Recipes: map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
						"cosmosDB": {
							ConnectorType: to.Ptr("Applications.Connector/mongoDatabases"),
							TemplatePath:  to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
						},
					},
				},
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetEnvDetails(gomock.Any(), gomock.Any()).
				Return(envResource, nil).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
				RecipeName:        "cosmosDB1",
			}

			err := runner.Run(context.Background())
			require.Error(t, err)
		})
		t.Run("Delete recipe with no recipes added to the environment", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			envResource := v20220315privatepreview.EnvironmentResource{
				ID:       to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
				Name:     to.Ptr("kind-kind"),
				Type:     to.Ptr("applications.core/environments"),
				Location: to.Ptr("global"),
				Properties: &v20220315privatepreview.EnvironmentProperties{
					UseRadiusOwnedRecipes: to.Ptr(true),
				},
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetEnvDetails(gomock.Any(), gomock.Any()).
				Return(envResource, nil).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
				RecipeName:        "cosmosDB",
			}

			err := runner.Run(context.Background())
			require.Error(t, err)
		})
	})
}
