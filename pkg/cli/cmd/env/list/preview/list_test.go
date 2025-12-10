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
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "List command with incorrect args",
			Input:         []string{"group"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "List command with bad workspace",
			Input:         []string{"-w", "doesnotexist"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "List command with valid workspace",
			Input:         []string{"-w", radcli.TestWorkspaceName},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "List command with fallback workspace",
			Input:         []string{"--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: radcli.LoadEmptyConfig(t),
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
		name           string
		serverFactory  func() fake.EnvironmentsServer
		expectedOutput []any
	}{
		{
			name:          "environments returned",
			serverFactory: test_client_factory.WithEnvironmentServerNoError,
			expectedOutput: []any{
				output.FormattedOutput{
					Format: "table",
					Obj: []*corerpv20250801.EnvironmentResource{
						{Name: to.Ptr("test-env-1")},
						{Name: to.Ptr("test-env-2")},
					},
					Options: objectformats.GetResourceTableFormat(),
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, tc.serverFactory, nil)
			require.NoError(t, err)

			outputSink := &output.MockOutput{}
			runner := &Runner{
				RadiusCoreClientFactory: factory,
				Workspace:               workspace,
				Format:                  "table",
				Output:                  outputSink,
			}

			err = runner.Run(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.expectedOutput, outputSink.Writes)
		})
	}
}
