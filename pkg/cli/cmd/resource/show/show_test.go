// // ------------------------------------------------------------
// // Copyright (c) Microsoft Corporation.
// // Licensed under the MIT License.
// // ------------------------------------------------------------

package show

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

var (
	ResourceID   = "/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/containers/containera-app-with-resources"
	ResourceName = "containera-app-with-resources"
	ResourceType = "applications.core/containers"
	Location     = "global"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	configWithoutWorkspace := radcli.LoadConfigWithoutWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid Show Command",
			Input:         []string{"containers", "foo"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command without workspace",
			Input:         []string{"containers", "foo"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithoutWorkspace,
			},
		},
		{
			Name:          "Show Command with invalid resource type",
			Input:         []string{"invalidResourceType", "foo"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with in sufficient args",
			Input:         []string{"containers"},
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
	t.Run("Validate rad resource show valid container resource", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			ShowResource(gomock.Any(), "containers", "foo").
			Return(CreateContainerResource(), nil).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{},
			ResourceType:      "containers",
			ResourceName:      "foo",
			Format:            "table",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []interface{}{
			output.FormattedOutput{
				Format:  "table",
				Obj:     CreateContainerResource(),
				Options: objectformats.GetResourceTableFormat(),
			},
		}
		require.Equal(t, expected, outputSink.Writes)
	})
}

func CreateContainerResource() generated.GenericResource {
	return generated.GenericResource{
		ID:       &ResourceID,
		Name:     &ResourceName,
		Type:     &ResourceType,
		Location: &Location,
	}
}
