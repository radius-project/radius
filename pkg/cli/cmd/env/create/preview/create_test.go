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

package preview

import (
	"context"
	"errors"
	"net/http"
	"testing"

	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/recipepack"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	corerpfake "github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid create command",
			Input:         []string{"testingenv"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupSuccess(mocks.ApplicationManagementClient, "test-resource-group")
			},
			ValidateCallback: func(t *testing.T, runner framework.Runner) {
				r := runner.(*Runner)
				require.Equal(t, "testingenv", r.EnvironmentName)
				require.Equal(t, "test-resource-group", r.ResourceGroupName)
			},
		},
		{
			Name:          "Create command with explicit resource group",
			Input:         []string{"testingenv", "-g", "explicit-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupSuccess(mocks.ApplicationManagementClient, "explicit-group")
			},
			ValidateCallback: func(t *testing.T, runner framework.Runner) {
				r := runner.(*Runner)
				require.Equal(t, "explicit-group", r.ResourceGroupName)
				require.Equal(t, "/planes/radius/local/resourceGroups/explicit-group", r.Workspace.Scope)
			},
		},
		{
			Name:          "Create command with invalid resource group",
			Input:         []string{"testingenv", "-g", "missing-group"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupNotFound(mocks.ApplicationManagementClient, "missing-group")
			},
		},
		{
			Name:          "Create command with resource group lookup error",
			Input:         []string{"testingenv"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupError(mocks.ApplicationManagementClient, "test-resource-group", errors.New("lookup failed"))
			},
		},
		{
			Name:          "Create command with fallback workspace",
			Input:         []string{"testingenv", "--group", "test-resource-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupSuccess(mocks.ApplicationManagementClient, "test-resource-group")
			},
		},
		{
			Name:          "Create command with fallback workspace - requires resource group",
			Input:         []string{"testingenv"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Create command with invalid environment",
			Input:         []string{"testingenv", "-e", "testingenv"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Create command with invalid workspace",
			Input:         []string{"testingenv", "-w", "invalidworkspace"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Create command with extra positional arg",
			Input:         []string{"a", "b"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Create command with --kubernetes-namespace flag",
			Input:         []string{"testingenv", "--kubernetes-namespace", "mynamespace"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupSuccess(mocks.ApplicationManagementClient, "test-resource-group")
			},
			ValidateCallback: func(t *testing.T, runner framework.Runner) {
				r := runner.(*Runner)
				require.NotNil(t, r.providers)
				require.NotNil(t, r.providers.Kubernetes)
				require.NotNil(t, r.providers.Kubernetes.Namespace)
				require.Equal(t, "mynamespace", *r.providers.Kubernetes.Namespace)
			},
		},
		{
			Name:          "Create command with --namespace flag (legacy alias)",
			Input:         []string{"testingenv", "--namespace", "mynamespace"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupSuccess(mocks.ApplicationManagementClient, "test-resource-group")
			},
			ValidateCallback: func(t *testing.T, runner framework.Runner) {
				r := runner.(*Runner)
				require.NotNil(t, r.providers)
				require.NotNil(t, r.providers.Kubernetes)
				require.NotNil(t, r.providers.Kubernetes.Namespace)
				require.Equal(t, "mynamespace", *r.providers.Kubernetes.Namespace)
			},
		},
		{
			Name:          "Create command with invalid Kubernetes namespace",
			Input:         []string{"testingenv", "--kubernetes-namespace", "BadNamespace"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupSuccess(mocks.ApplicationManagementClient, "test-resource-group")
			},
		},
		{
			Name:          "Create command with Azure provider flags",
			Input:         []string{"testingenv", "--azure-subscription-id", "00000000-0000-0000-0000-000000000000", "--azure-resource-group", "testResourceGroup"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupSuccess(mocks.ApplicationManagementClient, "test-resource-group")
			},
			ValidateCallback: func(t *testing.T, runner framework.Runner) {
				r := runner.(*Runner)
				require.NotNil(t, r.providers)
				require.NotNil(t, r.providers.Azure)
				require.NotNil(t, r.providers.Azure.SubscriptionID)
				require.NotNil(t, r.providers.Azure.ResourceGroupName)
				require.Equal(t, "00000000-0000-0000-0000-000000000000", *r.providers.Azure.SubscriptionID)
				require.Equal(t, "testResourceGroup", *r.providers.Azure.ResourceGroupName)
			},
		},
		{
			Name:          "Create command with invalid Azure subscription ID",
			Input:         []string{"testingenv", "--azure-subscription-id", "not-a-guid", "--azure-resource-group", "testResourceGroup"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupSuccess(mocks.ApplicationManagementClient, "test-resource-group")
			},
		},
		{
			Name:          "Create command with AWS provider flags",
			Input:         []string{"testingenv", "--aws-region", "us-west-2", "--aws-account-id", "testAWSAccount"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupSuccess(mocks.ApplicationManagementClient, "test-resource-group")
			},
			ValidateCallback: func(t *testing.T, runner framework.Runner) {
				r := runner.(*Runner)
				require.NotNil(t, r.providers)
				require.NotNil(t, r.providers.Aws)
				require.NotNil(t, r.providers.Aws.Region)
				require.NotNil(t, r.providers.Aws.AccountID)
				require.Equal(t, "us-west-2", *r.providers.Aws.Region)
				require.Equal(t, "testAWSAccount", *r.providers.Aws.AccountID)
			},
		},
		{
			Name:          "Create command without --kubernetes-namespace defaults to \"default\"",
			Input:         []string{"testingenv"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupSuccess(mocks.ApplicationManagementClient, "test-resource-group")
			},
			ValidateCallback: func(t *testing.T, runner framework.Runner) {
				r := runner.(*Runner)
				require.NotNil(t, r.providers)
				require.NotNil(t, r.providers.Kubernetes)
				require.NotNil(t, r.providers.Kubernetes.Namespace)
				require.Equal(t, "default", *r.providers.Kubernetes.Namespace)
			},
		},
		{
			Name:          "Create command with Azure provider does not default the Kubernetes namespace",
			Input:         []string{"testingenv", "--azure-subscription-id", "00000000-0000-0000-0000-000000000000", "--azure-resource-group", "testResourceGroup"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupSuccess(mocks.ApplicationManagementClient, "test-resource-group")
			},
			ValidateCallback: func(t *testing.T, runner framework.Runner) {
				r := runner.(*Runner)
				require.NotNil(t, r.providers)
				require.NotNil(t, r.providers.Azure)
				require.Nil(t, r.providers.Kubernetes, "should not default Kubernetes namespace when Azure provider is configured")
			},
		},
		{
			Name:          "Create command with AWS provider does not default the Kubernetes namespace",
			Input:         []string{"testingenv", "--aws-region", "us-west-2", "--aws-account-id", "testAWSAccount"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				expectResourceGroupSuccess(mocks.ApplicationManagementClient, "test-resource-group")
			},
			ValidateCallback: func(t *testing.T, runner framework.Runner) {
				r := runner.(*Runner)
				require.NotNil(t, r.providers)
				require.NotNil(t, r.providers.Aws)
				require.Nil(t, r.providers.Kubernetes, "should not default Kubernetes namespace when AWS provider is configured")
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-resource-group",
		Connection: map[string]any{
			"kind":    "kubernetes",
			"context": "kind-kind",
		},
	}

	t.Run("creates environment with default recipe pack", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockAppClient := clients.NewMockApplicationsManagementClient(ctrl)
		mockAppClient.EXPECT().
			CreateOrUpdateResourceGroup(gomock.Any(), "local", "default", gomock.Any()).
			Return(nil).
			Times(1)

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			workspace.Scope,
			test_client_factory.WithEnvironmentServer404OnGet,
			nil,
		)
		require.NoError(t, err)

		// Default recipe pack is created in the default scope.
		defaultScopeFactory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			recipepack.DefaultResourceGroupScope,
			nil,
			nil,
		)
		require.NoError(t, err)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory:   factory,
			DefaultScopeClientFactory: defaultScopeFactory,
			ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: mockAppClient},
			Output:                    outputSink,
			Workspace:                 workspace,
			EnvironmentName:           "testenv",
			ResourceGroupName:         "test-resource-group",
		}

		expectedOutput := []any{
			output.LogOutput{
				Format: "Radius.Core/environments/%s created",
				Params: []any{
					"testenv",
				},
			},
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)

		require.Equal(t, expectedOutput, outputSink.Writes)
	})

	t.Run("creates default recipe pack when not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockAppClient := clients.NewMockApplicationsManagementClient(ctrl)
		mockAppClient.EXPECT().
			CreateOrUpdateResourceGroup(gomock.Any(), "local", "default", gomock.Any()).
			Return(nil).
			Times(1)

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			workspace.Scope,
			test_client_factory.WithEnvironmentServer404OnGet,
			nil,
		)
		require.NoError(t, err)

		// Default scope factory returns 404 on GET, succeeds on CreateOrUpdate.
		defaultScopeFactory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			recipepack.DefaultResourceGroupScope,
			nil,
			test_client_factory.WithRecipePackServer404OnGet,
		)
		require.NoError(t, err)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory:   factory,
			DefaultScopeClientFactory: defaultScopeFactory,
			ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: mockAppClient},
			Output:                    outputSink,
			Workspace:                 workspace,
			EnvironmentName:           "testenv",
			ResourceGroupName:         "test-resource-group",
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)
	})

	t.Run("returns error when default recipe pack GET fails with non-404", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockAppClient := clients.NewMockApplicationsManagementClient(ctrl)
		mockAppClient.EXPECT().
			CreateOrUpdateResourceGroup(gomock.Any(), "local", "default", gomock.Any()).
			Return(nil).
			Times(1)

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			workspace.Scope,
			test_client_factory.WithEnvironmentServer404OnGet,
			nil,
		)
		require.NoError(t, err)

		// Default scope factory returns 500 on GET.
		defaultScopeFactory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			recipepack.DefaultResourceGroupScope,
			nil,
			test_client_factory.WithRecipePackServerInternalError,
		)
		require.NoError(t, err)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory:   factory,
			DefaultScopeClientFactory: defaultScopeFactory,
			ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: mockAppClient},
			Output:                    outputSink,
			Workspace:                 workspace,
			EnvironmentName:           "testenv",
			ResourceGroupName:         "test-resource-group",
		}

		err = runner.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get default recipe pack from default scope")
	})

	// Without this defaulting, recipes that deploy Kubernetes resources fail with
	// "Namespace parameter required." when the environment is created without one.
	t.Run("sends default Kubernetes namespace on the created environment", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockAppClient := clients.NewMockApplicationsManagementClient(ctrl)
		mockAppClient.EXPECT().
			CreateOrUpdateResourceGroup(gomock.Any(), "local", "default", gomock.Any()).
			Return(nil).
			Times(1)

		// Capture the resource sent on CreateOrUpdate so we can assert the namespace.
		var capturedResource v20250801preview.EnvironmentResource
		capturingServer := func() corerpfake.EnvironmentsServer {
			return corerpfake.EnvironmentsServer{
				Get: func(
					ctx context.Context,
					environmentName string,
					options *v20250801preview.EnvironmentsClientGetOptions,
				) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
					errResp.SetResponseError(http.StatusNotFound, "Not Found")
					return
				},
				CreateOrUpdate: func(
					ctx context.Context,
					environmentName string,
					resource v20250801preview.EnvironmentResource,
					options *v20250801preview.EnvironmentsClientCreateOrUpdateOptions,
				) (resp azfake.Responder[v20250801preview.EnvironmentsClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
					capturedResource = resource
					result := v20250801preview.EnvironmentsClientCreateOrUpdateResponse{
						EnvironmentResource: resource,
					}
					resp.SetResponse(http.StatusOK, result, nil)
					return
				},
			}
		}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			workspace.Scope,
			capturingServer,
			nil,
		)
		require.NoError(t, err)

		defaultScopeFactory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			recipepack.DefaultResourceGroupScope,
			nil,
			nil,
		)
		require.NoError(t, err)

		// Validate() would normally populate r.providers; mimic that here for the
		// no-flag case by passing the default namespace explicitly.
		runner := &Runner{
			RadiusCoreClientFactory:   factory,
			DefaultScopeClientFactory: defaultScopeFactory,
			ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: mockAppClient},
			Output:                    &output.MockOutput{},
			Workspace:                 workspace,
			EnvironmentName:           "testenv",
			ResourceGroupName:         "test-resource-group",
			providers: &v20250801preview.Providers{
				Kubernetes: &v20250801preview.ProvidersKubernetes{
					Namespace: to.Ptr("default"),
				},
			},
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)

		require.NotNil(t, capturedResource.Properties)
		require.NotNil(t, capturedResource.Properties.Providers)
		require.NotNil(t, capturedResource.Properties.Providers.Kubernetes)
		require.NotNil(t, capturedResource.Properties.Providers.Kubernetes.Namespace)
		require.Equal(t, "default", *capturedResource.Properties.Providers.Kubernetes.Namespace)
	})
}

func expectResourceGroupSuccess(client *clients.MockApplicationsManagementClient, name string) {
	resource := radcli.CreateResourceGroup(name)
	client.EXPECT().
		GetResourceGroup(gomock.Any(), "local", name).
		Return(resource, nil).
		Times(1)
}

func expectResourceGroupNotFound(client *clients.MockApplicationsManagementClient, name string) {
	client.EXPECT().
		GetResourceGroup(gomock.Any(), "local", name).
		Return(v20231001preview.ResourceGroupResource{}, radcli.Create404Error()).
		Times(1)
}

func expectResourceGroupError(client *clients.MockApplicationsManagementClient, name string, err error) {
	client.EXPECT().
		GetResourceGroup(gomock.Any(), "local", name).
		Return(v20231001preview.ResourceGroupResource{}, err).
		Times(1)
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
