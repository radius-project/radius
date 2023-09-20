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

package unregister

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
	"github.com/radius-project/radius/pkg/portableresources"
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
			Name:          "Valid Unregister Command",
			Input:         []string{"test_recipe", "--resource-type", "resource-type"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Unregister Command with fallback workspace",
			Input:         []string{"-e", "my-env", "test_recipe", "--resource-type", "resource-type"},
			ExpectedValid: true,
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
			Input:         []string{"foo", "bar", "foo1", "--resource-type", "resource-type"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Unregister Command without resource type",
			Input:         []string{"foo"},
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

			testEnvProperties := &v20231001preview.EnvironmentProperties{
				Recipes: map[string]map[string]v20231001preview.RecipePropertiesClassification{
					portableresources.MongoDatabasesResourceType: {
						"cosmosDB": &v20231001preview.BicepRecipeProperties{
							TemplateKind: to.Ptr(recipes.TemplateKindBicep),
							TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
						},
					},
				},
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
				RecipeName:        "cosmosDB",
				ResourceType:      "Applications.Datastores/mongoDatabases",
			}

			expectedOutput := []any{
				output.LogOutput{
					Format: "Successfully unregistered recipe %q from environment %q ",
					Params: []interface{}{
						"cosmosDB",
						"kind-kind",
					},
				},
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)
			require.Equal(t, expectedOutput, outputSink.Writes)
		})

		t.Run("Failure", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			testEnvProperties := &v20231001preview.EnvironmentProperties{
				Recipes: map[string]map[string]v20231001preview.RecipePropertiesClassification{
					portableresources.MongoDatabasesResourceType: {
						"cosmosDB": &v20231001preview.BicepRecipeProperties{
							TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
						},
					},
				},
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

			expectedError := errors.New("failed to unregister recipe from the environment")
			expectedErrorMessage := fmt.Sprintf(
				"Failed to unregister the recipe %s from the environment %s. Cause: %s.",
				"cosmosDB",
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
				RecipeName:        "cosmosDB",
				ResourceType:      "Applications.Datastores/mongoDatabases",
			}

			err := runner.Run(context.Background())
			require.Error(t, err)
			require.Equal(t, expectedErrorMessage, err.Error())
		})

		t.Run("No Namespace", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			testEnvProperties := &v20231001preview.EnvironmentProperties{
				Recipes: map[string]map[string]v20231001preview.RecipePropertiesClassification{
					portableresources.MongoDatabasesResourceType: {
						"cosmosDB": &v20231001preview.BicepRecipeProperties{
							TemplateKind: to.Ptr(recipes.TemplateKindBicep),
							TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
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
				RecipeName:        "cosmosDB",
				ResourceType:      "Applications.Datastores/mongoDatabases",
			}

			expectedOutput := []any{
				output.LogOutput{
					Format: "Successfully unregistered recipe %q from environment %q ",
					Params: []interface{}{
						"cosmosDB",
						"kind-kind",
					},
				},
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)
			require.Equal(t, expectedOutput, outputSink.Writes)
		})

		t.Run("Unregister recipe that doesn't exist in the environment", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			envResource := v20231001preview.EnvironmentResource{
				ID:       to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
				Name:     to.Ptr("kind-kind"),
				Type:     to.Ptr("applications.core/environments"),
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &v20231001preview.EnvironmentProperties{
					Recipes: map[string]map[string]v20231001preview.RecipePropertiesClassification{
						portableresources.MongoDatabasesResourceType: {
							"cosmosDB": &v20231001preview.BicepRecipeProperties{
								TemplateKind: to.Ptr(recipes.TemplateKindBicep),
								TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
							},
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
				ResourceType:      "Applications.Datastores/mongoDatabases",
			}

			err := runner.Run(context.Background())
			require.Error(t, err)
		})

		t.Run("Unregister recipe with resourceType doesn't exist in the environment", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			envResource := v20231001preview.EnvironmentResource{
				ID:       to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
				Name:     to.Ptr("kind-kind"),
				Type:     to.Ptr("applications.core/environments"),
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &v20231001preview.EnvironmentProperties{
					Recipes: map[string]map[string]v20231001preview.RecipePropertiesClassification{
						portableresources.MongoDatabasesResourceType: {
							"testResource": &v20231001preview.BicepRecipeProperties{
								TemplateKind: to.Ptr(recipes.TemplateKindBicep),
								TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
							},
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
				RecipeName:        "testResource",
				ResourceType:      "Applications.Datastores/redisCaches",
			}

			err := runner.Run(context.Background())
			require.Error(t, err)
		})

		t.Run("Unregister recipe with no recipes added to the environment", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			envResource := v20231001preview.EnvironmentResource{
				ID:         to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
				Name:       to.Ptr("kind-kind"),
				Type:       to.Ptr("applications.core/environments"),
				Location:   to.Ptr(v1.LocationGlobal),
				Properties: &v20231001preview.EnvironmentProperties{},
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
				ResourceType:      "Applications.Datastores/mongoDatabases",
			}

			err := runner.Run(context.Background())
			require.Error(t, err)
		})

		t.Run("Unregister recipe with same name for different resource types.", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			testEnvProperties := &v20231001preview.EnvironmentProperties{
				Recipes: map[string]map[string]v20231001preview.RecipePropertiesClassification{
					portableresources.MongoDatabasesResourceType: {
						"testResource": &v20231001preview.BicepRecipeProperties{
							TemplateKind: to.Ptr(recipes.TemplateKindBicep),
							TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
						},
					},
					portableresources.RedisCachesResourceType: {
						"testResource": &v20231001preview.BicepRecipeProperties{
							TemplateKind: to.Ptr(recipes.TemplateKindBicep),
							TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/rediscaches:v1"),
						},
					},
				},
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
				RecipeName:        "testResource",
				ResourceType:      "Applications.Datastores/mongoDatabases",
			}

			expectedOutput := []any{
				output.LogOutput{
					Format: "Successfully unregistered recipe %q from environment %q ",
					Params: []interface{}{
						"testResource",
						"kind-kind",
					},
				},
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)
			require.Equal(t, expectedOutput, outputSink.Writes)
		})
	})
}
