// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radInit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd/api"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/cmd/credential/common"
	"github.com/project-radius/radius/pkg/cli/connections"
	cli_credential "github.com/project-radius/radius/pkg/cli/credential"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	config := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid Init Command",
			Input:         []string{},
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
				initRadiusReinstallNo(mocks.Prompter)

				// No existing environment, users will be prompted to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{})

				// Use default env name and namespace
				initEnvNamePrompt(mocks.Prompter)
				initNamespacePrompt(mocks.Prompter)

				// No cloud providers
				initAddCloudProviderPromptNo(mocks.Prompter)

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)
			},
		},
		{
			Name:          "Valid Init Command Without Radius installed",
			Input:         []string{},
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
				initEnvNamePrompt(mocks.Prompter)
				initNamespacePrompt(mocks.Prompter)

				// No cloud providers
				initAddCloudProviderPromptNo(mocks.Prompter)

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)
			},
		},
		{
			Name:          "Initialize with existing environment, choose to create new",
			Input:         []string{},
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
				initRadiusReinstallNo(mocks.Prompter)

				// Configure an existing environment - but then choose to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{
					{
						Name: to.Ptr("cool-existing-env"),
					},
				})
				initExistingEnvironmentSelection(mocks.Prompter, common.SelectExistingEnvironmentCreateSentinel)

				// Use default env name and namespace
				initEnvNamePrompt(mocks.Prompter)
				initNamespacePrompt(mocks.Prompter)

				// No cloud providers
				initAddCloudProviderPromptNo(mocks.Prompter)

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)
			},
		},
		{
			Name:          "Initialize with existing environment, choose existing",
			Input:         []string{},
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
				initRadiusReinstallNo(mocks.Prompter)

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
			},
		},
		{
			Name:          "Init Command With Cloud Provider (Reinstall)",
			Input:         []string{},
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

				// Reinstall
				initRadiusReinstallYes(mocks.Prompter)

				// No existing environment, users will be prompted to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{})

				// Choose default name and namespace
				initEnvNamePrompt(mocks.Prompter)
				initNamespacePrompt(mocks.Prompter)

				// Add azure provider
				initAddCloudProviderPromptYes(mocks.Prompter)
				initSelectCloudProvider(mocks.Prompter, "Azure")
				initParseCloudProvider(mocks.Setup, mocks.Prompter)

				// Don't add any other cloud providers
				initAddCloudProviderPromptNo(mocks.Prompter)

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)
			},
		},
		{
			Name:          "Initialize with existing environment create application - initial appname is invalid",
			Input:         []string{},
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
				initRadiusReinstallNo(mocks.Prompter)

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
			},
		},
		{
			Name:          "rad init --dev create new environment",
			Input:         []string{"--dev"},
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
			Name:          "rad init --dev without Radius installed",
			Input:         []string{"--dev"},
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
			Name:          "rad init --dev chooses existing environment",
			Input:         []string{"--dev"},
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
			Name:          "rad init --dev prompts for existing environment",
			Input:         []string{"--dev"},
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
			Name:          "Init Command With Error KubeContext Read",
			Input:         []string{},
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
			Name:          "Init Command With Error KubeContext Selection",
			Input:         []string{},
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
			Name:          "Init Command With Error EnvName Read",
			Input:         []string{},
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
				initRadiusReinstallNo(mocks.Prompter)

				// No existing environment, users will be prompted to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{})

				// User cancels from environment name prompt
				initEnvNamePromptError(mocks.Prompter)
			},
		},
		{
			Name:          "Init Command With Error Namespace Read",
			Input:         []string{},
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
				initRadiusReinstallNo(mocks.Prompter)

				// No existing environment, users will be prompted to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{})

				// Choose default name and cancel out of namespace prompt
				initEnvNamePrompt(mocks.Prompter)
				initNamespacePromptError(mocks.Prompter)
			},
		},
		{
			Name:          "Init Command Navigate back while configuring cloud provider",
			Input:         []string{},
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
				// Reinstall radius to configure cloud provider
				initRadiusReinstallYes(mocks.Prompter)
				initAddCloudProviderPromptYes(mocks.Prompter)
				// No existing environment, users will be prompted to create a new one
				setExistingEnvironments(mocks.ApplicationManagementClient, []corerp.EnvironmentResource{})

				// Choose default name and namespace
				initEnvNamePrompt(mocks.Prompter)
				initNamespacePrompt(mocks.Prompter)
				// Oops! I don't need to add cloud provider, navigate back to reinstall prompt
				initSelectCloudProvider(mocks.Prompter, "[back]")
				initAddCloudProviderPromptNo(mocks.Prompter)

				// No application
				setScaffoldApplicationPromptNo(mocks.Prompter)
			},
		},
		{
			Name:          "Init Command exit console with interrupt signal",
			Input:         []string{},
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

func Test_Run_InstallAndCreateEnvironment_WithAzureProvider_WithRecipes(t *testing.T) {
	ctrl := gomock.NewController(t)
	configFileInterface := framework.NewMockConfigFileInterface(ctrl)
	configFileInterface.EXPECT().
		ConfigFromContext(context.Background()).
		Return(nil).Times(1)

	appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
	appManagementClient.EXPECT().
		CreateUCPGroup(context.Background(), "radius", "local", "default", gomock.Any()).
		Return(true, nil).Times(1)
	appManagementClient.EXPECT().
		CreateUCPGroup(context.Background(), "deployments", "local", "default", gomock.Any()).
		Return(true, nil).Times(1)
	skipRecipes := false
	testEnvProperties := &corerp.EnvironmentProperties{
		Compute: &corerp.KubernetesCompute{
			Namespace: to.Ptr("defaultNamespace"),
		},
		UseDevRecipes: to.Ptr(!skipRecipes),
		Providers: &corerp.Providers{
			Azure: &corerp.ProvidersAzure{
				Scope: to.Ptr("/subscriptions/test-subscription/resourceGroups/test-rg"),
			},
		},
	}
	appManagementClient.EXPECT().
		CreateEnvironment(context.Background(), "default", v1.LocationGlobal, testEnvProperties).
		Return(true, nil).Times(1)

	credentialManagementClient := cli_credential.NewMockCredentialManagementClient(ctrl)
	credentialManagementClient.EXPECT().
		Put(context.Background(), gomock.Any()).
		Return(nil).Times(1)

	configFileInterface.EXPECT().
		EditWorkspaces(context.Background(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).Times(1)

	outputSink := &output.MockOutput{}

	helmInterface := helm.NewMockInterface(ctrl)
	helmInterface.EXPECT().
		InstallRadius(context.Background(), gomock.Any(), "kind-kind").
		Return(true, nil).Times(1)

	runner := &Runner{
		ConnectionFactory: &connections.MockFactory{
			ApplicationsManagementClient: appManagementClient,
			CredentialManagementClient:   credentialManagementClient,
		},
		ConfigFileInterface: configFileInterface,
		ConfigHolder:        &framework.ConfigHolder{ConfigFilePath: "filePath"},
		HelmInterface:       helmInterface,
		Output:              outputSink,
		Workspace:           &workspaces.Workspace{Name: "defaultWorkspace"},
		KubeContext:         "kind-kind",
		EnvName:             "default",
		Namespace:           "defaultNamespace",
		RadiusInstalled:     true, // We're testing the reinstall case
		Reinstall:           true,
		SkipDevRecipes:      skipRecipes,
		AzureCloudProvider: &azure.Provider{
			SubscriptionID: "test-subscription",
			ResourceGroup:  "test-rg",
			ServicePrincipal: &azure.ServicePrincipal{
				TenantID:     "test-tenantId",
				ClientID:     "test-clientId",
				ClientSecret: "test-clientSecret",
			},
		},
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)
}

func Test_Run_InstallAndCreateEnvironment_WithAWSProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	configFileInterface := framework.NewMockConfigFileInterface(ctrl)
	configFileInterface.EXPECT().
		ConfigFromContext(context.Background()).
		Return(nil).Times(1)

	appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
	appManagementClient.EXPECT().
		CreateUCPGroup(context.Background(), "radius", "local", "default", gomock.Any()).
		Return(true, nil).Times(1)
	appManagementClient.EXPECT().
		CreateUCPGroup(context.Background(), "deployments", "local", "default", gomock.Any()).
		Return(true, nil).Times(1)
	testEnvProperties := &corerp.EnvironmentProperties{
		Compute: &corerp.KubernetesCompute{
			Namespace: to.Ptr("defaultNamespace"),
		},
		UseDevRecipes: to.Ptr(true),
		Providers: &corerp.Providers{
			Aws: &corerp.ProvidersAws{
				Scope: to.Ptr("/planes/aws/aws/accounts/test-account-id/regions/us-west-2"),
			},
		},
	}
	appManagementClient.EXPECT().
		CreateEnvironment(context.Background(), "default", v1.LocationGlobal, testEnvProperties).
		Return(true, nil).Times(1)

	credentialManagementClient := cli_credential.NewMockCredentialManagementClient(ctrl)
	credentialManagementClient.EXPECT().
		Put(context.Background(), gomock.Any()).
		Return(nil).Times(1)

	configFileInterface.EXPECT().
		EditWorkspaces(context.Background(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).Times(1)

	outputSink := &output.MockOutput{}

	helmInterface := helm.NewMockInterface(ctrl)
	helmInterface.EXPECT().
		InstallRadius(context.Background(), gomock.Any(), "kind-kind").
		Return(true, nil).Times(1)

	runner := &Runner{
		ConnectionFactory: &connections.MockFactory{
			ApplicationsManagementClient: appManagementClient,
			CredentialManagementClient:   credentialManagementClient,
		},
		ConfigFileInterface: configFileInterface,
		ConfigHolder:        &framework.ConfigHolder{ConfigFilePath: "filePath"},
		HelmInterface:       helmInterface,
		Output:              outputSink,
		Workspace:           &workspaces.Workspace{Name: "defaultWorkspace"},
		KubeContext:         "kind-kind",
		EnvName:             "default",
		Namespace:           "defaultNamespace",
		RadiusInstalled:     true, // We're testing the reinstall case
		Reinstall:           true,
		AwsCloudProvider: &aws.Provider{
			AccessKeyId:     "test-access-key",
			SecretAccessKey: "test-secret-access",
			TargetRegion:    "us-west-2",
			AccountId:       "test-account-id",
		},
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)
}

func Test_Run_InstallAndCreateEnvironment_WithoutAzureProvider_WithSkipRecipes(t *testing.T) {
	ctrl := gomock.NewController(t)
	configFileInterface := framework.NewMockConfigFileInterface(ctrl)
	configFileInterface.EXPECT().
		ConfigFromContext(context.Background()).
		Return(nil).Times(1)

	appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
	appManagementClient.EXPECT().
		CreateUCPGroup(context.Background(), "radius", "local", "default", gomock.Any()).
		Return(true, nil).Times(1)
	appManagementClient.EXPECT().
		CreateUCPGroup(context.Background(), "deployments", "local", "default", gomock.Any()).
		Return(true, nil).Times(1)
	skipRecipes := true
	testEnvProperties := &corerp.EnvironmentProperties{
		Compute: &corerp.KubernetesCompute{
			Namespace: to.Ptr("defaultNamespace"),
		},
		UseDevRecipes: to.Ptr(!skipRecipes),
		Providers:     &corerp.Providers{},
	}
	appManagementClient.EXPECT().
		CreateEnvironment(context.Background(), "default", v1.LocationGlobal, testEnvProperties).
		Return(true, nil).Times(1)

	configFileInterface.EXPECT().
		EditWorkspaces(context.Background(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).Times(1)

	outputSink := &output.MockOutput{}

	helmInterface := helm.NewMockInterface(ctrl)
	helmInterface.EXPECT().
		InstallRadius(context.Background(), gomock.Any(), "kind-kind").
		Return(true, nil).Times(1)

	runner := &Runner{
		ConnectionFactory:   &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
		ConfigFileInterface: configFileInterface,
		ConfigHolder:        &framework.ConfigHolder{ConfigFilePath: "filePath"},
		HelmInterface:       helmInterface,
		Output:              outputSink,
		Workspace:           &workspaces.Workspace{Name: "defaultWorkspace"},
		KubeContext:         "kind-kind",
		SkipDevRecipes:      skipRecipes,
		RadiusInstalled:     false,
		Namespace:           "defaultNamespace",
		EnvName:             "default",
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)
}

func Test_Run_InstalledRadiusExistingEnvironment(t *testing.T) {
	ctrl := gomock.NewController(t)
	configFileInterface := framework.NewMockConfigFileInterface(ctrl)
	configFileInterface.EXPECT().
		ConfigFromContext(context.Background()).
		Return(nil).Times(1)

	configFileInterface.EXPECT().
		EditWorkspaces(context.Background(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).Times(1)

	outputSink := &output.MockOutput{}

	runner := &Runner{
		ConnectionFactory:   &connections.MockFactory{},
		ConfigFileInterface: configFileInterface,
		ConfigHolder:        &framework.ConfigHolder{ConfigFilePath: "filePath"},
		Output:              outputSink,
		Workspace:           &workspaces.Workspace{Name: "defaultWorkspace"},
		KubeContext:         "kind-kind",
		RadiusInstalled:     true,
		EnvName:             "default",
		ExistingEnvironment: true,
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)
}

func Test_Run_InstalledRadiusExistingEnvironment_CreateApplication(t *testing.T) {
	ctrl := gomock.NewController(t)
	configFileInterface := framework.NewMockConfigFileInterface(ctrl)
	configFileInterface.EXPECT().
		ConfigFromContext(context.Background()).
		Return(nil).Times(1)

	configFileInterface.EXPECT().
		EditWorkspaces(context.Background(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).Times(1)

	appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
	appManagementClient.EXPECT().
		CreateApplicationIfNotFound(context.Background(), "cool-application", gomock.Any()).
		Return(nil).Times(1)

	outputSink := &output.MockOutput{}

	runner := &Runner{
		ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
		ConfigFileInterface:     configFileInterface,
		ConfigHolder:            &framework.ConfigHolder{ConfigFilePath: "filePath"},
		Output:                  outputSink,
		Workspace:               &workspaces.Workspace{Name: "defaultWorkspace"},
		KubeContext:             "kind-kind",
		RadiusInstalled:         true,
		EnvName:                 "default",
		ExistingEnvironment:     true,
		ScaffoldApplication:     true,
		ScaffoldApplicationName: "cool-application",
	}

	// Sandbox the command in a temp directory so we can do file-creation
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(wd) // Restore when test is done
	})

	directory := t.TempDir()
	err = os.Chdir(directory)
	require.NoError(t, err)

	err = runner.Run(context.Background())
	require.NoError(t, err)

	// For init, just test that these files were created. The setup code that creates them tests
	// them in detail.
	require.FileExists(t, filepath.Join(directory, "app.bicep"))
	require.FileExists(t, filepath.Join(directory, ".rad", "rad.yaml"))
}

func initParseCloudProvider(setup *setup.MockInterface, prompter *prompt.MockInterface) {
	setup.EXPECT().ParseAzureProviderArgs(gomock.Any(), true, prompter).Return(&azure.Provider{
		SubscriptionID: "test-subscription",
		ResourceGroup:  "test-rg",
		ServicePrincipal: &azure.ServicePrincipal{
			ClientID:     gomock.Any().String(),
			ClientSecret: gomock.Any().String(),
			TenantID:     gomock.Any().String(),
		},
	}, nil)
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
		GetListInput(gomock.Any(), selectKubeContextPrompt).
		Return("kind-kind", nil).Times(1)
}

func initKubeContextSelectionError(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), selectKubeContextPrompt).
		Return("", errors.New("cannot read selection")).Times(1)
}

func initKubeContextWithInterruptSignal(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), selectKubeContextPrompt).
		Return("", &prompt.ErrExitConsole{}).Times(1)
}

