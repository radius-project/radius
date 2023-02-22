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
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/cmd"
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
	testResourceGroup := v20220901privatepreview.ResourceGroupResource{}

	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid create command",
			Input:         []string{"testingenv"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Valid create command
				createMocksWithValidCommand(mocks.Namespace, mocks.ApplicationManagementClient, testResourceGroup)
			},
		},
		{
			Name:          "Create command with invalid resource group",
			Input:         []string{"testingenv", "-g", "invalidresourcegroup"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Invalid resource group
				createShowUCPError(mocks.ApplicationManagementClient, testResourceGroup)
			},
		},
		{
			Name:          "Create command with invalid namespace",
			Input:         []string{"testingenv", "-n", "invalidnamespace"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				// Invalid create command with invalid namespace
				createMocksWithInvalidResourceGroup(mocks.Namespace, mocks.ApplicationManagementClient, testResourceGroup)
			},
		},
		{
			Name:          "Create command with fallback workspace",
			Input:         []string{"testingenv"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Create command with invalid environment",
			Input:         []string{"testingenv", "-e", "testingenv"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Create command with invalid workspace",
			Input:         []string{"testingenv", "-w", "invalidworkspace"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run_Success(t *testing.T) {
	runCreateTests := []struct {
		name             string
		providerConfig   workspaces.ProviderConfig
		expectedProvider []any
	}{
		{
			name: "Run env create with only Azure",
			providerConfig: workspaces.ProviderConfig{
				Azure: &workspaces.AzureProvider{
					SubscriptionID: "test-subscription",
					ResourceGroup:  "test-rg",
				},
			},
			expectedProvider: []any{
				&azure.Provider{
					SubscriptionID: "test-subscription",
					ResourceGroup:  "test-rg",
				},
			},
		},
		{
			name: "Run env create with only AWS",
			providerConfig: workspaces.ProviderConfig{
				AWS: &workspaces.AWSProvider{
					AccountId: "0",
					Region:    "westus",
				},
			},
			expectedProvider: []any{
				&aws.Provider{
					AccountId:    "0",
					TargetRegion: "westus",
				},
			},
		},
		{
			name: "Run env create with Azure and AWS",
			providerConfig: workspaces.ProviderConfig{
				Azure: &workspaces.AzureProvider{
					SubscriptionID: "test-subscription",
					ResourceGroup:  "test-rg",
				},
				AWS: &workspaces.AWSProvider{
					AccountId: "0",
					Region:    "westus",
				},
			},
			expectedProvider: []any{
				&azure.Provider{
					SubscriptionID: "test-subscription",
					ResourceGroup:  "test-rg",
				},
				&aws.Provider{
					AccountId:    "0",
					TargetRegion: "westus",
				},
			},
		},
		{
			name:             "Run env create without providers",
			expectedProvider: []any{},
		},
		{
			name: "Run env create without incomplete providers",
			providerConfig: workspaces.ProviderConfig{
				Azure: &workspaces.AzureProvider{
					SubscriptionID: "test-subscription",
					ResourceGroup:  "",
				},
				AWS: &workspaces.AWSProvider{
					AccountId: "0",
					Region:    "",
				},
			},
			expectedProvider: []any{},
		},
	}

	for _, tc := range runCreateTests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

			p, err := cmd.CreateEnvProviders(tc.expectedProvider)
			require.NoError(t, err)

			namespaceClient := namespace.NewMockInterface(ctrl)
			appManagementClient.EXPECT().
				CreateEnvironment(context.Background(), "default", v1.LocationGlobal, "default", "Kubernetes", gomock.Any(), gomock.Any(), &p, gomock.Any()).
				Return(true, nil).Times(1)

			configFileInterface := framework.NewMockConfigFileInterface(ctrl)
			outputSink := &output.MockOutput{}
			workspace := &workspaces.Workspace{
				Connection: map[string]any{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name:           "defaultWorkspace",
				ProviderConfig: tc.providerConfig,
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
				SkipDevRecipes:      true,
			}

			err = runner.Run(context.Background())
			require.NoError(t, err)
		})
	}
}

func Test_Run_SkipDevRecipes(t *testing.T) {
	t.Run("Run env create tests", func(t *testing.T) {
		t.Run("Success with set to true", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

			namespaceClient := namespace.NewMockInterface(ctrl)
			appManagementClient.EXPECT().
				CreateEnvironment(context.Background(), "default", v1.LocationGlobal, "default", "Kubernetes", gomock.Any(), gomock.Any(), gomock.Any(), false).
				Return(true, nil).Times(1)

			configFileInterface := framework.NewMockConfigFileInterface(ctrl)
			outputSink := &output.MockOutput{}
			workspace := &workspaces.Workspace{
				Connection: map[string]any{
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
				SkipDevRecipes:      true,
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)
		})

		t.Run("Success with set to false", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

			namespaceClient := namespace.NewMockInterface(ctrl)
			appManagementClient.EXPECT().
				CreateEnvironment(context.Background(), "default", v1.LocationGlobal, "default", "Kubernetes", gomock.Any(), gomock.Any(), gomock.Any(), true).
				Return(true, nil).Times(1)

			configFileInterface := framework.NewMockConfigFileInterface(ctrl)
			outputSink := &output.MockOutput{}
			workspace := &workspaces.Workspace{
				Connection: map[string]any{
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
				SkipDevRecipes:      false,
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
