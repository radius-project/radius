// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package register

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
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
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
			Name:          "Valid Register Command with parameters",
			Input:         []string{"--name", "test_recipe", "--template-path", "test_template", "--link-type", linkrp.MongoDatabasesResourceType, "--parameters", "a=b"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid Register Command with parameters passed as file",
			Input:         []string{"--name", "test_recipe", "--template-path", "test_template", "--link-type", linkrp.MongoDatabasesResourceType, "--parameters", "@testdata/recipeparam.json"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Register Command with fallback workspace",
			Input:         []string{"--name", "test_recipe", "--template-path", "test_template", "--link-type", linkrp.MongoDatabasesResourceType},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Register Command without name",
			Input:         []string{"--template-path", "test_template", "--link-type", linkrp.MongoDatabasesResourceType},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Register Command without template path",
			Input:         []string{"--name", "test_recipe", "--link-type", linkrp.MongoDatabasesResourceType},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Register Command without link-type",
			Input:         []string{"--name", "test_recipe", "--template-path", "test_template"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "List Command with too many args",
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
	t.Run("Register recipe Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		envResource := v20220315privatepreview.EnvironmentResource{
			ID:       to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
			Name:     to.Ptr("kind-kind"),
			Type:     to.Ptr("applications.core/environments"),
			Location: to.Ptr(v1.LocationGlobal),
			Properties: &v20220315privatepreview.EnvironmentProperties{
				UseDevRecipes: to.Ptr(true),
				Recipes: map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
					"cosmosDB": {
						LinkType:     to.Ptr(linkrp.MongoDatabasesResourceType),
						TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
					},
				},
				Compute: &v20220315privatepreview.KubernetesCompute{
					Namespace: to.Ptr("default"),
				},
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetEnvDetails(gomock.Any(), gomock.Any()).
			Return(envResource, nil).Times(1)
		appManagementClient.EXPECT().
			CreateEnvironment(context.Background(), "kind-kind", v1.LocationGlobal, "default", "Kubernetes", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(true, nil).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
			TemplatePath:      "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
			LinkType:          linkrp.MongoDatabasesResourceType,
			RecipeName:        "cosmosDB_new",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
	})

	t.Run("Register recipe with parameters", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		envResource := v20220315privatepreview.EnvironmentResource{
			ID:       to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
			Name:     to.Ptr("kind-kind"),
			Type:     to.Ptr("applications.core/environments"),
			Location: to.Ptr(v1.LocationGlobal),
			Properties: &v20220315privatepreview.EnvironmentProperties{
				UseDevRecipes: to.Ptr(true),
				Recipes: map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
					"cosmosDB": {
						LinkType:     to.Ptr(linkrp.MongoDatabasesResourceType),
						TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
						Parameters:   map[string]any{"throughput": 400},
					},
				},
				Compute: &v20220315privatepreview.KubernetesCompute{
					Namespace: to.Ptr("default"),
				},
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetEnvDetails(gomock.Any(), gomock.Any()).
			Return(envResource, nil).Times(1)
		appManagementClient.EXPECT().
			CreateEnvironment(context.Background(), "kind-kind", v1.LocationGlobal, "default", "Kubernetes", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(true, nil).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
			TemplatePath:      "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
			LinkType:          linkrp.MongoDatabasesResourceType,
			RecipeName:        "cosmosDB_new",
			Parameters:        map[string]map[string]any{},
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
	})

	t.Run("Register recipe with no namespace", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		envResource := v20220315privatepreview.EnvironmentResource{
			ID:       to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
			Name:     to.Ptr("kind-kind"),
			Type:     to.Ptr("applications.core/environments"),
			Location: to.Ptr(v1.LocationGlobal),
			Properties: &v20220315privatepreview.EnvironmentProperties{
				UseDevRecipes: to.Ptr(true),
				Recipes: map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
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
		appManagementClient.EXPECT().
			CreateEnvironment(context.Background(), "kind-kind", v1.LocationGlobal, "", "Kubernetes", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(true, nil).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
			TemplatePath:      "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
			LinkType:          linkrp.MongoDatabasesResourceType,
			RecipeName:        "cosmosDB_no_namespace",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
	})

	t.Run("Register recipe with an existing name.", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		envResource := v20220315privatepreview.EnvironmentResource{
			ID:       to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
			Name:     to.Ptr("kind-kind"),
			Type:     to.Ptr("applications.core/environments"),
			Location: to.Ptr(v1.LocationGlobal),
			Properties: &v20220315privatepreview.EnvironmentProperties{
				UseDevRecipes: to.Ptr(true),
				Recipes: map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
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
			TemplatePath:      "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
			LinkType:          linkrp.MongoDatabasesResourceType,
			RecipeName:        "cosmosDB",
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
	})
}
