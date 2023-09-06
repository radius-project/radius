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

package radinit

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd/api"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/aws"
	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/connections"
	cli_credential "github.com/radius-project/radius/pkg/cli/credential"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/radcli"

	ucp "github.com/radius-project/radius/pkg/ucp/api/v20220901privatepreview"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	config := radcli.LoadConfigWithWorkspace(t)

	azureProvider := azure.Provider{
		SubscriptionID: "test-subscription-id",
		ResourceGroup:  "test-resource-group",
		ServicePrincipal: &azure.ServicePrincipal{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			TenantID:     "test-tenant-id",
		},
	}

	awsProvider := aws.Provider{
		Region:          "test-region",
		AccessKeyID:     "test-access-key-id",
		SecretAccessKey: "test-secret-access-key",
		AccountID:       "test-account-id",
	}

	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid Init --full Command",
			Input:         []string{"--full"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed, no reinstall
				initGetKubeContextSuccess(mocks.Kubernetes)
				initKubeContextWithKind(mocks.Prompter)
				initHelmMockRadiusInstalled(mocks.Helm)

				// No existing environment, users will be prompted to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{})

				// Use default env name and namespace
				initEnvNamePrompt(mocks.Prompter, "default")
				initNamespacePrompt(mocks.Prompter, "default")

				// No cloud providers
				initAddCloudProviderPromptNo(mocks.Prompter)

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)

				setConfirmOption(mocks.Prompter, resultConfimed)
			},
		},
		{
			Name:          "Valid Init --full Command Without Radius installed",
			Input:         []string{"--full"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed, no reinstall
				initGetKubeContextSuccess(mocks.Kubernetes)
				initKubeContextWithKind(mocks.Prompter)
				initHelmMockRadiusNotInstalled(mocks.Helm)

				// We do not prompt for reinstall if Radius is not yet installed

				// We do not check for existing environments if Radius is not installed

				// Use default env name and namespace
				initEnvNamePrompt(mocks.Prompter, "default")
				initNamespacePrompt(mocks.Prompter, "default")

				// No cloud providers
				initAddCloudProviderPromptNo(mocks.Prompter)

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)

				setConfirmOption(mocks.Prompter, resultConfimed)
			},
		},
		{
			Name:          "Initialize --full with existing environment, choose to create new",
			Input:         []string{"--full"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed, no reinstall
				initGetKubeContextSuccess(mocks.Kubernetes)
				initKubeContextWithKind(mocks.Prompter)
				initHelmMockRadiusInstalled(mocks.Helm)

				// Configure an existing environment - but then choose to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{
					{
						Name: to.Ptr("cool-existing-env"),
					},
				})
				initExistingEnvironmentSelection(mocks.Prompter, selectExistingEnvironmentCreateSentinel)

				// Use default env name and namespace
				initEnvNamePrompt(mocks.Prompter, "default")
				initNamespacePrompt(mocks.Prompter, "default")

				// No cloud providers
				initAddCloudProviderPromptNo(mocks.Prompter)

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)

				setConfirmOption(mocks.Prompter, resultConfimed)
			},
		},
		{
			Name:          "Initialize --full with existing environment, choose existing",
			Input:         []string{"--full"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed, no reinstall
				initGetKubeContextSuccess(mocks.Kubernetes)
				initKubeContextWithKind(mocks.Prompter)
				initHelmMockRadiusInstalled(mocks.Helm)

				// Configure an existing environment - but then choose to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{
					{
						Name: to.Ptr("cool-existing-env"),
					},
				})
				initExistingEnvironmentSelection(mocks.Prompter, "cool-existing-env")

				// No need to choose env settings since we're using existing

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)

				setConfirmOption(mocks.Prompter, resultConfimed)
			},
		},
		{
			Name:          "Initialize --full with existing environment, choose existing, with Cloud Providers",
			Input:         []string{"--full"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed, no reinstall
				initGetKubeContextSuccess(mocks.Kubernetes)
				initKubeContextWithKind(mocks.Prompter)
				initHelmMockRadiusInstalled(mocks.Helm)

				// Configure an existing environment - but then choose to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{
					{
						Name: to.Ptr("cool-existing-env"),
						Properties: &corerp.EnvironmentProperties{
							Providers: &corerp.Providers{
								Azure: &corerp.ProvidersAzure{
									Scope: to.Ptr("/subscriptions/123/resourceGroups/cool-rg"),
								},
								Aws: &corerp.ProvidersAws{
									Scope: to.Ptr("/planes/aws/aws/accounts/123/regions/us-west-2"),
								},
							},
						},
					},
				})
				initExistingEnvironmentSelection(mocks.Prompter, "cool-existing-env")

				// No need to choose env settings since we're using existing

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)

				setConfirmOption(mocks.Prompter, resultConfimed)
			},
		},
		{
			Name:          "Init --full Command With Azure Cloud Provider",
			Input:         []string{"--full"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed
				initGetKubeContextSuccess(mocks.Kubernetes)
				initKubeContextWithKind(mocks.Prompter)
				initHelmMockRadiusInstalled(mocks.Helm)

				// No existing environment, users will be prompted to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{})

				// Choose default name and namespace
				initEnvNamePrompt(mocks.Prompter, "default")
				initNamespacePrompt(mocks.Prompter, "default")

				// Add azure provider
				initAddCloudProviderPromptYes(mocks.Prompter)
				initSelectCloudProvider(mocks.Prompter, azure.ProviderDisplayName)
				setAzureCloudProvider(mocks.Prompter, mocks.AzureClient, azureProvider)

				// Don't add any other cloud providers
				initAddCloudProviderPromptNo(mocks.Prompter)

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)

				setConfirmOption(mocks.Prompter, resultConfimed)
			},
		},
		{
			Name:          "Init --full Command With AWS Cloud Provider",
			Input:         []string{"--full"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed
				initGetKubeContextSuccess(mocks.Kubernetes)
				initKubeContextWithKind(mocks.Prompter)
				initHelmMockRadiusInstalled(mocks.Helm)

				// No existing environment, users will be prompted to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{})

				// Choose default name and namespace
				initEnvNamePrompt(mocks.Prompter, "default")
				initNamespacePrompt(mocks.Prompter, "default")

				// Add aws provider
				initAddCloudProviderPromptYes(mocks.Prompter)
				initSelectCloudProvider(mocks.Prompter, aws.ProviderDisplayName)
				setAWSCloudProvider(mocks.Prompter, mocks.AWSClient, awsProvider)

				// Don't add any other cloud providers
				initAddCloudProviderPromptNo(mocks.Prompter)

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)

				setConfirmOption(mocks.Prompter, resultConfimed)
			},
		},
		{
			Name:          "Initialize --full with existing environment create application - initial appname is invalid",
			Input:         []string{"--full"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			CreateTempDirectory: "in.valid", // Invalid app name
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed, no reinstall
				initGetKubeContextSuccess(mocks.Kubernetes)
				initKubeContextWithKind(mocks.Prompter)
				initHelmMockRadiusInstalled(mocks.Helm)

				// Configure an existing environment - but then choose to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{
					{
						Name: to.Ptr("cool-existing-env"),
					},
				})
				initExistingEnvironmentSelection(mocks.Prompter, "cool-existing-env")

				// No need to choose env settings since we're using existing

				// Create Application
				setScaffoldApplicationPromptYes(mocks.Prompter)
				setApplicationNamePrompt(mocks.Prompter, "valid")

				setConfirmOption(mocks.Prompter, resultConfimed)
			},
		},
		{
			Name:          "rad init create new environment",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed, no reinstall
				initGetKubeContextSuccess(mocks.Kubernetes)
				initHelmMockRadiusInstalled(mocks.Helm)

				// No existing environment, users will be prompted to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{})

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)
			},
		},
		{
			Name:          "rad init without Radius installed",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed
				initGetKubeContextSuccess(mocks.Kubernetes)
				initHelmMockRadiusNotInstalled(mocks.Helm)

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)
			},
		},
		{
			Name:          "rad init chooses existing environment without default",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed, no reinstall
				initGetKubeContextSuccess(mocks.Kubernetes)
				initHelmMockRadiusInstalled(mocks.Helm)

				// Configure an existing environment - this will be chosen automatically
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{
					{
						Name: to.Ptr("myenv"),
					},
				})
				initExistingEnvironmentSelection(mocks.Prompter, "myenv")
				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)
			},
		},
		{
			Name:          "rad init chooses existing environment with default",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed, no reinstall
				initGetKubeContextSuccess(mocks.Kubernetes)
				initHelmMockRadiusInstalled(mocks.Helm)

				// Configure an existing environment - this will be chosen automatically
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{
					{
						Name: to.Ptr("default"),
					},
				})
				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)
			},
		},
		{
			Name:          "rad init prompts for existing environment",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed, no reinstall
				initGetKubeContextSuccess(mocks.Kubernetes)
				initHelmMockRadiusInstalled(mocks.Helm)

				// Configure an existing environment - user has to choose
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{
					{
						Name: to.Ptr("dev"),
					},
					{
						Name: to.Ptr("prod"),
					},
				})

				// prompt the user since there's no 'default'
				initExistingEnvironmentSelection(mocks.Prompter, "prod")

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)
			},
		},
		{
			Name:          "Init --full Command With Error KubeContext Read",
			Input:         []string{"--full"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Fail to read Kubernetes context
				initGetKubeContextError(mocks.Kubernetes)
			},
		},
		{
			Name:          "Init --full Command With Error KubeContext Selection",
			Input:         []string{"--full"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Cancel instead of choosing kubernetes context
				initGetKubeContextSuccess(mocks.Kubernetes)
				initKubeContextSelectionError(mocks.Prompter)
			},
		},
		{
			Name:          "Init --full Command With Error EnvName Read",
			Input:         []string{"--full"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed, no reinstall
				initGetKubeContextSuccess(mocks.Kubernetes)
				initKubeContextWithKind(mocks.Prompter)
				initHelmMockRadiusInstalled(mocks.Helm)

				// No existing environment, users will be prompted to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{})

				// User cancels from environment name prompt
				initEnvNamePromptError(mocks.Prompter)
			},
		},
		{
			Name:          "Init --full Command With Error Namespace Read",
			Input:         []string{"--full"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed, no reinstall
				initGetKubeContextSuccess(mocks.Kubernetes)
				initKubeContextWithKind(mocks.Prompter)
				initHelmMockRadiusInstalled(mocks.Helm)

				// No existing environment, users will be prompted to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{})

				// Choose default name and cancel out of namespace prompt
				initEnvNamePrompt(mocks.Prompter, "default")
				initNamespacePromptError(mocks.Prompter)
			},
		},
		{
			Name:          "Init --full Command Navigate back while configuring cloud provider",
			Input:         []string{"--full"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed, no reinstall
				initGetKubeContextSuccess(mocks.Kubernetes)
				initKubeContextWithKind(mocks.Prompter)
				initHelmMockRadiusInstalled(mocks.Helm)
				// No existing environment, users will be prompted to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{})

				// Choose default name and namespace
				initEnvNamePrompt(mocks.Prompter, "default")
				initNamespacePrompt(mocks.Prompter, "default")

				// Oops! I don't need to add cloud provider, navigate back to reinstall prompt
				initAddCloudProviderPromptYes(mocks.Prompter)
				initSelectCloudProvider(mocks.Prompter, confirmCloudProviderBackNavigationSentinel)

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)

				setConfirmOption(mocks.Prompter, resultConfimed)
			},
		},
		{
			Name:          "Init --full Command exit console with interrupt signal",
			Input:         []string{"--full"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         config,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Radius is already installed, no reinstall
				initGetKubeContextSuccess(mocks.Kubernetes)
				initKubeContextWithInterruptSignal(mocks.Prompter)
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run_InstallAndCreateEnvironment(t *testing.T) {
	testCases := []struct {
		name           string
		full           bool
		azureProvider  *azure.Provider
		awsProvider    *aws.Provider
		recipes        map[string]map[string]corerp.RecipePropertiesClassification
		expectedOutput []any
	}{
		{
			name:          "`rad init` with recipes",
			full:          false,
			azureProvider: nil,
			awsProvider:   nil,
			recipes: map[string]map[string]corerp.RecipePropertiesClassification{
				"Applications.Datastores/redisCaches": {
					"default": &corerp.BicepRecipeProperties{
						TemplateKind: to.Ptr(recipes.TemplateKindBicep),
						TemplatePath: to.Ptr("radiusdev.azurecr.io/redis:latest"),
					},
				},
			},
		},
		{
			name:           "`rad init` w/o recipes",
			full:           false,
			azureProvider:  nil,
			awsProvider:    nil,
			recipes:        map[string]map[string]corerp.RecipePropertiesClassification{},
			expectedOutput: []any{},
		},
		{
			name: "`rad init --full` with Azure Provider",
			full: true,
			azureProvider: &azure.Provider{
				SubscriptionID: "test-subscription",
				ResourceGroup:  "test-rg",
				ServicePrincipal: &azure.ServicePrincipal{
					TenantID:     "test-tenantId",
					ClientID:     "test-clientId",
					ClientSecret: "test-clientSecret",
				},
			},
			awsProvider:    nil,
			recipes:        nil,
			expectedOutput: []any{},
		},
		{
			name:          "`rad init` with AWS Provider",
			full:          false,
			azureProvider: nil,
			awsProvider: &aws.Provider{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-access",
				Region:          "us-west-2",
				AccountID:       "test-account-id",
			},
			recipes:        map[string]map[string]corerp.RecipePropertiesClassification{},
			expectedOutput: []any{},
		},
		{
			name:          "`rad init --full` with AWS Provider",
			full:          true,
			azureProvider: nil,
			awsProvider: &aws.Provider{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-access",
				Region:          "us-west-2",
				AccountID:       "test-account-id",
			},
			recipes:        nil,
			expectedOutput: []any{},
		},
		{
			name:           "`rad init --full` with no providers",
			full:           true,
			azureProvider:  nil,
			awsProvider:    nil,
			recipes:        nil,
			expectedOutput: []any{},
		},
		{
			name:           "`rad init` with no providers",
			full:           false,
			azureProvider:  nil,
			awsProvider:    nil,
			recipes:        map[string]map[string]corerp.RecipePropertiesClassification{},
			expectedOutput: []any{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			configFileInterface := framework.NewMockConfigFileInterface(ctrl)
			configFileInterface.EXPECT().
				ConfigFromContext(context.Background()).
				Return(nil).
				Times(1)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				CreateUCPGroup(context.Background(), "radius", "local", "default", gomock.Any()).
				Return(nil).
				Times(1)

			devRecipeClient := NewMockDevRecipeClient(ctrl)
			if !tc.full {
				devRecipeClient.EXPECT().
					GetDevRecipes(context.Background()).
					Return(tc.recipes, nil).
					Times(1)
			}

			testEnvProperties := &corerp.EnvironmentProperties{
				Compute: &corerp.KubernetesCompute{
					Namespace: to.Ptr("defaultNamespace"),
				},
				Providers: buildProviders(tc.azureProvider, tc.awsProvider),
				Recipes:   tc.recipes,
			}
			appManagementClient.EXPECT().
				CreateEnvironment(context.Background(), "default", v1.LocationGlobal, testEnvProperties).
				Return(nil).
				Times(1)

			credentialManagementClient := cli_credential.NewMockCredentialManagementClient(ctrl)
			if tc.azureProvider != nil {
				credentialManagementClient.EXPECT().
					PutAzure(context.Background(), gomock.Any()).
					Return(nil).
					Times(1)
			}
			if tc.awsProvider != nil {
				credentialManagementClient.EXPECT().
					PutAWS(context.Background(), ucp.AwsCredentialResource{
						Location: to.Ptr(v1.LocationGlobal),
						Type:     to.Ptr(cli_credential.AWSCredential),
						Properties: &ucp.AwsAccessKeyCredentialProperties{
							Storage: &ucp.CredentialStorageProperties{
								Kind: to.Ptr(ucp.CredentialStorageKindInternal),
							},
							AccessKeyID:     to.Ptr(tc.awsProvider.AccessKeyID),
							SecretAccessKey: to.Ptr(tc.awsProvider.SecretAccessKey),
						},
					}).
					Return(nil).
					Times(1)
			}

			configFileInterface.EXPECT().
				EditWorkspaces(context.Background(), gomock.Any(), gomock.Any()).
				Return(nil).
				Times(1)

			outputSink := &output.MockOutput{}

			helmInterface := helm.NewMockInterface(ctrl)
			helmInterface.EXPECT().
				InstallRadius(context.Background(), gomock.Any(), "kind-kind").
				Return(true, nil).
				Times(1)

			prompter := prompt.NewMockInterface(ctrl)
			setProgressHandler(prompter)

			options := initOptions{
				Cluster: clusterOptions{
					Install: true,
					Context: "kind-kind",
				},
				Environment: environmentOptions{
					Create:    true,
					Name:      "default",
					Namespace: "defaultNamespace",
				},
				CloudProviders: cloudProviderOptions{
					Azure: tc.azureProvider,
					AWS:   tc.awsProvider,
				},
				Recipes: recipePackOptions{
					DevRecipes: !tc.full,
				},
				Application: applicationOptions{
					Scaffold: false,
				},
			}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{
					ApplicationsManagementClient: appManagementClient,
					CredentialManagementClient:   credentialManagementClient,
				},
				ConfigFileInterface: configFileInterface,
				ConfigHolder:        &framework.ConfigHolder{ConfigFilePath: "filePath"},
				HelmInterface:       helmInterface,
				Output:              outputSink,
				Prompter:            prompter,
				DevRecipeClient:     devRecipeClient,
				Options:             &options,
				Workspace: &workspaces.Workspace{
					Name: "default",
				},
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)

			if len(tc.expectedOutput) == 0 {
				require.Len(t, outputSink.Writes, 0)
			} else {
				require.Equal(t, tc.expectedOutput, outputSink.Writes)
			}
		})
	}
}

func buildProviders(azureProvider *azure.Provider, awsProvider *aws.Provider) *corerp.Providers {
	providers := &corerp.Providers{}
	if azureProvider != nil {
		providers.Azure = &corerp.ProvidersAzure{
			Scope: to.Ptr(fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", azureProvider.SubscriptionID, azureProvider.ResourceGroup)),
		}
	}
	if awsProvider != nil {
		providers.Aws = &corerp.ProvidersAws{
			Scope: to.Ptr(fmt.Sprintf("/planes/aws/aws/accounts/%s/regions/%s", awsProvider.AccountID, awsProvider.Region)),
		}
	}
	return providers
}

func initGetKubeContextSuccess(kubernestesMock *kubernetes.MockInterface) {
	kubernestesMock.EXPECT().
		GetKubeContext().
		Return(getTestKubeConfig(), nil).Times(1)
}

func initGetKubeContextError(kubernestesMock *kubernetes.MockInterface) {
	kubernestesMock.EXPECT().
		GetKubeContext().
		Return(nil, errors.New("unable to fetch kube context")).Times(1)
}

func getTestKubeConfig() *api.Config {
	kubeContexts := map[string]*api.Context{
		"docker-desktop": {Cluster: "docker-desktop"},
		"k3d-radius-dev": {Cluster: "k3d-radius-dev"},
		"kind-kind":      {Cluster: "kind-kind"},
	}
	return &api.Config{
		CurrentContext: "kind-kind",
		Contexts:       kubeContexts,
	}
}

func initKubeContextWithKind(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), selectClusterPrompt).
		Return("kind-kind", nil).Times(1)
}

