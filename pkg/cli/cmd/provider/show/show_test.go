// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
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
			Name:          "Show Command without workspace",
			Input:         []string{"Azure"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadConfigWithoutWorkspace(t),
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
	connection := map[string]interface{}{
		"kind":    workspaces.KindKubernetes,
		"context": "my-context",
	}

	t.Run("Show azure provider", func(t *testing.T) {
		t.Run("Exists", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			provider := clients.CloudProviderResource{
				Name:    "azure",
				Enabled: true,
			}

			client := clients.NewMockCloudProviderManagementClient(ctrl)
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

			expected := []interface{}{
				output.LogOutput{
					Format: "Showing cloud provider %q for Radius installation %q...",
					Params: []interface{}{"azure", "Kubernetes (context=my-context)"},
				},
				output.FormattedOutput{
					Format:  "table",
					Obj:     provider,
					Options: objectformats.GetCloudProviderTableFormat(),
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})
		t.Run("Not Found", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			client := clients.NewMockCloudProviderManagementClient(ctrl)
			client.EXPECT().
				Get(gomock.Any(), "azure").
				Return(clients.CloudProviderResource{}, radcli.Create404Error()).
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
