// // ------------------------------------------------------------
// // Copyright (c) Microsoft Corporation.
// // Licensed under the MIT License.
// // ------------------------------------------------------------

package create

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/cmd/env/namespace"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	configWithoutWorkspace := radcli.LoadConfigWithoutWorkspace(t)

	ctrl := gomock.NewController(t)
	appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
	namespaceClient := namespace.NewMockInterface(ctrl)
	testResourceGroup := v20220901privatepreview.ResourceGroupResource{}

	// Valid create command
	createMocksWithValidCommand(namespaceClient, appManagementClient, testResourceGroup)
	// Invalid resource group
	createShowUCPError(appManagementClient, testResourceGroup)
	// Invalid create command with invalid namespace
	createMocksWithInvalidResourceGroup(namespaceClient, appManagementClient, testResourceGroup)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid create command",
			Input:         []string{"testingenv"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConnectionFactory:  &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			NamespaceInterface: namespaceClient,
		},
		{
			Name:          "Create command with invalid resource group",
			Input:         []string{"testingenv", "-g", "invalidresourcegroup"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConnectionFactory:  &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			NamespaceInterface: namespaceClient,
		},
		{
			Name:          "Create command with invalid namespace",
			Input:         []string{"testingenv", "-n", "invalidnamespace"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConnectionFactory:  &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			NamespaceInterface: namespaceClient,
		},
		{
			Name:          "Create command without workspace",
			Input:         []string{"testingenv"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithoutWorkspace,
			},
			ConnectionFactory:  &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			NamespaceInterface: namespaceClient,
		},
		{
			Name:          "Create command with invalid environment",
			Input:         []string{"testingenv", "-e", "testingenv"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConnectionFactory:  &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			NamespaceInterface: namespaceClient,
		},
		{
			Name:          "Create command with invalid workspace",
			Input:         []string{"testingenv", "-w", "invalidworkspace"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConnectionFactory:  &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			NamespaceInterface: namespaceClient,
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Run env create tests", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

			namespaceClient := namespace.NewMockInterface(ctrl)
			appManagementClient.EXPECT().
				CreateEnvironment(context.Background(), "default", "global", "default", "Kubernetes", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(true, nil).Times(1)

			configFileInterface := framework.NewMockConfigFileInterface(ctrl)
			outputSink := &output.MockOutput{}
			azureProvider := &workspaces.AzureProvider{
				SubscriptionID: "test-subscription",
				ResourceGroup:  "test-rg"}
			awsProvider := &workspaces.AWSProvider{}
			providerConfig := workspaces.ProviderConfig{Azure: azureProvider, AWS: awsProvider}
			workspace := &workspaces.Workspace{
				Connection: map[string]interface{}{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name:           "defaultWorkspace",
				ProviderConfig: providerConfig,
			}

			runner := &Runner{
				ConnectionFactory:   &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				ConfigHolder:        &framework.ConfigHolder{ConfigFilePath: "filePath"},
				Output:              outputSink,
				Workspace:           workspace,
				EnvironmentName:     "default",
				UCPResourceGroup:    "default",
				Namespace:           "default",
				NamespaceInterface:  namespaceClient,
				ConfigFileInterface: configFileInterface,
				AppManagementClient: appManagementClient,
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)
		})
	})
}

func Test_RunWithoutAzureProvider(t *testing.T) {
	t.Run("Run env create tests", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

			namespaceClient := namespace.NewMockInterface(ctrl)
			appManagementClient.EXPECT().
				CreateEnvironment(context.Background(), "default", "global", "default", "Kubernetes", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(true, nil).Times(1)

			configFileInterface := framework.NewMockConfigFileInterface(ctrl)
			outputSink := &output.MockOutput{}

			awsProvider := &workspaces.AWSProvider{}
			providerConfig := workspaces.ProviderConfig{AWS: awsProvider}
			workspace := &workspaces.Workspace{
				Connection: map[string]interface{}{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name:           "defaultWorkspace",
				ProviderConfig: providerConfig,
			}

			runner := &Runner{
				ConnectionFactory:   &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				ConfigHolder:        &framework.ConfigHolder{ConfigFilePath: "filePath"},
				Output:              outputSink,
				Workspace:           workspace,
				EnvironmentName:     "default",
				UCPResourceGroup:    "default",
				Namespace:           "default",
				NamespaceInterface:  namespaceClient,
				ConfigFileInterface: configFileInterface,
				AppManagementClient: appManagementClient,
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)
		})
	})
}

func Test_Run_WithoutProvider(t *testing.T) {
	t.Run("Run env create tests", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

			namespaceClient := namespace.NewMockInterface(ctrl)
			appManagementClient.EXPECT().
				CreateEnvironment(context.Background(), "default", "global", "default", "Kubernetes", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(true, nil).Times(1)

			configFileInterface := framework.NewMockConfigFileInterface(ctrl)
			outputSink := &output.MockOutput{}
			workspace := &workspaces.Workspace{
				Connection: map[string]interface{}{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name: "defaultWorkspace",
			}

			runner := &Runner{
				ConnectionFactory:   &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				ConfigHolder:        &framework.ConfigHolder{ConfigFilePath: "filePath"},
				Output:              outputSink,
				Workspace:           workspace,
				EnvironmentName:     "default",
				UCPResourceGroup:    "default",
				Namespace:           "default",
				NamespaceInterface:  namespaceClient,
				ConfigFileInterface: configFileInterface,
				AppManagementClient: appManagementClient,
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)
		})
	})
}

func createMocksWithValidCommand(namespaceClient *namespace.MockInterface, appManagementClient *clients.MockApplicationsManagementClient, testResourceGroup v20220901privatepreview.ResourceGroupResource) {
	createShowUCPSuccess(appManagementClient, testResourceGroup)
	createValidateNamespaceSuccess(namespaceClient)
}

func createMocksWithInvalidResourceGroup(namespaceClient *namespace.MockInterface, appManagementClient *clients.MockApplicationsManagementClient, testResourceGroup v20220901privatepreview.ResourceGroupResource) {
	createShowUCPSuccess(appManagementClient, testResourceGroup)
	createValidateNamespaceError(namespaceClient)
}

func createValidateNamespaceSuccess(namespaceClient *namespace.MockInterface) {
	namespaceClient.EXPECT().
		ValidateNamespace(gomock.Any(), "testingenv").
		Return(nil).Times(1)
}

func createValidateNamespaceError(namespaceClient *namespace.MockInterface) {
	namespaceClient.EXPECT().
		ValidateNamespace(gomock.Any(), gomock.Any()).
		Return(fmt.Errorf("failed to create namespace")).Times(1)
}

func createShowUCPSuccess(appManagementClient *clients.MockApplicationsManagementClient, testResourceGroup v20220901privatepreview.ResourceGroupResource) {
	appManagementClient.EXPECT().
		ShowUCPGroup(gomock.Any(), gomock.Any(), gomock.Any(), "test-resource-group").
		Return(testResourceGroup, nil).Times(1)
}

func createShowUCPError(appManagementClient *clients.MockApplicationsManagementClient, testResourceGroup v20220901privatepreview.ResourceGroupResource) {
	appManagementClient.EXPECT().
		ShowUCPGroup(gomock.Any(), gomock.Any(), gomock.Any(), "invalidresourcegroup").
		Return(testResourceGroup, &cli.FriendlyError{Message: fmt.Sprintf("Resource group %q could not be found.", "invalidresourcegroup")}).Times(1)

}