func initKubeContextSelectionError(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), selectClusterPrompt).
		Return("", errors.New("cannot read selection")).Times(1)
}

func initKubeContextWithInterruptSignal(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), selectClusterPrompt).
		Return("", &prompt.ErrExitConsole{}).Times(1)
}

func initEnvNamePrompt(prompter *prompt.MockInterface, name string) {
	prompter.EXPECT().
		GetTextInput(enterEnvironmentNamePrompt, gomock.Any()).
		Return(name, nil).Times(1)
}

func initEnvNamePromptError(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetTextInput(enterEnvironmentNamePrompt, gomock.Any()).
		Return("", errors.New("unable to read prompt")).Times(1)
}

func initNamespacePrompt(prompter *prompt.MockInterface, namespace string) {
	prompter.EXPECT().
		GetTextInput(enterNamespacePrompt, gomock.Any()).
		Return(namespace, nil).Times(1)
}

func initNamespacePromptError(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetTextInput(enterNamespacePrompt, gomock.Any()).
		Return("", errors.New("Unable to read namespace")).Times(1)
}

var _ gomock.Matcher = &cloudProviderPromptMatcher{}

type cloudProviderPromptMatcher struct {
}

// Matches implements gomock.Matcher
func (*cloudProviderPromptMatcher) Matches(x interface{}) bool {
	return x == confirmCloudProviderPrompt || x == confirmCloudProviderAdditionalPrompt
}

