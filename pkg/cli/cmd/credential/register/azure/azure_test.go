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
	cli_credential "github.com/project-radius/radius/pkg/cli/credential"
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
			Name: "Azure command with fallback workspace",
			Input: []string{
				"--client-id", "abcd",
				"--client-secret", "efgh",
				"--tenant-id", "ijkl",
				"--subscription", "E3955194-FC78-40A8-8143-C5D8DCDC45C5",
				"--resource-group", "cool-group",
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: radcli.LoadEmptyConfig(t)},
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
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
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
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
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
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
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
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
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
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
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
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
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
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
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

			yamlData, err := yaml.Marshal(map[string]any{
				"workspaces": cli.WorkspaceSection{
					Default: "a",
					Items: map[string]workspaces.Workspace{
						"a": {
							Connection: map[string]any{
								"kind":    workspaces.KindKubernetes,
								"context": "my-context",
							},
							Source: workspaces.SourceUserConfig,

							// Will have provider info added
						},
						"b": {
							Connection: map[string]any{
								"kind":    workspaces.KindKubernetes,
								"context": "my-context",
							},
							Source: workspaces.SourceUserConfig,

							// Will be over-written
							ProviderConfig: workspaces.ProviderConfig{
								Azure: &workspaces.AzureProvider{
									SubscriptionID: "FE3955194-FC78-40A8-8143-C5D8DCDC45C5",
									ResourceGroup:  "another-cool-group",
								},
							},
						},
						"c": {
							Connection: map[string]any{
								"kind":    workspaces.KindKubernetes,
								"context": "my-other-context",
							},
							Source: workspaces.SourceUserConfig,

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

			expectedPut := cli_credential.ProviderCredentialConfiguration{
				ProviderCredentialResource: cli_credential.ProviderCredentialResource{
					Name:    "azure",
					Enabled: true,
				},
				AzureCredentials: &cli_credential.ServicePrincipalCredentials{
					ClientID:     "cool-client-id",
					ClientSecret: "cool-client-secret",
					TenantID:     "cool-tenant-id",
				},
			}

			client := clients.NewMockProviderCredentialManagementClient(ctrl)
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
					Connection: map[string]any{
						"kind":    workspaces.KindKubernetes,
						"context": "my-context",
					},
					Source: workspaces.SourceUserConfig,
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

			expected := []any{
				output.LogOutput{
					Format: "Configuring credential for cloud provider %q for Radius installation %q...",
					Params: []any{"azure", "Kubernetes (context=my-context)"},
				},
			}
			require.Equal(t, expected, outputSink.Writes)

			expectedConfig := cli.WorkspaceSection{
				Default: "a",
				Items: map[string]workspaces.Workspace{
					"a": {
						Name: "a",
						Connection: map[string]any{
							"kind":    workspaces.KindKubernetes,
							"context": "my-context",
						},
						Source: workspaces.SourceUserConfig,
						ProviderConfig: workspaces.ProviderConfig{
							Azure: &workspaces.AzureProvider{
								SubscriptionID: "E3955194-FC78-40A8-8143-C5D8DCDC45C5",
								ResourceGroup:  "cool-resource-group",
							},
						},
					},
					"b": {
						Name: "b",
						Connection: map[string]any{
							"kind":    workspaces.KindKubernetes,
							"context": "my-context",
						},
						Source: workspaces.SourceUserConfig,
						ProviderConfig: workspaces.ProviderConfig{
							Azure: &workspaces.AzureProvider{
								SubscriptionID: "E3955194-FC78-40A8-8143-C5D8DCDC45C5",
								ResourceGroup:  "cool-resource-group",
							},
						},
					},
					"c": {
						Name: "c",
						Connection: map[string]any{
							"kind":    workspaces.KindKubernetes,
							"context": "my-other-context",
						},
						Source: workspaces.SourceUserConfig,
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
