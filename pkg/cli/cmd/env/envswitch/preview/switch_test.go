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
	"fmt"
	"testing"

	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	corerpfake "github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	type envServerFactory func() corerpfake.EnvironmentsServer

	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	emptyConfig := radcli.LoadEmptyConfig(t)

	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	validEnvServer := func() corerpfake.EnvironmentsServer {
		return corerpfake.EnvironmentsServer{
			Get: test_client_factory.WithEnvironmentServerNoError().Get,
		}
	}

	nonExistentEnvServer := func() corerpfake.EnvironmentsServer {
		return corerpfake.EnvironmentsServer{
			Get: func(
				_ context.Context,
				_ string,
				_ *v20250801preview.EnvironmentsClientGetOptions,
			) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
				errResp.SetError(fmt.Errorf("Environment not found"))
				errResp.SetResponseError(404, "Not Found")
				return
			},
		}
	}

	tests := []struct {
		name        string
		args        []string
		config      framework.ConfigHolder
		workspace   *workspaces.Workspace
		envFactory  envServerFactory
		expectError bool
	}{
		{
			name: "Switch Command with valid arguments",
			args: []string{"validEnvToSwitchTo"},
			config: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			workspace:   workspace,
			envFactory:  validEnvServer,
			expectError: false,
		},
		{
			name: "Switch Command with non-existent env",
			args: []string{"nonexistent-env"},
			config: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			workspace:   workspace,
			envFactory:  nonExistentEnvServer,
			expectError: true,
		},
		{
			name: "Switch Command with non-editable workspace invalid",
			args: []string{},
			config: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         emptyConfig,
			},
			workspace:   nil, // will be derived from config; no editable workspace
			envFactory:  nil, // not needed; Validate should fail before calling Radius.Core
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &framework.Impl{
				ConfigHolder: &tc.config,
				Output:       &output.MockOutput{},
			}

			cmd, runner := NewCommand(f)
			r := runner.(*Runner)

			// Inject workspace if provided.
			if tc.workspace != nil {
				r.Workspace = tc.workspace
			}

			// Inject Radius.Core client factory when we have an env server.
			if tc.envFactory != nil && tc.workspace != nil {
				factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
					tc.workspace.Scope,
					tc.envFactory,
					nil,
				)
				require.NoError(t, err)
				r.RadiusCoreClientFactory = factory
			}

			cmd.SetArgs(tc.args)
			cmd.SetContext(context.Background())

			err := r.Validate(cmd, tc.args)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
