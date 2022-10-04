// // ------------------------------------------------------------
// // Copyright (c) Microsoft Corporation.
// // Licensed under the MIT License.
// // ------------------------------------------------------------

package create

import (
	"context"
	"errors"
	"fmt"
	"path"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/cmd/env/namespace"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/api/v20220315privatepreview"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	configWithoutWorkspace := radcli.LoadConfigWithoutWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid create command",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Create command without workspace",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithoutWorkspace,
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
			Name:          "Create command with invalid namespace",
			Input:         []string{"-n", "invalidnamespace"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Create command with invalid workspace",
			Input:         []string{"-w", "invalidworkspace"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Create command with invalid resource group",
			Input:         []string{"-g", "invalidresourcegroup"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Run env create tests", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

			k8sGoClient :=
				fake.NewSimpleClientset(&v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "default",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				})

			namespaceClient := namespace.NewMockInterface(ctrl)
			namespaceClient.EXPECT().
				ValidateNamespace(context.Background(), gomock.Any(), "default").
				Return(nil).Times(1)

			testResourceGroup := v20220315privatepreview.ResourceGroupResource{}
			appManagementClient.EXPECT().
				ShowUCPGroup(gomock.Any(), gomock.Any(), gomock.Any(), "default").
				Return(testResourceGroup, nil).Times(1)

			appManagementClient.EXPECT().
				CreateEnvironment(context.Background(), "default", "global", "default", "Kubernetes", gomock.Any(), gomock.Any()).
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
				K8sGoClient:         k8sGoClient,
				NamespaceInterface:  namespaceClient,
				ConfigFileInterface: configFileInterface,
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)
		})

		t.Run("Failure with non-existant resource group", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			configPath := path.Join(t.TempDir(), "config.yaml")

			yamlData, err := yaml.Marshal(map[string]interface{}{
				"workspaces": cli.WorkspaceSection{
					Default: "defaultWorkspace",
					Items: map[string]workspaces.Workspace{

						"b": {
							Connection: map[string]interface{}{
								"kind":    workspaces.KindKubernetes,
								"context": "my-context",
							},
							Scope: "/planes/radius/local/resourceGroups/b",
						},
					},
				},
			})
			config := radcli.LoadConfig(t, string(yamlData))
			config.SetConfigFile(configPath)
			require.NoError(t, err)

			k8sGoClient :=
				fake.NewSimpleClientset(&v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "default",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				})

			namespaceClient := namespace.NewMockInterface(ctrl)
			namespaceClient.EXPECT().
				ValidateNamespace(context.Background(), gomock.Any(), "default").
				Return(nil).Times(1)

			testResourceGroup := v20220315privatepreview.ResourceGroupResource{}
			appManagementClient.EXPECT().
				ShowUCPGroup(gomock.Any(), gomock.Any(), gomock.Any(), "c").
				Return(testResourceGroup, &cli.FriendlyError{Message: fmt.Sprintf("Resource group %q could not be found.", "c")})

			configFileInterface := framework.NewMockConfigFileInterface(ctrl)
			outputSink := &output.MockOutput{}
			workspace := &workspaces.Workspace{
				Name: "defaultWorkspace",
				Connection: map[string]interface{}{
					"kind":    workspaces.KindKubernetes,
					"context": "my-context",
				},
				Scope: "/planes/radius/local/resourceGroups/b",
			}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				ConfigHolder: &framework.ConfigHolder{
					Config:         config,
					ConfigFilePath: configPath,
				}, Output: outputSink,
				Workspace:           workspace,
				EnvironmentName:     "default",
				UCPResourceGroup:    "c",
				Namespace:           "default",
				K8sGoClient:         k8sGoClient,
				NamespaceInterface:  namespaceClient,
				ConfigFileInterface: configFileInterface,
			}

			expected := &cli.FriendlyError{Message: fmt.Sprintf("Resource group %q could not be found.", runner.UCPResourceGroup)}
			err = runner.Run(context.Background())
			require.Equal(t, expected, err)

		})

		t.Run("Failure with invalid namespace", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

			k8sGoClient :=
				fake.NewSimpleClientset(&v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "default",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				})

			namespaceClient := namespace.NewMockInterface(ctrl)
			namespaceClient.EXPECT().
				ValidateNamespace(context.Background(), gomock.Any(), gomock.Any()).
				Return(errors.New(fmt.Sprintf("failed to create namespace %s", "notthedefault")))

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
				Namespace:           "notthedefault",
				K8sGoClient:         k8sGoClient,
				NamespaceInterface:  namespaceClient,
				ConfigFileInterface: configFileInterface,
			}

			expected := errors.New(fmt.Sprintf("failed to create namespace %s", "notthedefault"))
			err := runner.Run(context.Background())
			require.Equal(t, expected, err)
		})
	})
}
