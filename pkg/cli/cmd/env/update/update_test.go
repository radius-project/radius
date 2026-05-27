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
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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
			Name:          "Update Env Command with invalid Azure subscriptionId arg",
			Input:         []string{"default", "--azure-subscription-id", "subscriptionName", "--azure-resource-group", "testResourceGroup"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Update Env Command with single provider set",
			Input:         []string{"default", "--azure-subscription-id", "00000000-0000-0000-0000-000000000000", "--azure-resource-group", "testResourceGroup"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name: "Update Env Command with both providers set",
			Input: []string{"default", "--azure-subscription-id", "00000000-0000-0000-0000-000000000000", "--azure-resource-group", "testResourceGroup",
				"--aws-region", "us-west-2", "--aws-account-id", "testAWSAccount",
			},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Update Env Command with namespace flag",
			Input:         []string{"default", "--namespace", "mynamespace"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ValidateCallback: func(t *testing.T, runner framework.Runner) {
				r := runner.(*Runner)
				require.Equal(t, "mynamespace", r.Namespace)
			},
		},
		{
			Name:          "Update Env Command with --kubernetes-namespace flag",
			Input:         []string{"default", "--kubernetes-namespace", "mynamespace"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ValidateCallback: func(t *testing.T, runner framework.Runner) {
				r := runner.(*Runner)
				require.Equal(t, "mynamespace", r.Namespace)
			},
		},
		{
			Name:          "Update Env Command with invalid Kubernetes namespace",
			Input:         []string{"default", "--kubernetes-namespace", "BadNamespace"},
			ExpectedValid: false,
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
			Name:       new("test-env"),
			Properties: &corerp.EnvironmentProperties{},
		}

		expectedError := errors.New("failed to update the environment")

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetEnvironment(gomock.Any(), "test-env").
			Return(environment, expectedError).
			Times(1)

		testProviders := &corerp.Providers{
			Azure: &corerp.ProvidersAzure{
				Scope: new("/subscriptions/testSubId/resourceGroups/test-group"),
			},
			Aws: &corerp.ProvidersAws{
				Scope: new("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
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
			Name:       new("test-env"),
			Properties: &corerp.EnvironmentProperties{},
		}

		expectedError := &azcore.ResponseError{
			ErrorCode: v1.CodeNotFound,
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetEnvironment(gomock.Any(), "test-env").
			Return(environment, expectedError).
			Times(1)

		testProviders := &corerp.Providers{
			Azure: &corerp.ProvidersAzure{
				Scope: new("/subscriptions/testSubId/resourceGroups/test-group"),
			},
			Aws: &corerp.ProvidersAws{
				Scope: new("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
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
			Name:       new("test-env"),
			Properties: &corerp.EnvironmentProperties{},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetEnvironment(gomock.Any(), "test-env").
			Return(environment, nil).
			Times(1)

		testProviders := &corerp.Providers{
			Azure: &corerp.ProvidersAzure{
				Scope: new("/subscriptions/testSubId/resourceGroups/test-group"),
			},
			Aws: &corerp.ProvidersAws{
				Scope: new("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
			},
		}

		expectedResource := &corerp.EnvironmentResource{
			Location: to.Ptr(v1.LocationGlobal),
			Properties: &corerp.EnvironmentProperties{
				Providers: testProviders,
			},
		}

		expectedError := errors.New("failed to update the environment")
		expectedErrorMessage := fmt.Sprintf("Failed to apply cloud provider scope to the environment %q. Cause: %s.", "test-env", expectedError.Error())

		appManagementClient.EXPECT().
			CreateOrUpdateEnvironment(gomock.Any(), "test-env", expectedResource).
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
			Name: new("test-env"),
			Properties: &corerp.EnvironmentProperties{
				Recipes: map[string]map[string]corerp.RecipePropertiesClassification{},
				Compute: &corerp.KubernetesCompute{
					Namespace:  new("default"),
					Kind:       new("kubernetes"),
					ResourceID: new("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind/compute/kubernetes"),
				},
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetEnvironment(gomock.Any(), "test-env").
			Return(environment, nil).
			Times(1)

		testProviders := &corerp.Providers{
			Azure: &corerp.ProvidersAzure{
				Scope: new("/subscriptions/testSubId/resourceGroups/test-group"),
			},
			Aws: &corerp.ProvidersAws{
				Scope: new("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
			},
		}

		testEnvProperties := &corerp.EnvironmentProperties{
			Providers: testProviders,
			Recipes:   map[string]map[string]corerp.RecipePropertiesClassification{},
			Compute: &corerp.KubernetesCompute{
				Namespace:  new("default"),
				Kind:       new("kubernetes"),
				ResourceID: new("/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/environments/kind-kind/compute/kubernetes"),
			},
		}
		appManagementClient.EXPECT().
			CreateOrUpdateEnvironment(gomock.Any(), "test-env", &corerp.EnvironmentResource{
				Location:   to.Ptr(v1.LocationGlobal),
				Properties: testEnvProperties,
			}).
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
		_ = environment

		expected := []any{
			output.LogOutput{
				Format: "Applications.Core/environments/%s updated",
				Params: []any{"test-env"},
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Success: Update Environment With Namespace", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		environment := corerp.EnvironmentResource{
			Name: new("test-env"),
			Properties: &corerp.EnvironmentProperties{
				Compute: &corerp.KubernetesCompute{
					Namespace: new("old-namespace"),
					Kind:      new("kubernetes"),
				},
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetEnvironment(gomock.Any(), "test-env").
			Return(environment, nil).
			Times(1)

		testProviders := &corerp.Providers{
			Azure: &corerp.ProvidersAzure{},
			Aws:   &corerp.ProvidersAws{},
		}

		expectedEnvProperties := &corerp.EnvironmentProperties{
			Compute: &corerp.KubernetesCompute{
				Namespace: new("new-namespace"),
				Kind:      new("kubernetes"),
			},
		}
		appManagementClient.EXPECT().
			CreateOrUpdateEnvironment(gomock.Any(), "test-env", &corerp.EnvironmentResource{
				Location:   to.Ptr(v1.LocationGlobal),
				Properties: expectedEnvProperties,
			}).
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
			Namespace:         "new-namespace",
			providers:         testProviders,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.LogOutput{
				Format: "Applications.Core/environments/%s updated",
				Params: []any{"test-env"},
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Success: Update Environment With Namespace When Compute Is Nil", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		environment := corerp.EnvironmentResource{
			Name:       new("test-env"),
			Properties: &corerp.EnvironmentProperties{
				// Compute is nil — exercises the nil-branch which must set Kind.
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetEnvironment(gomock.Any(), "test-env").
			Return(environment, nil).
			Times(1)

		expectedEnvProperties := &corerp.EnvironmentProperties{
			Compute: &corerp.KubernetesCompute{
				Kind:      to.Ptr(string(rpv1.KubernetesComputeKind)),
				Namespace: new("new-namespace"),
			},
		}
		appManagementClient.EXPECT().
			CreateOrUpdateEnvironment(gomock.Any(), "test-env", &corerp.EnvironmentResource{
				Location:   to.Ptr(v1.LocationGlobal),
				Properties: expectedEnvProperties,
			}).
			Return(nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{"kind": "kubernetes", "context": "kind-kind"},
			Name:       "kind-kind",
			Scope:      "/planes/radius/local/resourceGroups/test-group",
		}
		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Workspace:         workspace,
			Output:            outputSink,
			EnvName:           "test-env",
			Namespace:         "new-namespace",
			providers:         &corerp.Providers{},
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
	})

	t.Run("Failure: Update Environment With Namespace When Compute Is Non-Kubernetes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Use a non-Kubernetes concrete compute type to exercise the default branch.
		environment := corerp.EnvironmentResource{
			Name: new("test-env"),
			Properties: &corerp.EnvironmentProperties{
				Compute: &corerp.AzureContainerInstanceCompute{},
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetEnvironment(gomock.Any(), "test-env").
			Return(environment, nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{"kind": "kubernetes", "context": "kind-kind"},
			Name:       "kind-kind",
			Scope:      "/planes/radius/local/resourceGroups/test-group",
		}
		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Workspace:         workspace,
			Output:            outputSink,
			EnvName:           "test-env",
			Namespace:         "new-namespace",
			providers:         &corerp.Providers{},
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "existing compute kind is not Kubernetes")
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
						Scope: new("/subscriptions/testSubId-1/resourceGroups/test-group-1"),
					},
					Aws: &corerp.ProvidersAws{
						Scope: new("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
					},
				},
				expectedProviders: &corerp.Providers{
					Azure: &corerp.ProvidersAzure{
						Scope: new("/subscriptions/testSubId/resourceGroups/test-group"),
					},
					Aws: &corerp.ProvidersAws{
						Scope: new("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
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
						Scope: new("/subscriptions/testSubId-1/resourceGroups/test-group-1"),
					},
					Aws: &corerp.ProvidersAws{
						Scope: new("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
					},
				},
				expectedProviders: &corerp.Providers{
					Aws: &corerp.ProvidersAws{
						Scope: new("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
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
						Scope: new("/subscriptions/testSubId/resourceGroups/test-group"),
					},
					Aws: &corerp.ProvidersAws{
						Scope: new("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
					},
				},
				expectedProviders: &corerp.Providers{
					Azure: &corerp.ProvidersAzure{
						Scope: new("/subscriptions/testSubId/resourceGroups/test-group"),
					},
					Aws: &corerp.ProvidersAws{
						Scope: new("/planes/aws/aws/accounts/testAwsAccount-1/regions/us-west-2"),
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
						Scope: new("/subscriptions/testSubId/resourceGroups/test-group"),
					},
					Aws: &corerp.ProvidersAws{
						Scope: new("/planes/aws/aws/accounts/testAwsAccount/regions/us-west-2"),
					},
				},
				expectedProviders: &corerp.Providers{
					Azure: &corerp.ProvidersAzure{
						Scope: new("/subscriptions/testSubId/resourceGroups/test-group"),
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
					Name: new("test-env"),
					Properties: &corerp.EnvironmentProperties{
						Providers: tc.existingProviders,
					},
				}

				appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
				appManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "test-env").
					Return(existingEnvironment, nil).
					Times(1)

				existingEnvironment.Properties.Providers = tc.expectedProviders

				appManagementClient.EXPECT().
					CreateOrUpdateEnvironment(gomock.Any(), "test-env", &corerp.EnvironmentResource{
						Location:   to.Ptr(v1.LocationGlobal),
						Properties: existingEnvironment.Properties,
					}).
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

				expected := []any{
					output.LogOutput{
						Format: "Applications.Core/environments/%s updated",
						Params: []any{"test-env"},
					},
				}

				require.Equal(t, expected, outputSink.Writes)
			})
		}
	})
}

func Test_FlagGroups(t *testing.T) {
	t.Run("--namespace and --kubernetes-namespace are mutually exclusive", func(t *testing.T) {
		cmd, _ := NewCommand(&framework.Impl{})
		require.NoError(t, cmd.ParseFlags([]string{"--namespace", "ns1", "--kubernetes-namespace", "ns2"}))
		require.Error(t, cmd.ValidateFlagGroups())
	})

	t.Run("--azure-subscription-id requires --azure-resource-group", func(t *testing.T) {
		cmd, _ := NewCommand(&framework.Impl{})
		require.NoError(t, cmd.ParseFlags([]string{"--azure-subscription-id", "00000000-0000-0000-0000-000000000000"}))
		require.Error(t, cmd.ValidateFlagGroups())
	})

	t.Run("--aws-region requires --aws-account-id", func(t *testing.T) {
		cmd, _ := NewCommand(&framework.Impl{})
		require.NoError(t, cmd.ParseFlags([]string{"--aws-region", "us-west-2"}))
		require.Error(t, cmd.ValidateFlagGroups())
	})
}