func initRadiusReinstallNo(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), confirmReinstallRadiusPrompt).
		Return("No", nil).Times(1)
}

func initRadiusReinstallYes(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), confirmReinstallRadiusPrompt).
		Return("Yes", nil).Times(1)
}

func initEnvNamePrompt(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetTextInput(common.EnterEnvironmentNamePrompt, gomock.Any()).
		Return("default", nil).Times(1)
}

func initEnvNamePromptError(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetTextInput(common.EnterEnvironmentNamePrompt, gomock.Any()).
		Return("", errors.New("unable to read prompt")).Times(1)
}

func initNamespacePrompt(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetTextInput(common.EnterNamespacePrompt, gomock.Any()).
		Return("default", nil).Times(1)
}

func initNamespacePromptError(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetTextInput(common.EnterNamespacePrompt, gomock.Any()).
		Return("", errors.New("Unable to read namespace")).Times(1)
}

func initAddCloudProviderPromptNo(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), confirmCloudProviderPrompt).
		Return("No", nil).Times(1)
}

func initAddCloudProviderPromptYes(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), confirmCloudProviderPrompt).
		Return("Yes", nil).Times(1)
}

func initSelectCloudProvider(prompter *prompt.MockInterface, value string) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), selectCloudProviderPrompt).
		Return(value, nil).Times(1)
}

