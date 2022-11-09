// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package list

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
			Name:          "Valid List Command",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "List Command with fallback workspace",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "List Command with too many args",
			Input:         []string{"azure"},
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

	t.Run("List", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			providers := []clients.CloudProviderResource{
				{
					Name:    "azure",
					Enabled: true,
				},
			}

			client := clients.NewMockCloudProviderManagementClient(ctrl)
			client.EXPECT().
				List(gomock.Any()).
				Return(providers, nil).
				Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{CloudProviderManagementClient: client},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{Connection: connection},
				Format:            "table",
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)

			expected := []interface{}{
				output.LogOutput{
					Format: "Listing all cloud providers for Radius installation %q...",
					Params: []interface{}{"Kubernetes (context=my-context)"},
				},
				output.FormattedOutput{
					Format:  "table",
					Obj:     providers,
					Options: objectformats.GetCloudProviderTableFormat(),
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})
	})
}
