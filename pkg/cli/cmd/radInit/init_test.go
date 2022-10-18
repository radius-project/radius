// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radInit

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd/api"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	ctrl := gomock.NewController(t)
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	// Scenario with no cloud provider
	kubernetesMock := kubernetes.NewMockInterface(ctrl)
	prompter := prompt.NewMockInterface(ctrl)
	helmMock := helm.NewMockInterface(ctrl)

	initMocksWithoutCloudProvider(kubernetesMock, prompter, helmMock)
	// Scenario with error kubeContext read
	initMocksWithKubeContextReadError(kubernetesMock)
	// Scenario with error kubeContext selection
	initMocksWithKubeContextSelectionError(kubernetesMock, prompter)
	// Scenario with error env name read
	initMocksWithErrorEnvNameRead(kubernetesMock, prompter, helmMock)
	// Scenario with error name space read
	initMocksWithErrorNamespaceRead(kubernetesMock, prompter, helmMock)
	// Scenario with cloud provider configured
	initMocksWithCloudProvider(kubernetesMock, prompter, helmMock)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid Init Command",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			KubernetesInterface: kubernetesMock,
			Prompter:            prompter,
			HelmInterface:       helmMock,
		},
		{
			Name:          "Init Command With Error KubeContext Read",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			KubernetesInterface: kubernetesMock,
		},
		{
			Name:          "Init Command With Error KubeContext Selection",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			KubernetesInterface: kubernetesMock,
			Prompter:            prompter,
		},
		{
			Name:          "Init Command With Error EnvName Read",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			KubernetesInterface: kubernetesMock,
			Prompter:            prompter,
			HelmInterface:       helmMock,
		},
		{
			Name:          "Init Command With Error Namespace Read",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			KubernetesInterface: kubernetesMock,
			Prompter:            prompter,
			HelmInterface:       helmMock,
		},
		{
			Name:          "Init Command With Cloud Provider Read",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			KubernetesInterface: kubernetesMock,
			Prompter:            prompter,
			HelmInterface:       helmMock,
			Context:             context.Background(),
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
			CreateEnvironment(context.Background(), "default", "global", "defaultNameSpace", "kubernetes", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
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
			NameSpace:           "defaultNameSpace",
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
			CreateEnvironment(context.Background(), "default", "global", "defaultNameSpace", "kubernetes", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
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
			NameSpace:           "defaultNameSpace",
			Reinstall:           true,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
	})
}

func initMocksWithoutCloudProvider(kubernetesMock *kubernetes.MockInterface, prompterMock *prompt.MockInterface, helmMock *helm.MockInterface) {
	initGetKubeContextSuccess(kubernetesMock)
	initKubeContextWithKind(prompterMock)
	initHelmMockRadiusInstalled(helmMock)
	initRadiusReInstallNo(prompterMock)
	initEnvNamePrompt(prompterMock)
	initNameSpacePrompt(prompterMock)
	initAddCloudProviderPromptNo(prompterMock)
}

func initMocksWithCloudProvider(kubernetesMock *kubernetes.MockInterface, prompterMock *prompt.MockInterface, helmMock *helm.MockInterface) {
	initGetKubeContextSuccess(kubernetesMock)
	initKubeContextWithKind(prompterMock)
	initHelmMockRadiusInstalled(helmMock)
	initRadiusReInstallYes(prompterMock)
	initEnvNamePrompt(prompterMock)
	initNameSpacePrompt(prompterMock)
	initAddCloudProviderPromptYes(prompterMock)
	initSelectCloudProvider(prompterMock)
	initAddCloudProviderPromptYes(prompterMock) // Y add azure provider
	initAddCloudProviderPromptYes(prompterMock) // Y use default subscription
	initAddCloudProviderPromptYes(prompterMock) // Y create RG
	initSelectRGName(prompterMock)              // Use radius-rg as name
	initEnvNamePrompt(prompterMock)             // Mock appID for cloud provider
	initEnvNamePrompt(prompterMock)             // Mock password for cloud provider
	initEnvNamePrompt(prompterMock)             // Mock tenant for cloud provider
	initAddCloudProviderPromptNo(prompterMock)  // N dont add another cloud provider
}

func initMocksWithKubeContextReadError(kubernetesMock *kubernetes.MockInterface) {
	initGetKubeContextError(kubernetesMock)
}

func initMocksWithKubeContextSelectionError(kubernetesMock *kubernetes.MockInterface, prompterMock *prompt.MockInterface) {
	initGetKubeContextSuccess(kubernetesMock)
	initKubeContextSelectionError(prompterMock)
}

func initMocksWithErrorEnvNameRead(kubernetesMock *kubernetes.MockInterface, prompterMock *prompt.MockInterface, helmMock *helm.MockInterface) {
	initGetKubeContextSuccess(kubernetesMock)
	initKubeContextWithKind(prompterMock)
	initHelmMockRadiusInstalled(helmMock)
	initRadiusReInstallNo(prompterMock)
	initEnvNamePromptError(prompterMock)
}

func initMocksWithErrorNamespaceRead(kubernetesMock *kubernetes.MockInterface, prompterMock *prompt.MockInterface, helmMock *helm.MockInterface) {
	initGetKubeContextSuccess(kubernetesMock)
	initKubeContextWithKind(prompterMock)
	initHelmMockRadiusInstalled(helmMock)
	initRadiusReInstallNo(prompterMock)
	initEnvNamePrompt(prompterMock)
	initNameSpacePromptError(prompterMock)
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
		RunSelect(gomock.Any()).
		Return(2, "", nil).Times(1)
}

func initKubeContextSelectionError(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunSelect(gomock.Any()).
		Return(-1, "", errors.New("cannot read selection")).Times(1)
}

func initRadiusReInstallNo(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(gomock.Any()).
		Return("N", nil).Times(1)
}

func initRadiusReInstallYes(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(gomock.Any()).
		Return("Y", nil).Times(1)
}

func initEnvNamePrompt(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(gomock.Any()).
		Return("default", nil).Times(1)
}

func initEnvNamePromptError(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(gomock.Any()).
		Return("", errors.New("unable to read prompt")).Times(1)
}

func initNameSpacePrompt(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(gomock.Any()).
		Return("default", nil).Times(1)
}

func initNameSpacePromptError(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(gomock.Any()).
		Return("", errors.New("Unable to read namespace")).Times(1)
}

func initAddCloudProviderPromptNo(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(gomock.Any()).
		Return("N", nil).Times(1)
}

func initAddCloudProviderPromptYes(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(gomock.Any()).
		Return("y", nil).Times(1)
}

func initSelectRGName(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunPrompt(gomock.Any()).
		Return("radius-rg", nil).Times(1)
}

func initSelectCloudProvider(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		RunSelect(gomock.Any()).Return(0, "", nil).Times(1)
}

func initHelmMockRadiusInstalled(helmMock *helm.MockInterface) {
	helmMock.EXPECT().
		CheckRadiusInstall(gomock.Any()).
		Return(true, nil).Times(1)
}