// String implements gomock.Matcher
func (*cloudProviderPromptMatcher) String() string {
	return fmt.Sprintf("Matches either: %s or %s", confirmCloudProviderPrompt, confirmCloudProviderAdditionalPrompt)
}

func initAddCloudProviderPromptNo(prompter *prompt.MockInterface) {
	// We show a different prompt the second time with different phrasing.
	prompter.EXPECT().
		GetListInput(gomock.Any(), &cloudProviderPromptMatcher{}).
		Return(prompt.ConfirmNo, nil).Times(1)
}

func initAddCloudProviderPromptYes(prompter *prompt.MockInterface) {
	// We show a different prompt the second time with different phrasing.
	prompter.EXPECT().
		GetListInput(gomock.Any(), &cloudProviderPromptMatcher{}).
		Return(prompt.ConfirmYes, nil).Times(1)
}

func initSelectCloudProvider(prompter *prompt.MockInterface, value string) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), selectCloudProviderPrompt).
		Return(value, nil).Times(1)
}

func initHelmMockRadiusInstalled(helmMock *helm.MockInterface) {
	helmMock.EXPECT().
		CheckRadiusInstall(gomock.Any()).
		Return(helm.InstallState{Installed: true, Version: "test-version"}, nil).Times(1)
}

func initHelmMockRadiusNotInstalled(helmMock *helm.MockInterface) {
	helmMock.EXPECT().
		CheckRadiusInstall(gomock.Any()).
		Return(helm.InstallState{Installed: false}, nil).Times(1)
}

