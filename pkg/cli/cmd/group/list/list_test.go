// // ------------------------------------------------------------
// // Copyright (c) Microsoft Corporation.
// // Licensed under the MIT License.
// // ------------------------------------------------------------

package list

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
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
	configWithoutWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "List Command with incorrect args",
			Input:         []string{"group"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "List Command with correct options but bad workspace",
			Input:         []string{"-w", "doesnotexist"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithoutWorkspace,
			},
		},
		{
			Name:          "List Command with no workspace specified",
			Input:         []string{},
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

	t.Run("Validate rad group show", func(t *testing.T) {

		id := "/planes/radius/local/resourcegroups/testrg"
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
			Connection: map[string]interface{}{
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

		require.Equal(t, testResourceGroup, outputSink.Writes)

	})

}
