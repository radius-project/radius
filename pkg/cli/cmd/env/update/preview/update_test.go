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

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Update Env Command without any flags",
			Input:         []string{"default"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Update Env Command without env arg",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Update Env Command with invalid Azure subscriptionId arg",
			Input:         []string{"default", "--azure-subscription-id", "subscriptionName", "--azure-resource-group", "testResourceGroup"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Update Env Command with single provider set",
			Input:         []string{"default", "--azure-subscription-id", "00000000-0000-0000-0000-000000000000", "--azure-resource-group", "testResourceGroup"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name: "Update Env Command with all providers set",
			Input: []string{"default", "--azure-subscription-id", "00000000-0000-0000-0000-000000000000", "--azure-resource-group", "testResourceGroup",
				"--aws-region", "us-west-2", "--aws-account-id", "testAWSAccount", "--kubernetes-namespace", "testNamespace",
			},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
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
		envName        string
		serverFactory  func() fake.EnvironmentsServer
		expectedOutput []any
	}{
		{
			name:          "update environment with azure provider and recipe packs",
			envName:       "test-env",
			serverFactory: test_client_factory.WithEnvironmentServerNoError,
			expectedOutput: []any{
				output.LogOutput{
					Format: "Updating Environment...",
				},
				output.FormattedOutput{
					Format: "table",
					Obj: environmentForDisplay{
						Name:        "test-env",
						RecipePacks: 3,
						Providers:   3,
					},
					Options: environmentFormat(),
				},
				output.LogOutput{
					Format: "Successfully updated environment %q.",
					Params: []any{"test-env"},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
				workspace.Scope,
				tc.serverFactory,
				nil,
			)
			require.NoError(t, err)

			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConfigHolder:            &framework.ConfigHolder{},
				Output:                  outputSink,
				Workspace:               workspace,
				EnvironmentName:         tc.envName,
				RadiusCoreClientFactory: factory,
				recipePacks:             []string{"rp1", "rp2"},
				providers: &v20250801preview.Providers{
					Azure: &v20250801preview.ProvidersAzure{
						SubscriptionID:    to.Ptr("00000000-0000-0000-0000-000000000000"),
						ResourceGroupName: to.Ptr("testResourceGroup"),
					},
					Aws: &v20250801preview.ProvidersAws{
						Scope: to.Ptr("test-aws-scope"),
					},
					Kubernetes: &v20250801preview.ProvidersKubernetes{
						Namespace: to.Ptr("test-namespace"),
					},
				},
			}

			err = runner.Run(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.expectedOutput, outputSink.Writes)
		})
	}
}