func setExistingEnvironments(clientMock *clients.MockApplicationsManagementClient, environments []corerp.EnvironmentResource) {
	clientMock.EXPECT().
		ListEnvironmentsAll(gomock.Any()).
		Return(environments, nil).Times(1)
}

func initExistingEnvironmentSelection(prompter *prompt.MockInterface, choice string) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), selectExistingEnvironmentPrompt).
		Return(choice, nil).Times(1)
}

func setScaffoldApplicationPromptNo(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), confirmSetupApplicationPrompt).
		Return(prompt.ConfirmNo, nil).Times(1)
}

func setScaffoldApplicationPromptYes(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), confirmSetupApplicationPrompt).
		Return(prompt.ConfirmYes, nil).Times(1)
}

func setApplicationNamePrompt(prompter *prompt.MockInterface, applicationName string) {
	prompter.EXPECT().
		GetTextInput(enterApplicationNamePrompt, gomock.Any()).
		Return(applicationName, nil).Times(1)
}

func setAWSRegionPrompt(prompter *prompt.MockInterface, regions []string, region string) {
	prompter.EXPECT().
		GetListInput(regions, selectAWSRegionPrompt).
		Return(region, nil).
		Times(1)
}

func setAWSAccessKeyIDPrompt(prompter *prompt.MockInterface, accessKeyID string) {
	prompter.EXPECT().
		GetTextInput(enterAWSIAMAcessKeyIDPrompt, gomock.Any()).
		Return(accessKeyID, nil).Times(1)
}

