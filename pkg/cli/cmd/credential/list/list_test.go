/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package list

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/cli/connections"
	cli_credential "github.com/radius-project/radius/pkg/cli/credential"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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
			ExpectedValid: true,
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
	connection := map[string]any{
		"kind":    workspaces.KindKubernetes,
		"context": "my-context",
	}

	t.Run("List", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			providers := []cli_credential.CloudProviderStatus{
				{
					Name:    "azure",
					Enabled: true,
				},
			}

			client := cli_credential.NewMockCredentialManagementClient(ctrl)
			client.EXPECT().
				List(gomock.Any()).
				Return(providers, nil).
				Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{CredentialManagementClient: client},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{Connection: connection},
				Format:            "table",
			}

			err := runner.Run(t.Context())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "Listing credentials for all cloud providers for Radius installation %q...",
					Params: []any{"Kubernetes (context=my-context)"},
				},
				output.FormattedOutput{
					Format:  "table",
					Obj:     providers,
					Options: credentialFormat(),
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})
	})
}

func Test_Command_JSON(t *testing.T) {
	t.Parallel()

	listErr := errors.New("failed to list credentials")
	testcases := []struct {
		name      string
		providers []cli_credential.CloudProviderStatus
		err       error
	}{
		{
			name:      "empty list",
			providers: []cli_credential.CloudProviderStatus{},
		},
		{
			name: "populated list",
			providers: []cli_credential.CloudProviderStatus{
				{Name: "azure", Enabled: true},
				{Name: "aws", Enabled: true},
			},
		},
		{
			name: "client error",
			err:  listErr,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			client := cli_credential.NewMockCredentialManagementClient(ctrl)
			client.EXPECT().List(gomock.Any()).Return(tt.providers, tt.err)

			var stdout, stderr bytes.Buffer
			cmd, _ := NewCommand(&framework.Impl{
				ConfigHolder:      &framework.ConfigHolder{Config: radcli.LoadConfigWithWorkspace(t)},
				ConnectionFactory: &connections.MockFactory{CredentialManagementClient: client},
				Output:            &output.OutputWriter{Writer: &stdout},
			})
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SilenceUsage = true
			cmd.SetArgs([]string{"--output", "json"})

			err := cmd.ExecuteContext(t.Context())
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
				require.Empty(t, stdout.String())
				return
			}
			require.NoError(t, err)

			var actual []cli_credential.CloudProviderStatus
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &actual))
			require.Equal(t, tt.providers, actual)
		})
	}
}