func initHelmMockRadiusInstalled(helmMock *helm.MockInterface) {
	helmMock.EXPECT().
		CheckRadiusInstall(gomock.Any()).
		Return(true, nil).Times(1)
}

func initHelmMockRadiusNotInstalled(helmMock *helm.MockInterface) {
	helmMock.EXPECT().
		CheckRadiusInstall(gomock.Any()).
		Return(false, nil).Times(1)
}

func setExistingEnvironments(clientMock *clients.MockApplicationsManagementClient, environments []corerp.EnvironmentResource) {
	clientMock.EXPECT().
		ListEnvironmentsAll(gomock.Any()).
		Return(environments, nil).Times(1)
}

func initExistingEnvironmentSelection(prompter *prompt.MockInterface, choice string) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), common.SelectExistingEnvironmentPrompt).
		Return(choice, nil).Times(1)
}

func setScaffoldApplicationPromptNo(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), confirmSetupApplicationPrompt).
		Return("No", nil).Times(1)
}

func setScaffoldApplicationPromptYes(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetListInput(gomock.Any(), confirmSetupApplicationPrompt).
		Return("Yes", nil).Times(1)
}

func setApplicationNamePrompt(prompter *prompt.MockInterface, applicationName string) {
	prompter.EXPECT().
		GetTextInput(enterApplicationName, gomock.Any()).
		Return(applicationName, nil).Times(1)
}
