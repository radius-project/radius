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

package register

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	ds_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller"
	"github.com/radius-project/radius/pkg/recipes"
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
			Name:          "Valid Register Command with parameters",
			Input:         []string{"test_recipe", "--template-kind", recipes.TemplateKindBicep, "--template-path", "test_template", "--resource-type", ds_ctrl.MongoDatabasesResourceType, "--parameters", "a=b"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid Register Command for terraform recipe",
			Input:         []string{"test_recipe", "--template-kind", recipes.TemplateKindTerraform, "--template-path", "test_template", "--resource-type", ds_ctrl.MongoDatabasesResourceType, "--template-version", "1.1.0"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid Register Command with parameters passed as file",
			Input:         []string{"test_recipe", "--template-kind", recipes.TemplateKindBicep, "--template-path", "test_template", "--resource-type", ds_ctrl.MongoDatabasesResourceType, "--parameters", "@testdata/recipeparam.json", "--plain-http"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Register Command with fallback workspace",
			Input:         []string{"-e", "myenvironment", "test_recipe", "--template-kind", recipes.TemplateKindBicep, "--template-path", "test_template", "--resource-type", ds_ctrl.MongoDatabasesResourceType},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Register Command without name",
			Input:         []string{"--template-kind", recipes.TemplateKindBicep, "--template-path", "test_template", "--resource-type", ds_ctrl.MongoDatabasesResourceType},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Register Command without template kind",
			Input:         []string{"test_recipe", "--template-path", "test_template", "--resource-type", ds_ctrl.MongoDatabasesResourceType},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Register Command without template path",
			Input:         []string{"test_recipe", "--template-kind", recipes.TemplateKindBicep, "--resource-type", ds_ctrl.MongoDatabasesResourceType},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Register Command without resource-type",
			Input:         []string{"test_recipe", "--template-kind", recipes.TemplateKindBicep, "--template-path", "test_template"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Register Command with too many args",
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

		testRecipes := map[string]map[string]v20231001preview.RecipePropertiesClassification{
			ds_ctrl.MongoDatabasesResourceType: {
				"cosmosDB": &v20231001preview.BicepRecipeProperties{
					TemplateKind: to.Ptr(recipes.TemplateKindBicep),
					TemplatePath: to.Ptr("ghcr.io/testpublicrecipe/bicep/modules/mongodatabases:v1"),
				},
			},
		}

		testEnvProperties := &v20231001preview.EnvironmentProperties{
			Recipes: testRecipes,
			Compute: &v20231001preview.KubernetesCompute{
				Namespace: to.Ptr("default"),
			},
		}

		envResource := v20231001preview.EnvironmentResource{
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
			Return(nil).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
			TemplateKind:      recipes.TemplateKindTerraform,
			TemplatePath:      "Azure/cosmosdb/azurerm",
			TemplateVersion:   "1.1.0",
			ResourceType:      ds_ctrl.MongoDatabasesResourceType,
			RecipeName:        "cosmosDB_new",
		}

		expectedOutput := []any{
			output.LogOutput{
				Format: "Successfully linked recipe %q to environment %q ",
				Params: []interface{}{
					"cosmosDB_new",
					"kind-kind",
				},
			},
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
		require.Equal(t, expectedOutput, outputSink.Writes)
	})

	t.Run("Register recipe Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		testRecipes := map[string]map[string]v20231001preview.RecipePropertiesClassification{
			ds_ctrl.MongoDatabasesResourceType: {
				"cosmosDB": &v20231001preview.BicepRecipeProperties{
					TemplatePath: to.Ptr("ghcr.io/testpublicrecipe/bicep/modules/mongodatabases:v1"),
				},
			},
		}

		testEnvProperties := &v20231001preview.EnvironmentProperties{
			Recipes: testRecipes,
			Compute: &v20231001preview.KubernetesCompute{
				Namespace: to.Ptr("default"),
			},
		}

		envResource := v20231001preview.EnvironmentResource{
			ID:         to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
			Name:       to.Ptr("kind-kind"),
			Type:       to.Ptr("applications.core/environments"),
			Location:   to.Ptr(v1.LocationGlobal),
			Properties: testEnvProperties,
		}

		expectedError := errors.New("failed to register recipe to the environment")
		expectedErrorMessage := fmt.Sprintf(
			"Failed to register the recipe %q to the environment %q. Cause: %s.",
			"cosmosDB_new",
			"/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind",
			expectedError.Error())

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetEnvDetails(gomock.Any(), gomock.Any()).
			Return(envResource, nil).
			Times(1)
		appManagementClient.EXPECT().
			CreateEnvironment(context.Background(), "kind-kind", v1.LocationGlobal, testEnvProperties).
			Return(expectedError).
			Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
			TemplatePath:      "ghcr.io/testpublicrecipe/bicep/modules/mongodatabases:v1",
			ResourceType:      ds_ctrl.MongoDatabasesResourceType,
			RecipeName:        "cosmosDB_new",
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Equal(t, expectedErrorMessage, err.Error())
	})

	t.Run("Failure Getting Environment Details", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		expectedError := errors.New("failed to get environment details")

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetEnvDetails(gomock.Any(), gomock.Any()).
			Return(v20231001preview.EnvironmentResource{}, expectedError).
			Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
			TemplatePath:      "ghcr.io/testpublicrecipe/bicep/modules/mongodatabases:v1",
			ResourceType:      ds_ctrl.MongoDatabasesResourceType,
			RecipeName:        "cosmosDB_new",
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Equal(t, expectedError, err)
	})

	t.Run("Register recipe with parameters", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		testEnvProperties := &v20231001preview.EnvironmentProperties{
			Recipes: map[string]map[string]v20231001preview.RecipePropertiesClassification{
				ds_ctrl.MongoDatabasesResourceType: {
					"cosmosDB": &v20231001preview.BicepRecipeProperties{
						TemplateKind: to.Ptr(recipes.TemplateKindBicep),
						TemplatePath: to.Ptr("ghcr.io/testpublicrecipe/bicep/modules/mongodatabases:v1"),
						Parameters:   map[string]any{"throughput": 400},
						PlainHTTP:    to.Ptr(true),
					},
				},
			},
			Compute: &v20231001preview.KubernetesCompute{
				Kind:       to.Ptr("kubernetes"),
				Namespace:  to.Ptr("default"),
				ResourceID: to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind/compute/kubernetes"),
			},
		}

		envResource := v20231001preview.EnvironmentResource{
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
			Return(nil).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
			TemplateKind:      recipes.TemplateKindBicep,
			TemplatePath:      "ghcr.io/testpublicrecipe/bicep/modules/rediscaches:v1",
			ResourceType:      ds_ctrl.RedisCachesResourceType,
			RecipeName:        "redis",
			Parameters:        map[string]map[string]any{},
		}

		expectedOutput := []any{
			output.LogOutput{
				Format: "Successfully linked recipe %q to environment %q ",
				Params: []interface{}{
					"redis",
					"kind-kind",
				},
			},
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
		require.Equal(t, expectedOutput, outputSink.Writes)
	})

	t.Run("Register recipe with no namespace", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		testEnvProperties := &v20231001preview.EnvironmentProperties{
			Recipes: map[string]map[string]v20231001preview.RecipePropertiesClassification{
				ds_ctrl.MongoDatabasesResourceType: {
					"cosmosDB": &v20231001preview.BicepRecipeProperties{
						TemplateKind: to.Ptr(recipes.TemplateKindBicep),
						TemplatePath: to.Ptr("ghcr.io/testpublicrecipe/bicep/modules/mongodatabases:v1"),
					},
				},
			},
		}

		envResource := v20231001preview.EnvironmentResource{
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
			Return(nil).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
			TemplateKind:      recipes.TemplateKindBicep,
			TemplatePath:      "ghcr.io/testpublicrecipe/bicep/modules/mongodatabases:v1",
			ResourceType:      ds_ctrl.MongoDatabasesResourceType,
			RecipeName:        "cosmosDB_no_namespace",
		}

		expectedOutput := []any{
			output.LogOutput{
				Format: "Successfully linked recipe %q to environment %q ",
				Params: []interface{}{
					"cosmosDB_no_namespace",
					"kind-kind",
				},
			},
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
		require.Equal(t, expectedOutput, outputSink.Writes)
	})

	t.Run("Register the first recipe", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		testEnvProperties := &v20231001preview.EnvironmentProperties{
			Compute: &v20231001preview.KubernetesCompute{
				Namespace: to.Ptr("default"),
			},
		}

		envResource := v20231001preview.EnvironmentResource{
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
			Return(nil).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
			TemplateKind:      recipes.TemplateKindBicep,
			TemplatePath:      "ghcr.io/testpublicrecipe/bicep/modules/rediscaches:v1",
			ResourceType:      ds_ctrl.RedisCachesResourceType,
			RecipeName:        "redis",
		}

		expectedOutput := []any{
			output.LogOutput{
				Format: "Successfully linked recipe %q to environment %q ",
				Params: []interface{}{
					"redis",
					"kind-kind",
				},
			},
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
		require.Equal(t, expectedOutput, outputSink.Writes)
	})
}
