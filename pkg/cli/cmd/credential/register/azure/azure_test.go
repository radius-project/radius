// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"fmt"
	"path"
	"testing"

	"github.com/golang/mock/gomock"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/credential/common"
	"github.com/project-radius/radius/pkg/cli/connections"
	cli_credential "github.com/project-radius/radius/pkg/cli/credential"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/to"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
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
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
		},
		{
			Name: "Azure command without client-id",
			Input: []string{
				"--client-secret", "efgh",
				"--tenant-id", "ijkl",
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
		},
		{
			Name: "Azure command without client-secret",
			Input: []string{
				"--client-id", "abcd",
				"--tenant-id", "ijkl",
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
		},
		{
			Name: "Azure command without tenant-id",
			Input: []string{
				"--client-id", "abcd",
				"--client-secret", "efgh",
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
		},
		{
			Name: "Azure command without subscription",
			Input: []string{
				"--client-id", "abcd",
				"--client-secret", "efgh",
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
						},
						"c": {
							Connection: map[string]any{
								"kind":    workspaces.KindKubernetes,
								"context": "my-other-context",
							},
							Source: workspaces.SourceUserConfig,
						},
					},
				},
			})
			require.NoError(t, err)

			config := radcli.LoadConfig(t, string(yamlData))
			config.SetConfigFile(configPath)

			expectedPut := ucp.CredentialResource{
				Name:     to.Ptr("default"),
				Location: to.Ptr(v1.LocationGlobal),
				Type:     to.Ptr(cli_credential.AzureCredential),
				ID:       to.Ptr(fmt.Sprintf(common.AzureCredentialID, "default")),
				Properties: &ucp.AzureServicePrincipalProperties{
					Storage: &ucp.CredentialStorageProperties{
						Kind: to.Ptr(ucp.CredentialStorageKindInternal),
					},
					ClientID:     to.Ptr("cool-client-id"),
					ClientSecret: to.Ptr("cool-client-secret"),
					TenantID:     to.Ptr("cool-tenant-id"),
				},
			}

			client := cli_credential.NewMockCredentialManagementClient(ctrl)
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
				ConnectionFactory: &connections.MockFactory{CredentialManagementClient: client},
				Output:            outputSink,
				Workspace: &workspaces.Workspace{
					Connection: map[string]any{
						"kind":    workspaces.KindKubernetes,
						"context": "my-context",
					},
					Source: workspaces.SourceUserConfig,
				},
				Format: "table",

				ClientID:     "cool-client-id",
				ClientSecret: "cool-client-secret",
				TenantID:     "cool-tenant-id",
				KubeContext:  "my-context",
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
					},
					"b": {
						Name: "b",
						Connection: map[string]any{
							"kind":    workspaces.KindKubernetes,
							"context": "my-context",
						},
						Source: workspaces.SourceUserConfig,
					},
					"c": {
						Name: "c",
						Connection: map[string]any{
							"kind":    workspaces.KindKubernetes,
							"context": "my-other-context",
						},
						Source: workspaces.SourceUserConfig,
					},
				},
			}

			actualConfig, err := cli.ReadWorkspaceSection(config)
			require.NoError(t, err)
			require.Equal(t, expectedConfig, actualConfig)
		})
	})
}