func setAWSSecretAccessKeyPrompt(prompter *prompt.MockInterface, secretAccessKey string) {
	prompter.EXPECT().
		GetTextInput(enterAWSIAMSecretAccessKeyPrompt, gomock.Any()).
		Return(secretAccessKey, nil).Times(1)
}

func setAWSCallerIdentity(client *aws.MockClient, region string, accessKeyID string, secretAccessKey string, callerIdentityOutput *sts.GetCallerIdentityOutput) {
	client.EXPECT().
		GetCallerIdentity(gomock.Any(), region, accessKeyID, secretAccessKey).
		Return(callerIdentityOutput, nil).
		Times(1)
}

func setAWSListRegions(client *aws.MockClient, region string, accessKeyID string, secretAccessKey string, ec2DescribeRegionsOutput *ec2.DescribeRegionsOutput) {
	client.EXPECT().
		ListRegions(gomock.Any(), region, accessKeyID, secretAccessKey).
		Return(ec2DescribeRegionsOutput, nil).
		Times(1)
}

// setAWSCloudProvider sets up mocks that will configure an AWS cloud provider.
func setAWSCloudProvider(prompter *prompt.MockInterface, client *aws.MockClient, provider aws.Provider) {
	setAWSAccessKeyIDPrompt(prompter, provider.AccessKeyID)
	setAWSSecretAccessKeyPrompt(prompter, provider.SecretAccessKey)
	setAWSCallerIdentity(client, QueryRegion, provider.AccessKeyID, provider.SecretAccessKey, &sts.GetCallerIdentityOutput{Account: &provider.AccountID})
	setAWSListRegions(client, QueryRegion, provider.AccessKeyID, provider.SecretAccessKey, &ec2.DescribeRegionsOutput{Regions: getMockAWSRegions()})
	setAWSRegionPrompt(prompter, getMockAWSRegionsString(), provider.Region)
}

