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
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"

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
			Name:          "Show Command with incorrect args",
			Input:         []string{""},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with correct options",
			Input:         []string{"groupname"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with fallback workspace",
			Input:         []string{"groupname"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {

	t.Run("Validate rad group show", func(t *testing.T) {
		id := "/planes/radius/local/resourceGroups/testrg"
		name := "testrg"

		testResourceGroup := v20220901privatepreview.ResourceGroupResource{
			ID:   &id,
			Name: &name,
		}

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().ShowUCPGroup(gomock.Any(), gomock.Any(), gomock.Any(), "testrg").Return(testResourceGroup, nil)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},

			Name: "kind-kind",
		}
		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory:    &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Workspace:            workspace,
			UCPResourceGroupName: "testrg",
			Format:               "table",
			Output:               outputSink,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		resourceGroup := v20220901privatepreview.ResourceGroupResource{
			ID:   &id,
			Name: &runner.UCPResourceGroupName,
		}
		expected := []any{
			output.FormattedOutput{
				Format:  "table",
				Obj:     resourceGroup,
				Options: objectformats.GetResourceGroupTableFormat(),
			},
		}
		require.Equal(t, expected, outputSink.Writes)

	})

}
