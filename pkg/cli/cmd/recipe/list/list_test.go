// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package list

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
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
			Name:          "Valid List Command",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "List Command with fallback workspace",
			Input:         []string{"-e", "my-env", "-g", "my-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
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
	t.Run("List recipes linked to the environment - Success", func(t *testing.T) {
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
		recipes := []types.EnvironmentRecipe{
			{
				Name:         "cosmosDB",
				LinkType:     linkrp.MongoDatabasesResourceType,
				TemplateKind: types.TemplateKindBicep,
				TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
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
			Workspace:         &workspaces.Workspace{},
			Format:            "table",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipes,
				Options: objectformats.GetEnvironmentRecipesTableFormat(),
			},
		}
		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("List recipes linked to the environment - empty template kind", func(t *testing.T) {
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
							TemplatePath: to.Ptr("testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1"),
						},
					},
				},
			},
		}
		recipes := []types.EnvironmentRecipe{
			{
				Name:         "cosmosDB",
				LinkType:     linkrp.MongoDatabasesResourceType,
				TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
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
			Workspace:         &workspaces.Workspace{},
			Format:            "table",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.FormattedOutput{
				Format:  "table",
				Obj:     recipes,
				Options: objectformats.GetEnvironmentRecipesTableFormat(),
			},
		}
		require.Equal(t, expected, outputSink.Writes)
	})
}
