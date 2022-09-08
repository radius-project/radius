// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"path"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	configWithoutWorkspace := radcli.LoadConfigWithoutWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name: "Valid Azure command",
			Input: []string{
				"--client-id", "abcd",
				"--client-secret", "efgh",
				"--tenant-id", "ijkl",
				"--subscription", "E3955194-FC78-40A8-8143-C5D8DCDC45C5",
				"--resource-group", "cool-group",
			},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
		},
		{
			Name: "Azure command without workspace",
			Input: []string{
				"--client-id", "abcd",
				"--client-secret", "efgh",
				"--tenant-id", "ijkl",
				"--subscription", "E3955194-FC78-40A8-8143-C5D8DCDC45C5",
				"--resource-group", "cool-group",
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: configWithoutWorkspace},
		},
		{
			Name: "Azure command with too many positional args",
			Input: []string{
				"letsgoooooo",
				"--client-id", "abcd",
				"--client-secret", "efgh",
				"--tenant-id", "ijkl",
				"--subscription", "E3955194-FC78-40A8-8143-C5D8DCDC45C5",
				"--resource-group", "cool-group",
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: configWithoutWorkspace},
		},
		{
			Name: "Azure command without client-id",
			Input: []string{
				"--client-secret", "efgh",
				"--tenant-id", "ijkl",
				"--subscription", "E3955194-FC78-40A8-8143-C5D8DCDC45C5",
				"--resource-group", "cool-group",
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: configWithoutWorkspace},
		},
		{
			Name: "Azure command without client-secret",
			Input: []string{
				"--client-id", "abcd",
				"--tenant-id", "ijkl",
				"--subscription", "E3955194-FC78-40A8-8143-C5D8DCDC45C5",
				"--resource-group", "cool-group",
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: configWithoutWorkspace},
		},
		{
			Name: "Azure command without tenant-id",
			Input: []string{
				"--client-id", "abcd",
				"--client-secret", "efgh",
				"--subscription", "E3955194-FC78-40A8-8143-C5D8DCDC45C5",
				"--resource-group", "cool-group",
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: configWithoutWorkspace},
		},
		{
			Name: "Azure command without subscription",
			Input: []string{
				"--client-id", "abcd",
				"--client-secret", "efgh",
				"--tenant-id", "ijkl",
				"--resource-group", "cool-group",
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: configWithoutWorkspace},
		},
		{
			Name: "Azure command without resource group",
			Input: []string{
				"letsgoooooo",
				"--client-id", "abcd",
				"--client-secret", "efgh",
				"--tenant-id", "ijkl",
				"--subscription", "E3955194-FC78-40A8-8143-C5D8DCDC45C5",
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: configWithoutWorkspace},
		},
		{
			Name: "Azure command with invalid subscription id",
			Input: []string{
				"--client-id", "abcd",
				"--client-secret", "efgh",
				"--tenant-id", "ijkl",
				"--subscription", "E3955194-FC78-40A8-8143-C5D8DCDC45C5invalid",
				"--resource-group", "cool-group",
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: configWithoutWorkspace},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Create azure provider", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			// We need to isolate the configuration because we're going to make edits
			configPath := path.Join(t.TempDir(), "config.yaml")

			yamlData, err := yaml.Marshal(map[string]interface{}{
				"workspaces": cli.WorkspaceSection{
					Default: "a",
					Items: map[string]workspaces.Workspace{
						"a": {
							Connection: map[string]interface{}{
								"kind":    workspaces.KindKubernetes,
								"context": "my-context",
							},

							// Will have provider info added
						},
						"b": {
							Connection: map[string]interface{}{
								"kind":    workspaces.KindKubernetes,
								"context": "my-context",
							},

							// Will be over-written
							ProviderConfig: workspaces.ProviderConfig{
								Azure: &workspaces.AzureProvider{
									SubscriptionID: "FE3955194-FC78-40A8-8143-C5D8DCDC45C5",
									ResourceGroup:  "another-cool-group",
								},
							},
						},
						"c": {
							Connection: map[string]interface{}{
								"kind":    workspaces.KindKubernetes,
								"context": "my-other-context",
							},

							// Will be left-alone
							ProviderConfig: workspaces.ProviderConfig{
								Azure: &workspaces.AzureProvider{
									SubscriptionID: "FE3955194-FC78-40A8-8143-C5D8DCDC45C5",
									ResourceGroup:  "another-cool-group",
								},
							},
						},
					},
				},
			})
			require.NoError(t, err)

			config := radcli.LoadConfig(t, string(yamlData))
			config.SetConfigFile(configPath)

			expectedPut := clients.AzureCloudProviderResource{
				CloudProviderResource: clients.CloudProviderResource{
					Name:    "azure",
					Enabled: true,
				},
				Credentials: &clients.ServicePrincipalCredentials{
					ClientID:     "cool-client-id",
					ClientSecret: "cool-client-secret",
					TenantID:     "cool-tenant-id",
				},
			}

			client := clients.NewMockCloudProviderManagementClient(ctrl)
			client.EXPECT().
				Put(gomock.Any(), expectedPut).
				Return(nil).
				Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConfigHolder: &framework.ConfigHolder{
					Config:         config,
					ConfigFilePath: configPath,
				},
				ConnectionFactory: &connections.MockFactory{CloudProviderManagementClient: client},
				Output:            outputSink,
				Workspace: &workspaces.Workspace{
					Connection: map[string]interface{}{
						"kind":    workspaces.KindKubernetes,
						"context": "my-context",
					},
				},
				Format: "table",

				ClientID:       "cool-client-id",
				ClientSecret:   "cool-client-secret",
				TenantID:       "cool-tenant-id",
				SubscriptionID: "E3955194-FC78-40A8-8143-C5D8DCDC45C5",
				ResourceGroup:  "cool-resource-group",
				KubeContext:    "my-context",
			}

			err = runner.Run(context.Background())
			require.NoError(t, err)

			expected := []interface{}{
				output.LogOutput{
					Format: "Setting cloud provider %q for Radius installation %q...",
					Params: []interface{}{"azure", "Kubernetes (context=my-context)"},
				},
			}
			require.Equal(t, expected, outputSink.Writes)

			expectedConfig := cli.WorkspaceSection{
				Default: "a",
				Items: map[string]workspaces.Workspace{
					"a": {
						Name: "a",
						Connection: map[string]interface{}{
							"kind":    workspaces.KindKubernetes,
							"context": "my-context",
						},
						ProviderConfig: workspaces.ProviderConfig{
							Azure: &workspaces.AzureProvider{
								SubscriptionID: "E3955194-FC78-40A8-8143-C5D8DCDC45C5",
								ResourceGroup:  "cool-resource-group",
							},
						},
					},
					"b": {
						Name: "b",
						Connection: map[string]interface{}{
							"kind":    workspaces.KindKubernetes,
							"context": "my-context",
						},
						ProviderConfig: workspaces.ProviderConfig{
							Azure: &workspaces.AzureProvider{
								SubscriptionID: "E3955194-FC78-40A8-8143-C5D8DCDC45C5",
								ResourceGroup:  "cool-resource-group",
							},
						},
					},
					"c": {
						Name: "c",
						Connection: map[string]interface{}{
							"kind":    workspaces.KindKubernetes,
							"context": "my-other-context",
						},
						ProviderConfig: workspaces.ProviderConfig{
							Azure: &workspaces.AzureProvider{
								SubscriptionID: "FE3955194-FC78-40A8-8143-C5D8DCDC45C5",
								ResourceGroup:  "another-cool-group",
							},
						},
					},
				},
			}

			actualConfig, err := cli.ReadWorkspaceSection(config)
			require.NoError(t, err)
			require.Equal(t, expectedConfig, actualConfig)
		})
	})
}
