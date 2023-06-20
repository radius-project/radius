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

package update

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clierrors"
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
			Name:          "Update Env Command without any flags",
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
	t.Run("Failure: No Flags Set", func(t *testing.T) {
		runner := &Runner{
			noFlagsSet: true,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
		require.Equal(t, nil, err)
	})

	t.Run("Failure: Get Environment Details Error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		environment := corerp.EnvironmentResource{
			Name:       to.Ptr("test-env"),
			Properties: &corerp.EnvironmentProperties{},
		}

		expectedError := errors.New("failed to update the environment")

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetEnvDetails(gomock.Any(), "test-env").
			Return(environment, expectedError).
			Times(1)

		testProviders := &corerp.Providers{
			Azure: &corerp.ProvidersAzure{
				Scope: to.Ptr("/subscriptions/testSubId/resourceGroups/test-group"),
			},
			Aws: &corerp.ProvidersAws{
				Scope: to.Ptr("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
			},
		}

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
		require.Error(t, expectedError)
		require.Equal(t, expectedError.Error(), err.Error())
	})

	t.Run("Failure: Environment Doesn't Exist", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		environment := corerp.EnvironmentResource{
			Name:       to.Ptr("test-env"),
			Properties: &corerp.EnvironmentProperties{},
		}

		expectedError := &azcore.ResponseError{
			ErrorCode: v1.CodeNotFound,
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetEnvDetails(gomock.Any(), "test-env").
			Return(environment, expectedError).
			Times(1)

		testProviders := &corerp.Providers{
			Azure: &corerp.ProvidersAzure{
				Scope: to.Ptr("/subscriptions/testSubId/resourceGroups/test-group"),
			},
			Aws: &corerp.ProvidersAws{
				Scope: to.Ptr("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
			},
		}

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
		require.Error(t, expectedError)
		require.Equal(t, clierrors.Message(envNotFoundErrMessageFmt, "test-env"), err)
	})

	t.Run("Failure: Update Environment Error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		environment := corerp.EnvironmentResource{
			Name:       to.Ptr("test-env"),
			Properties: &corerp.EnvironmentProperties{},
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
			Providers: testProviders,
		}

		expectedError := errors.New("failed to update the environment")
		expectedErrorMessage := fmt.Sprintf("Failed to apply cloud provider scope to the environment %q. Cause: %s.", "test-env", expectedError.Error())

		appManagementClient.EXPECT().
			CreateEnvironment(gomock.Any(), "test-env", v1.LocationGlobal, testEnvProperties).
			Return(expectedError).
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
		require.Error(t, expectedError)
		require.Equal(t, expectedErrorMessage, err.Error())
	})

	t.Run("Success: Update Environment With Providers", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		environment := corerp.EnvironmentResource{
			Name: to.Ptr("test-env"),
			Properties: &corerp.EnvironmentProperties{
				Recipes: map[string]map[string]*corerp.EnvironmentRecipeProperties{},
				Compute: &corerp.KubernetesCompute{
					Namespace:  to.Ptr("default"),
					Kind:       to.Ptr("kubernetes"),
					ResourceID: to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind/compute/kubernetes"),
				},
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
			Providers: testProviders,
			Recipes:   map[string]map[string]*corerp.EnvironmentRecipeProperties{},
			Compute: &corerp.KubernetesCompute{
				Namespace:  to.Ptr("default"),
				Kind:       to.Ptr("kubernetes"),
				ResourceID: to.Ptr("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind/compute/kubernetes"),
			},
		}
		appManagementClient.EXPECT().
			CreateEnvironment(gomock.Any(), "test-env", v1.LocationGlobal, testEnvProperties).
			Return(nil).
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
			EnvName:     "test-env",
			Recipes:     0,
			Providers:   2,
			ComputeKind: "kubernetes",
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

	t.Run("Update Environment With Existing Providers", func(t *testing.T) {
		testCases := []struct {
			name              string
			existingProviders *corerp.Providers
			expectedProviders *corerp.Providers
			clearEnvAzure     bool // only applies to Azure
			clearEnvAws       bool // only applies to AWS
			expectedError     error
		}{
			{
				name: "Update Environment With Existing Azure Provider",
				existingProviders: &corerp.Providers{
					Azure: &corerp.ProvidersAzure{
						Scope: to.Ptr("/subscriptions/testSubId-1/resourceGroups/test-group-1"),
					},
					Aws: &corerp.ProvidersAws{
						Scope: to.Ptr("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
					},
				},
				expectedProviders: &corerp.Providers{
					Azure: &corerp.ProvidersAzure{
						Scope: to.Ptr("/subscriptions/testSubId/resourceGroups/test-group"),
					},
					Aws: &corerp.ProvidersAws{
						Scope: to.Ptr("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
					},
				},
				clearEnvAzure: false,
				clearEnvAws:   false,
				expectedError: nil,
			},
			{
				name: "Update Environment With Existing Azure Provider and Clear Azure Provider",
				existingProviders: &corerp.Providers{
					Azure: &corerp.ProvidersAzure{
						Scope: to.Ptr("/subscriptions/testSubId-1/resourceGroups/test-group-1"),
					},
					Aws: &corerp.ProvidersAws{
						Scope: to.Ptr("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
					},
				},
				expectedProviders: &corerp.Providers{
					Aws: &corerp.ProvidersAws{
						Scope: to.Ptr("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
					},
				},
				clearEnvAzure: true,
				clearEnvAws:   false,
				expectedError: nil,
			},
			{
				name: "Update Environment With Existing AWS Provider",
				existingProviders: &corerp.Providers{
					Azure: &corerp.ProvidersAzure{
						Scope: to.Ptr("/subscriptions/testSubId/resourceGroups/test-group"),
					},
					Aws: &corerp.ProvidersAws{
						Scope: to.Ptr("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
					},
				},
				expectedProviders: &corerp.Providers{
					Azure: &corerp.ProvidersAzure{
						Scope: to.Ptr("/subscriptions/testSubId/resourceGroups/test-group"),
					},
					Aws: &corerp.ProvidersAws{
						Scope: to.Ptr("/planes/aws/aws/accounts/testAwsAccount-1/regions/us-west-2"),
					},
				},
				clearEnvAzure: false,
				clearEnvAws:   false,
				expectedError: nil,
			},
			{
				name: "Update Environment With Existing AWS Provider and Clear AWS Provider",
				existingProviders: &corerp.Providers{
					Azure: &corerp.ProvidersAzure{
						Scope: to.Ptr("/subscriptions/testSubId/resourceGroups/test-group"),
					},
					Aws: &corerp.ProvidersAws{
						Scope: to.Ptr("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
					},
				},
				expectedProviders: &corerp.Providers{
					Azure: &corerp.ProvidersAzure{
						Scope: to.Ptr("/subscriptions/testSubId/resourceGroups/test-group"),
					},
				},
				clearEnvAzure: false,
				clearEnvAws:   true,
				expectedError: nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				existingEnvironment := corerp.EnvironmentResource{
					Name: to.Ptr("test-env"),
					Properties: &corerp.EnvironmentProperties{
						Providers: tc.existingProviders,
					},
				}

				appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
				appManagementClient.EXPECT().
					GetEnvDetails(gomock.Any(), "test-env").
					Return(existingEnvironment, nil).
					Times(1)

				existingEnvironment.Properties.Providers = tc.expectedProviders

				appManagementClient.EXPECT().
					CreateEnvironment(gomock.Any(), "test-env", v1.LocationGlobal, existingEnvironment.Properties).
					Return(nil).
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
					providers:         tc.expectedProviders,
					clearEnvAzure:     tc.clearEnvAzure,
				}

				err := runner.Run(context.Background())
				require.NoError(t, err)

				numberOfProviders := func() int {
					numberOfProviders := 0
					if tc.expectedProviders.Azure != nil {
						numberOfProviders++
					}
					if tc.expectedProviders.Aws != nil {
						numberOfProviders++
					}
					return numberOfProviders
				}

				obj := objectformats.OutputEnvObject{
					EnvName:   "test-env",
					Recipes:   0,
					Providers: numberOfProviders(),
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
	})
}
