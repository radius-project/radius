// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/connections"
	cli_credential "github.com/project-radius/radius/pkg/cli/credential"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid Show Command",
			Input:         []string{"azure"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with fallback workspace",
			Input:         []string{"Azure"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Show Command with unsupported provider type",
			Input:         []string{"invalidProviderType"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with insufficient args",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with too many args",
			Input:         []string{"azure", "a", "b"},
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
	connection := map[string]any{
		"kind":    workspaces.KindKubernetes,
		"context": "my-context",
	}

	t.Run("Show azure provider", func(t *testing.T) {
		t.Run("Exists", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			provider := cli_credential.ProviderCredentialConfiguration{
				CloudProviderStatus: cli_credential.CloudProviderStatus{
					Name:    "azure",
					Enabled: true,
				},
			}

			client := cli_credential.NewMockCredentialManagementClient(ctrl)
			client.EXPECT().
				Get(gomock.Any(), "azure").
				Return(provider, nil).
				Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{CloudProviderManagementClient: client},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{Connection: connection},
				Kind:              "azure",
				Format:            "table",
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "Showing credential for cloud provider %q for Radius installation %q...",
					Params: []any{"azure", "Kubernetes (context=my-context)"},
				},
				output.FormattedOutput{
					Format:  "table",
					Obj:     provider,
					Options: objectformats.GetCloudProviderTableFormat(runner.Kind),
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})
		t.Run("Not Found", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			client := cli_credential.NewMockCredentialManagementClient(ctrl)
			client.EXPECT().
				Get(gomock.Any(), "azure").
				Return(cli_credential.ProviderCredentialConfiguration{}, radcli.Create404Error()).
				Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{CloudProviderManagementClient: client},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{Connection: connection},
				Kind:              "azure",
				Format:            "table",
			}

			err := runner.Run(context.Background())
			require.Error(t, err)
			require.Equal(t, "Cloud provider \"azure\" could not be found.", err.Error())
		})
	})
}
