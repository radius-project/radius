// // ------------------------------------------------------------
// // Copyright (c) Microsoft Corporation.
// // Licensed under the MIT License.
// // ------------------------------------------------------------

package create

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/framework"
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
	// configWithoutWorkspace := radcli.LoadConfigWithoutWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid env create",
			Input:         []string{"-e", "prod"},
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
	t.Run("Validate environment created with valid inputs", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			Output:           outputSink,
			Workspace:        &workspaces.Workspace{},
			EnvironmentName:  "prod",
			UCPResourceGroup: "default",
			Namespace:        "default",
			// K8sGoClient:      client_go.Interface,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

	})
	t.Run("Validate Scenario 2", func(t *testing.T) {

	})
	t.Run("Validate Scenario 3", func(t *testing.T) {

	})
	t.Run("Validate Scenario i", func(t *testing.T) {

	})
}