func setAzureSubscriptions(client *azure.MockClient, result *azure.SubscriptionResult) {
	client.EXPECT().
		Subscriptions(gomock.Any()).
		Return(result, nil).
		Times(1)
}

func setAzureResourceGroups(client *azure.MockClient, subscriptionID string, groups []armresources.ResourceGroup) {
	client.EXPECT().
		ResourceGroups(gomock.Any(), subscriptionID).
		Return(groups, nil).
		Times(1)
}

func setAzureCheckResourceGroupExistence(client *azure.MockClient, subscriptionID string, resourceGroupName string, exists bool) {
	client.EXPECT().
		CheckResourceGroupExistence(gomock.Any(), subscriptionID, resourceGroupName).
		Return(exists, nil).
		Times(1)
}

func setAzureCreateOrUpdateResourceGroup(client *azure.MockClient, subscriptionID string, resourceGroupName string, location string) {
	client.EXPECT().
		CreateOrUpdateResourceGroup(gomock.Any(), subscriptionID, resourceGroupName, location).
		Return(nil).
		Times(1)
}

func setAzureLocations(client *azure.MockClient, subscriptionID string, locations []armsubscriptions.Location) {
	client.EXPECT().
		Locations(gomock.Any(), subscriptionID).
		Return(locations, nil).
		Times(1)
}

