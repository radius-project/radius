// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package update

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
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
			Name:          "Update Env Command without pro",
			Input:         []string{"default"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Update Env Command without env arg",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Update Env Command with single provider set",
			Input:         []string{"default", "--azure-subscription-id", "testSubId", "--azure-resource-group", "testResourceGroup"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name: "Update Env Command with both providers set",
			Input: []string{"default", "--azure-subscription-id", "testSubId", "--azure-resource-group", "testResourceGroup",
				"--aws-region", "us-west-2", "--aws-account-id", "testAWSAccount",
			},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Update(t *testing.T) {
	t.Run("Success: Update Environment With Providers", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		environment := corerp.EnvironmentResource{
			Name: to.Ptr("test-env"),
			Properties: &corerp.EnvironmentProperties{
				UseDevRecipes: to.Ptr(false),
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetEnvDetails(gomock.Any(), "test-env").
			Return(environment, nil).
			Times(1)

		testProviders := &corerp.Providers{
			Azure: &corerp.ProvidersAzure{
				Scope: to.Ptr("/subscriptions/testSubId/resourceGroups/test-group"),
			},
			Aws: &corerp.ProvidersAws{
				Scope: to.Ptr("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
			},
		}

		testEnvProperties := &corerp.EnvironmentProperties{
			UseDevRecipes: to.Ptr(false),
			Providers:     testProviders,
		}
		appManagementClient.EXPECT().
			CreateEnvironment(gomock.Any(), "test-env", v1.LocationGlobal, testEnvProperties).
			Return(true, nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name:  "kind-kind",
			Scope: "/planes/radius/local/resourceGroups/test-group",
		}
		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Workspace:         workspace,
			Output:            outputSink,
			EnvName:           "test-env",
			providers:         testProviders,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		environment.Properties.Providers = testProviders
		obj := objectformats.OutputEnvObject{
			EnvName:   "test-env",
			Recipes:   0,
			Providers: 2,
		}

		expected := []any{
			output.LogOutput{
				Format: "Updating Environment...",
			},
			output.FormattedOutput{
				Format:  "table",
				Obj:     obj,
				Options: objectformats.GetUpdateEnvironmentTableFormat(),
			},
			output.LogOutput{
				Format: "Successfully updated environment %q.",
				Params: []any{"test-env"},
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})
	t.Run("Success: Update Environment With Existing Providers", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		environment := corerp.EnvironmentResource{
			Name: to.Ptr("test-env"),
			Properties: &corerp.EnvironmentProperties{
				Providers: &corerp.Providers{
					Azure: &corerp.ProvidersAzure{
						Scope: to.Ptr("/subscriptions/testSubId-1/resourceGroups/test-group-1"),
					},
				},
				UseDevRecipes: to.Ptr(false),
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetEnvDetails(gomock.Any(), "test-env").
			Return(environment, nil).
			Times(1)

		testProviders := &corerp.Providers{
			Azure: &corerp.ProvidersAzure{
				Scope: to.Ptr("/subscriptions/testSubId/resourceGroups/test-group"),
			},
			Aws: &corerp.ProvidersAws{
				Scope: to.Ptr("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
			},
		}

		testEnvProperties := &corerp.EnvironmentProperties{
			UseDevRecipes: to.Ptr(false),
			Providers:     testProviders,
		}
		appManagementClient.EXPECT().
			CreateEnvironment(gomock.Any(), "test-env", v1.LocationGlobal, testEnvProperties).
			Return(true, nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name:  "kind-kind",
			Scope: "/planes/radius/local/resourceGroups/test-group",
		}
		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Workspace:         workspace,
			Output:            outputSink,
			EnvName:           "test-env",
			providers:         testProviders,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		environment.Properties.Providers = testProviders
		obj := objectformats.OutputEnvObject{
			EnvName:   "test-env",
			Recipes:   0,
			Providers: 2,
		}

		expected := []any{
			output.LogOutput{
				Format: "Updating Environment...",
			},
			output.FormattedOutput{
				Format:  "table",
				Obj:     obj,
				Options: objectformats.GetUpdateEnvironmentTableFormat(),
			},
			output.LogOutput{
				Format: "Successfully updated environment %q.",
				Params: []any{"test-env"},
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})
}
