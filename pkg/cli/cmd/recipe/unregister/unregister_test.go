// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package unregister

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
			Name:          "Valid Unregister Command",
			Input:         []string{"test_recipe", "--link-type", "link-type"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Unregister Command with fallback workspace",
			Input:         []string{"-e", "my-env", "test_recipe", "--link-type", "link-type"},
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
			Input:         []string{"foo", "bar", "foo1", "--link-type", "link-type"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Unregister Command without link type",
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

			testEnvProperties := &v20220315privatepreview.EnvironmentProperties{
				Recipes: map[string]map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
					linkrp.MongoDatabasesResourceType: {
						"cosmosDB": {
							TemplateKind: to.Ptr(types.TemplateKindBicep),
							TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
						},
					},
				},
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
				Return(true, nil).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
				RecipeName:        "cosmosDB",
				LinkType:          "Applications.Link/mongoDatabases",
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

			testEnvProperties := &v20220315privatepreview.EnvironmentProperties{
				Recipes: map[string]map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
					linkrp.MongoDatabasesResourceType: {
						"cosmosDB": {
							TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
						},
					},
				},
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

			expectedError := errors.New("failed to unregister recipe from the environment")
			expectedErrorMessage := fmt.Sprintf("failed to unregister the recipe %s from the environment %s: %s", "cosmosDB",
				"/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind", expectedError.Error())

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetEnvDetails(gomock.Any(), gomock.Any()).
				Return(envResource, nil).
				Times(1)
			appManagementClient.EXPECT().
				CreateEnvironment(context.Background(), "kind-kind", v1.LocationGlobal, testEnvProperties).
				Return(false, expectedError).
				Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
				RecipeName:        "cosmosDB",
				LinkType:          "Applications.Link/mongoDatabases",
			}

			err := runner.Run(context.Background())
			require.Error(t, err)
			require.Equal(t, expectedErrorMessage, err.Error())
		})

		t.Run("No Namespace", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			testEnvProperties := &v20220315privatepreview.EnvironmentProperties{
				Recipes: map[string]map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
					linkrp.MongoDatabasesResourceType: {
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
				Return(true, nil).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
				RecipeName:        "cosmosDB",
				LinkType:          "Applications.Link/mongoDatabases",
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

			envResource := v20220315privatepreview.EnvironmentResource{
				ID:       to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
				Name:     to.Ptr("kind-kind"),
				Type:     to.Ptr("applications.core/environments"),
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &v20220315privatepreview.EnvironmentProperties{
					Recipes: map[string]map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
						linkrp.MongoDatabasesResourceType: {
							"cosmosDB": {
								TemplateKind: to.Ptr(types.TemplateKindBicep),
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
				LinkType:          "Applications.Link/mongoDatabases",
			}

			err := runner.Run(context.Background())
			require.Error(t, err)
		})

		t.Run("Unregister recipe with linkType doesn't exist in the environment", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			envResource := v20220315privatepreview.EnvironmentResource{
				ID:       to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
				Name:     to.Ptr("kind-kind"),
				Type:     to.Ptr("applications.core/environments"),
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &v20220315privatepreview.EnvironmentProperties{
					Recipes: map[string]map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
						linkrp.MongoDatabasesResourceType: {
							"testResource": {
								TemplateKind: to.Ptr(types.TemplateKindBicep),
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
				LinkType:          "Applications.Link/redisCaches",
			}

			err := runner.Run(context.Background())
			require.Error(t, err)
		})

		t.Run("Unregister recipe with no recipes added to the environment", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			envResource := v20220315privatepreview.EnvironmentResource{
				ID:         to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind"),
				Name:       to.Ptr("kind-kind"),
				Type:       to.Ptr("applications.core/environments"),
				Location:   to.Ptr(v1.LocationGlobal),
				Properties: &v20220315privatepreview.EnvironmentProperties{},
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
				LinkType:          "Applications.Link/mongoDatabases",
			}

			err := runner.Run(context.Background())
			require.Error(t, err)
		})

		t.Run("Unregister recipe with same name for different resource types.", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			testEnvProperties := &v20220315privatepreview.EnvironmentProperties{
				Recipes: map[string]map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
					linkrp.MongoDatabasesResourceType: {
						"testResource": {
							TemplateKind: to.Ptr(types.TemplateKindBicep),
							TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
						},
					},
					linkrp.RedisCachesResourceType: {
						"testResource": {
							TemplateKind: to.Ptr(types.TemplateKindBicep),
							TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/rediscaches:v1"),
						},
					},
				},
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
				Return(true, nil).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{Environment: "kind-kind"},
				RecipeName:        "testResource",
				LinkType:          "Applications.Link/mongoDatabases",
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