func setAzureSubscriptionConfirmPrompt(prompter *prompt.MockInterface, subscriptionName string, choice string) {
	prompter.EXPECT().
		GetListInput([]string{prompt.ConfirmYes, prompt.ConfirmNo}, fmt.Sprintf(confirmAzureSubscriptionPromptFmt, subscriptionName)).
		Return(choice, nil).
		Times(1)
}

func setAzureSubsubscriptionPrompt(prompter *prompt.MockInterface, names []string, name string) {
	prompter.EXPECT().
		GetListInput(names, selectAzureSubscriptionPrompt).
		Return(name, nil).
		Times(1)
}

func setAzureResourceGroupCreatePrompt(prompter *prompt.MockInterface, choice string) {
	prompter.EXPECT().
		GetListInput([]string{prompt.ConfirmYes, prompt.ConfirmNo}, confirmAzureCreateResourceGroupPrompt).
		Return(choice, nil).
		Times(1)
}

func setAzureResourceGroupPrompt(prompter *prompt.MockInterface, names []string, name string) {
	prompter.EXPECT().
		GetListInput(names, selectAzureResourceGroupPrompt).
		Return(name, nil).
		Times(1)
}

func setAzureResourceGroupNamePrompt(prompter *prompt.MockInterface, name string) {
	prompter.EXPECT().
		GetTextInput(enterAzureResourceGroupNamePrompt, gomock.Any()).
		Return(name, nil).
		Times(1)
}

