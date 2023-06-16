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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/clients"
	types "github.com/project-radius/radius/pkg/cli/cmd/recipe"
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
			Input:         []string{"test_recipe", "--template-kind", types.TemplateKindBicep, "--template-path", "test_template", "--link-type", linkrp.N_MongoDatabasesResourceType, "--parameters", "a=b"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid Register Command with parameters passed as file",
			Input:         []string{"test_recipe", "--template-kind", types.TemplateKindBicep, "--template-path", "test_template", "--link-type", linkrp.N_MongoDatabasesResourceType, "--parameters", "@testdata/recipeparam.json"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Register Command with fallback workspace",
			Input:         []string{"-e", "myenvironment", "test_recipe", "--template-kind", types.TemplateKindBicep, "--template-path", "test_template", "--link-type", linkrp.N_MongoDatabasesResourceType},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Register Command without name",
			Input:         []string{"--template-kind", types.TemplateKindBicep, "--template-path", "test_template", "--link-type", linkrp.N_MongoDatabasesResourceType},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Register Command without template kind",
			Input:         []string{"test_recipe", "--template-path", "test_template", "--link-type", linkrp.N_MongoDatabasesResourceType},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Register Command without template path",
			Input:         []string{"test_recipe", "--template-kind", types.TemplateKindBicep, "--link-type", linkrp.N_MongoDatabasesResourceType},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Register Command without link-type",
			Input:         []string{"test_recipe", "--template-kind", types.TemplateKindBicep, "--template-path", "test_template"},
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

		testRecipes := map[string]map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
			linkrp.N_MongoDatabasesResourceType: {
				"cosmosDB": {
					TemplateKind: to.Ptr(types.TemplateKindBicep),
					TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
				},
			},
		}

		testEnvProperties := &v20220315privatepreview.EnvironmentProperties{
			Recipes: testRecipes,
			Compute: &v20220315privatepreview.KubernetesCompute{
				Namespace: to.Ptr("default"),
			},
		}

		envResource := v20220315privatepreview.EnvironmentResource{
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
			TemplateKind:      types.TemplateKindBicep,
			TemplatePath:      "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
			LinkType:          linkrp.N_MongoDatabasesResourceType,
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

		testRecipes := map[string]map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
			linkrp.N_MongoDatabasesResourceType: {
				"cosmosDB": {
					TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
				},
			},
		}

		testEnvProperties := &v20220315privatepreview.EnvironmentProperties{
			Recipes: testRecipes,
			Compute: &v20220315privatepreview.KubernetesCompute{
				Namespace: to.Ptr("default"),
			},
		}

		envResource := v20220315privatepreview.EnvironmentResource{
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
			TemplatePath:      "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
			LinkType:          linkrp.N_MongoDatabasesResourceType,
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
			Return(v20220315privatepreview.EnvironmentResource{}, expectedError).
			Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
			TemplatePath:      "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
			LinkType:          linkrp.N_MongoDatabasesResourceType,
			RecipeName:        "cosmosDB_new",
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Equal(t, expectedError, err)
	})

	t.Run("Register recipe with parameters", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		testEnvProperties := &v20220315privatepreview.EnvironmentProperties{
			Recipes: map[string]map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
				linkrp.N_MongoDatabasesResourceType: {
					"cosmosDB": {
						TemplateKind: to.Ptr(types.TemplateKindBicep),
						TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
						Parameters:   map[string]any{"throughput": 400},
					},
				},
			},
			Compute: &v20220315privatepreview.KubernetesCompute{
				Kind:       to.Ptr("kubernetes"),
				Namespace:  to.Ptr("default"),
				ResourceID: to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind/compute/kubernetes"),
			},
		}

		envResource := v20220315privatepreview.EnvironmentResource{
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
			TemplateKind:      types.TemplateKindBicep,
			TemplatePath:      "testpublicrecipe.azurecr.io/bicep/modules/rediscaches:v1",
			LinkType:          linkrp.N_RedisCachesResourceType,
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
		testEnvProperties := &v20220315privatepreview.EnvironmentProperties{
			Recipes: map[string]map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
				linkrp.N_MongoDatabasesResourceType: {
					"cosmosDB": {
						TemplateKind: to.Ptr(types.TemplateKindBicep),
						TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
					},
				},
			},
		}

		envResource := v20220315privatepreview.EnvironmentResource{
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
			TemplateKind:      types.TemplateKindBicep,
			TemplatePath:      "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
			LinkType:          linkrp.N_MongoDatabasesResourceType,
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

		testEnvProperties := &v20220315privatepreview.EnvironmentProperties{
			Compute: &v20220315privatepreview.KubernetesCompute{
				Namespace: to.Ptr("default"),
			},
		}

		envResource := v20220315privatepreview.EnvironmentResource{
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
			TemplateKind:      types.TemplateKindBicep,
			TemplatePath:      "testpublicrecipe.azurecr.io/bicep/modules/rediscaches:v1",
			LinkType:          linkrp.N_RedisCachesResourceType,
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
