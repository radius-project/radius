// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radInit

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/golang/mock/gomock"
	"github.com/manifoldco/promptui"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/cmd/provider/common"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd/api"
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
						Name: to.StringPtr("cool-existing-env"),
					},
				})
				initExistingEnvironmentSelection(mocks.Prompter, common.SelectExistingEnvironmentCreateSentinel)

				// Use default env name and namespace
				initEnvNamePrompt(mocks.Prompter)
				initNamespacePrompt(mocks.Prompter)

				// No cloud providers
				initAddCloudProviderPromptNo(mocks.Prompter)
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
						Name: to.StringPtr("cool-existing-env"),
					},
				})
				initExistingEnvironmentSelection(mocks.Prompter, "cool-existing-env")

				// No need to choose env settings since we're using existing
			},
		},
		{
			Name:          "Init Command With Cloud Provider (Reinstall))",
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
				initSelectCloudProvider(mocks.Prompter)
				initParseCloudProvider(mocks.Setup, mocks.Prompter)

				// Don't add any other cloud providers
				initAddCloudProviderPromptNo(mocks.Prompter)
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

				// No prompts in this case
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

				// No prompts in this case
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
						Name: to.StringPtr("default"),
					},
				})

				// No prompts in this case
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
						Name: to.StringPtr("dev"),
					},
					{
						Name: to.StringPtr("prod"),
					},
				})

				// prompt the user since there's no 'default'
				initExistingEnvironmentSelection(mocks.Prompter, "prod")
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
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Init Radius", func(t *testing.T) {
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
		appManagementClient.EXPECT().
			CreateEnvironment(context.Background(), "default", v1.LocationGlobal, "defaultNamespace", "kubernetes", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
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
			EnvName:             "default",
			Namespace:           "defaultNamespace",
			Reinstall:           true,
			AzureCloudProvider: &azure.Provider{
				SubscriptionID: "test-subscription",
				ResourceGroup:  "test-rg",
			},
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
	})
}

func Test_Run_WithoutAzureProvider(t *testing.T) {
	t.Run("Init Radius", func(t *testing.T) {
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
		appManagementClient.EXPECT().
			CreateEnvironment(context.Background(), "default", v1.LocationGlobal, "defaultNamespace", "kubernetes", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
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
			EnvName:             "default",
			Namespace:           "defaultNamespace",
			Reinstall:           true,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
	})
}

func initParseCloudProvider(setup *setup.MockInterface, promper *prompt.MockInterface) {
	setup.EXPECT().ParseAzureProviderArgs(gomock.Any(), true, promper).Return(&azure.Provider{
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
		RunSelect(matchesSelect(selectKubeContextPrompt)).
		Return(2, "", nil).Times(1)
}

func initKubeContextSelectionError(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunSelect(matchesSelect(selectKubeContextPrompt)).
		Return(-1, "", errors.New("cannot read selection")).Times(1)
}

func initRadiusReinstallNo(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(matchesPrompt(confirmReinstallRadiusPrompt)).
		Return("N", nil).Times(1)
}

func initRadiusReinstallYes(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(matchesPrompt(confirmReinstallRadiusPrompt)).
		Return("Y", nil).Times(1)
}

func initEnvNamePrompt(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(matchesPrompt(common.EnterEnvironmentNamePrompt)).
		Return("default", nil).Times(1)
}

func initEnvNamePromptError(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(matchesPrompt(common.EnterEnvironmentNamePrompt)).
		Return("", errors.New("unable to read prompt")).Times(1)
}

func initNamespacePrompt(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(matchesPrompt(common.EnterNamespacePrompt)).
		Return("default", nil).Times(1)
}

func initNamespacePromptError(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(matchesPrompt(common.EnterNamespacePrompt)).
		Return("", errors.New("Unable to read namespace")).Times(1)
}

func initAddCloudProviderPromptNo(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(matchesPrompt(confirmCloudProviderPrompt)).
		Return("N", nil).Times(1)
}

func initAddCloudProviderPromptYes(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(matchesPrompt(confirmCloudProviderPrompt)).
		Return("Y", nil).Times(1)

}

func initSelectCloudProvider(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunSelect(matchesSelect(selectCloudProviderPrompt)).Return(0, "", nil).Times(1)
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
		RunSelect(matchesSelect(common.SelectExistingEnvironmentPrompt)).
		Return(-1, choice, nil).Times(1) // We ignore the index, so this is ok.
}

var _ gomock.Matcher = (*PromptMatcher)(nil)

func matchesPrompt(message string) *PromptMatcher {
	return &PromptMatcher{Message: message}
}

type PromptMatcher struct {
	Message string
}

func (m *PromptMatcher) Matches(x interface{}) bool {
	p, ok := x.(promptui.Prompt)
	if !ok {
		return false
	}

	return p.Label == m.Message
}

func (m *PromptMatcher) String() string {
	return fmt.Sprintf("promptui.Prompt { Label: \"%s\"}", m.Message)
}

var _ gomock.Matcher = (*SelectMatcher)(nil)

func matchesSelect(message string) *SelectMatcher {
	return &SelectMatcher{Message: message}
}

type SelectMatcher struct {
	Message string
}

func (m *SelectMatcher) Matches(x interface{}) bool {
	p, ok := x.(promptui.Select)
	if !ok {
		return false
	}

	return p.Label == m.Message
}

func (m *SelectMatcher) String() string {
	return fmt.Sprintf("promptui.Select { Label: \"%s\"}", m.Message)
}
