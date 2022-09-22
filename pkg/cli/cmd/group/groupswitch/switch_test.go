// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package groupswitch

import (
	"context"
	"errors"
	"fmt"
	"path"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/api/v20220315privatepreview"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Switch Command with incorrect args",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Switch command with correct arguments",
			Input:         []string{"groupname"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Switch resource group", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			configPath := path.Join(t.TempDir(), "config.yaml")

			yamlData, err := yaml.Marshal(map[string]interface{}{
				"workspaces": cli.WorkspaceSection{
					Default: "b",
					Items: map[string]workspaces.Workspace{
						"a": {
							Connection: map[string]interface{}{
								"kind":    workspaces.KindKubernetes,
								"context": "my-context",
							},
							Scope: "/planes/radius/local/resourceGroups/a",
						},
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
			require.NoError(t, err)

			config := radcli.LoadConfig(t, string(yamlData))
			config.SetConfigFile(configPath)

			expectedConfig := cli.WorkspaceSection{
				Default: "b",
				Items: map[string]workspaces.Workspace{
					"a": {
						Name: "a",
						Connection: map[string]interface{}{
							"kind":    workspaces.KindKubernetes,
							"context": "my-context",
						},
						Scope: "/planes/radius/local/resourceGroups/a",
					},
					"b": {
						Name: "b",
						Connection: map[string]interface{}{
							"kind":    workspaces.KindKubernetes,
							"context": "my-context",
						},
						Scope: "/planes/radius/local/resourceGroups/a",
					},
				},
			}

			testResourceGroup := v20220315privatepreview.ResourceGroupResource{}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().ShowUCPGroup(gomock.Any(), gomock.Any(), gomock.Any(), "a").Return(testResourceGroup, nil)

			workspace := &workspaces.Workspace{
				Name: "b",
				Connection: map[string]interface{}{
					"kind":    workspaces.KindKubernetes,
					"context": "my-context",
				},
				Scope: "/planes/radius/local/resourceGroups/b",
			}

			runner := &Runner{
				ConfigHolder: &framework.ConfigHolder{
					Config:         config,
					ConfigFilePath: configPath,
				},
				ConnectionFactory:    &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Workspace:            workspace,
				UCPResourceGroupName: "a",
			}

			err = runner.Run(context.Background())
			require.NoError(t, err)

			actualConfig, err := cli.ReadWorkspaceSection(config)
			require.NoError(t, err)
			require.Equal(t, expectedConfig, actualConfig)
		})

		t.Run("Switch (not existant)", func(t *testing.T) {
			configPath := path.Join(t.TempDir(), "config.yaml")

			yamlData, err := yaml.Marshal(map[string]interface{}{
				"workspaces": cli.WorkspaceSection{
					Default: "b",
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

			testResourceGroup := v20220315privatepreview.ResourceGroupResource{}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().ShowUCPGroup(gomock.Any(), gomock.Any(), gomock.Any(), "c").Return(testResourceGroup, errors.New("resource group doesnt exist"))

			workspace := &workspaces.Workspace{
				Name: "b",
				Connection: map[string]interface{}{
					"kind":    workspaces.KindKubernetes,
					"context": "my-context",
				},
				Scope: "/planes/radius/local/resourceGroups/b",
			}

			runner := &Runner{
				ConfigHolder: &framework.ConfigHolder{
					Config:         config,
					ConfigFilePath: configPath,
				},
				ConnectionFactory:    &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Workspace:            workspace,
				UCPResourceGroupName: "c",
			}

			expected := &cli.FriendlyError{Message: fmt.Sprintf("resource group %q does not exist. Run `rad group create` or `rad init` and try again \n", runner.UCPResourceGroupName)}
			err = runner.Run(context.Background())
			require.Equal(t, expected, err)
		})
	})
}