func setSelectAzureResourceGroupLocationPrompt(prompter *prompt.MockInterface, locations []string, location string) {
	prompter.EXPECT().
		GetListInput(locations, selectAzureResourceGroupLocationPrompt).
		Return(location, nil).
		Times(1)
}

func setAzureServicePrincipalAppIDPrompt(prompter *prompt.MockInterface, appID string) {
	prompter.EXPECT().
		GetTextInput(enterAzureServicePrincipalAppIDPrompt, gomock.Any()).
		Return(appID, nil).
		Times(1)
}

func setAzureServicePrincipalPasswordPrompt(prompter *prompt.MockInterface, password string) {
	prompter.EXPECT().
		GetTextInput(enterAzureServicePrincipalPasswordPrompt, gomock.Any()).
		Return(password, nil).
		Times(1)
}

func setAzureServicePrincipalTenantIDPrompt(prompter *prompt.MockInterface, tenantID string) {
	prompter.EXPECT().
		GetTextInput(enterAzureServicePrincipalTenantIDPrompt, gomock.Any()).
		Return(tenantID, nil).
		Times(1)
}

// setAzureCloudProvider sets up mocks that will configure an Azure cloud provider.
func setAzureCloudProvider(prompter *prompt.MockInterface, client *azure.MockClient, provider azure.Provider) {
	subscriptions := &azure.SubscriptionResult{
		Subscriptions: []azure.Subscription{{ID: provider.SubscriptionID, Name: "test-subscription"}},
	}
	subscriptions.Default = &subscriptions.Subscriptions[0]
	resourceGroups := []armresources.ResourceGroup{{Name: to.Ptr(provider.ResourceGroup)}}

	setAzureSubscriptions(client, subscriptions)
	setAzureSubscriptionConfirmPrompt(prompter, subscriptions.Default.Name, prompt.ConfirmYes)

	setAzureResourceGroupCreatePrompt(prompter, prompt.ConfirmNo)
	setAzureResourceGroups(client, provider.SubscriptionID, resourceGroups)
	setAzureResourceGroupPrompt(prompter, []string{provider.ResourceGroup}, provider.ResourceGroup)

	setAzureServicePrincipalAppIDPrompt(prompter, provider.ServicePrincipal.ClientID)
	setAzureServicePrincipalPasswordPrompt(prompter, provider.ServicePrincipal.ClientSecret)
	setAzureServicePrincipalTenantIDPrompt(prompter, provider.ServicePrincipal.TenantID)
}

func setConfirmOption(prompter *prompt.MockInterface, choice summaryResult) {
	prompter.EXPECT().
		RunProgram(gomock.Any()).
		Return(&summaryModel{result: choice}, nil).
		Times(1)
}

func setProgressHandler(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunProgram(gomock.Any()).
		DoAndReturn(func(program *tea.Program) (tea.Model, error) {
			program.Kill() // Quit the program immediately
			return &progressModel{}, nil
		}).
		Times(1)
}

func getMockAWSRegions() []ec2_types.Region {
	return []ec2_types.Region{
		{RegionName: to.Ptr("test-region")},
		{RegionName: to.Ptr("test-region-2")},
	}
}

func getMockAWSRegionsString() []string {
	return []string{"test-region", "test-region-2"}
}
