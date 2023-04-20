// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package unregister

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/corerp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid Unregister Command",
			Input:         []string{"--name", "test_recipe"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Unregister Command with fallback workspace",
			Input:         []string{"--name", "test_recipe"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Unregister Command without name",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Unregister Command with too many args",
			Input:         []string{"--name", "foo", "bar", "foo1"},
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
	t.Run("Unregister recipe from the environment", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			testEnvProperties := &v20230415preview.EnvironmentProperties{
				UseDevRecipes: to.Ptr(true),
				Recipes: map[string]*v20230415preview.EnvironmentRecipeProperties{
					"cosmosDB": {
						LinkType:     to.Ptr(linkrp.MongoDatabasesResourceType),
						TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
					},
				},
				Compute: &v20230415preview.KubernetesCompute{
					Namespace: to.Ptr("default"),
				},
			}

			envResource := v20230415preview.EnvironmentResource{
				ID:         to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
				Name:       to.Ptr("kind-kind"),
				Type:       to.Ptr("applications.core/environments"),
				Location:   to.Ptr(v1.LocationGlobal),
				Properties: testEnvProperties,
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetEnvDetails(gomock.Any(), gomock.Any()).
				Return(envResource, nil).Times(1)
			appManagementClient.EXPECT().
				CreateEnvironment(context.Background(), "kind-kind", v1.LocationGlobal, testEnvProperties).
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
		t.Run("No Namespace", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			testEnvProperties := &v20230415preview.EnvironmentProperties{
				UseDevRecipes: to.Ptr(true),
				Recipes: map[string]*v20230415preview.EnvironmentRecipeProperties{
					"cosmosDB": {
						LinkType:     to.Ptr(linkrp.MongoDatabasesResourceType),
						TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
					},
				},
			}

			envResource := v20230415preview.EnvironmentResource{
				ID:         to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
				Name:       to.Ptr("kind-kind"),
				Type:       to.Ptr("applications.core/environments"),
				Location:   to.Ptr(v1.LocationGlobal),
				Properties: testEnvProperties,
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetEnvDetails(gomock.Any(), gomock.Any()).
				Return(envResource, nil).Times(1)
			appManagementClient.EXPECT().
				CreateEnvironment(context.Background(), "kind-kind", v1.LocationGlobal, testEnvProperties).
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
		t.Run("Unregister recipe that doesn't exist in the environment", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			envResource := v20230415preview.EnvironmentResource{
				ID:       to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
				Name:     to.Ptr("kind-kind"),
				Type:     to.Ptr("applications.core/environments"),
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &v20230415preview.EnvironmentProperties{
					UseDevRecipes: to.Ptr(true),
					Recipes: map[string]*v20230415preview.EnvironmentRecipeProperties{
						"cosmosDB": {
							LinkType:     to.Ptr(linkrp.MongoDatabasesResourceType),
							TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
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
		t.Run("Unregister recipe with no recipes added to the environment", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			envResource := v20230415preview.EnvironmentResource{
				ID:       to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
				Name:     to.Ptr("kind-kind"),
				Type:     to.Ptr("applications.core/environments"),
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &v20230415preview.EnvironmentProperties{
					UseDevRecipes: to.Ptr(true),
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
