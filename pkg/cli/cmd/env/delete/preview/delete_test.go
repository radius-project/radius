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

package preview

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Delete command with default environment",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Delete command with flag",
			Input:         []string{"-e", "test-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Delete command with positional arg",
			Input:         []string{"test-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Delete command with fallback workspace",
			Input:         []string{"--environment", "test-env", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Delete command with incorrect args",
			Input:         []string{"foo", "bar"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	testcases := []struct {
		name          string
		serverFactory func() fake.EnvironmentsServer
		expectedLogs  []any
	}{
		{
			name:          "Success: environment deleted",
			serverFactory: test_client_factory.WithEnvironmentServerNoError,
			expectedLogs: []any{
				output.LogOutput{
					Format: msgDeletingEnvironmentPreview,
					Params: []any{"test-env"},
				},
				output.LogOutput{
					Format: msgEnvironmentDeletedPreview,
				},
			},
		},
	}

	for _, tc := range testcases {
		ct := tc
		t.Run(ct.name, func(t *testing.T) {
			factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, ct.serverFactory, nil)
			require.NoError(t, err)

			outputSink := &output.MockOutput{}
			runner := &Runner{
				RadiusCoreClientFactory: factory,
				Workspace:               workspace,
				Output:                  outputSink,
				EnvironmentName:         "test-env",
				Confirm:                 true,
			}

			err = runner.Run(context.Background())
			require.NoError(t, err)
			require.Equal(t, ct.expectedLogs, outputSink.Writes)
		})
	}
}
