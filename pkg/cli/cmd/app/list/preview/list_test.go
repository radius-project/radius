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
	"net/http"
	"testing"

	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
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
		serverFactory  func() fake.ApplicationsServer
		expectedOutput []any
		expectError    bool
	}{
		{
			name:          "applications returned",
			serverFactory: test_client_factory.WithApplicationsServerNoError,
			expectedOutput: []any{
				output.FormattedOutput{
					Format: "table",
					Obj: []*corerpv20250801.ApplicationResource{
						{Name: new("test-app-1")},
						{Name: new("test-app-2")},
					},
					Options: objectformats.GetResourceTableFormat(),
				},
			},
		},
		{
			name: "empty list",
			serverFactory: func() fake.ApplicationsServer {
				return fake.ApplicationsServer{
					NewListByScopePager: func(_ *corerpv20250801.ApplicationsClientListByScopeOptions) (resp azfake.PagerResponder[corerpv20250801.ApplicationsClientListByScopeResponse]) {
						resp.AddPage(http.StatusOK, corerpv20250801.ApplicationsClientListByScopeResponse{
							ApplicationResourceListResult: corerpv20250801.ApplicationResourceListResult{
								Value: []*corerpv20250801.ApplicationResource{},
							},
						}, nil)
						return
					},
				}
			},
			expectedOutput: []any{
				output.FormattedOutput{
					Format:  "table",
					Obj:     []*corerpv20250801.ApplicationResource(nil),
					Options: objectformats.GetResourceTableFormat(),
				},
			},
		},
		{
			name: "multi-page list",
			serverFactory: func() fake.ApplicationsServer {
				return fake.ApplicationsServer{
					NewListByScopePager: func(_ *corerpv20250801.ApplicationsClientListByScopeOptions) (resp azfake.PagerResponder[corerpv20250801.ApplicationsClientListByScopeResponse]) {
						resp.AddPage(http.StatusOK, corerpv20250801.ApplicationsClientListByScopeResponse{
							ApplicationResourceListResult: corerpv20250801.ApplicationResourceListResult{
								Value: []*corerpv20250801.ApplicationResource{{Name: new("page1-a")}, {Name: new("page1-b")}},
							},
						}, nil)
						resp.AddPage(http.StatusOK, corerpv20250801.ApplicationsClientListByScopeResponse{
							ApplicationResourceListResult: corerpv20250801.ApplicationResourceListResult{
								Value: []*corerpv20250801.ApplicationResource{{Name: new("page2-a")}},
							},
						}, nil)
						return
					},
				}
			},
			expectedOutput: []any{
				output.FormattedOutput{
					Format: "table",
					Obj: []*corerpv20250801.ApplicationResource{
						{Name: new("page1-a")},
						{Name: new("page1-b")},
						{Name: new("page2-a")},
					},
					Options: objectformats.GetResourceTableFormat(),
				},
			},
		},
		{
			name: "pager error surfaces",
			serverFactory: func() fake.ApplicationsServer {
				return fake.ApplicationsServer{
					NewListByScopePager: func(_ *corerpv20250801.ApplicationsClientListByScopeOptions) (resp azfake.PagerResponder[corerpv20250801.ApplicationsClientListByScopeResponse]) {
						resp.AddResponseError(http.StatusInternalServerError, "InternalServerError")
						return
					},
				}
			},
			expectError: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, tc.serverFactory)
			require.NoError(t, err)

			outputSink := &output.MockOutput{}
			runner := &Runner{
				RadiusCoreClientFactory: factory,
				Workspace:               workspace,
				Format:                  "table",
				Output:                  outputSink,
			}

			err = runner.Run(context.Background())
			if tc.expectError {
				require.Error(t, err)
				require.Empty(t, outputSink.Writes)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedOutput, outputSink.Writes)
		})
	}
}
