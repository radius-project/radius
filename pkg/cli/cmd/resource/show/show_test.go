// // ------------------------------------------------------------
// // Copyright (c) Microsoft Corporation.
// // Licensed under the MIT License.
// // ------------------------------------------------------------

package show

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/cmd/shared"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/test/radcli"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	config := radcli.LoadConfigWithWorkspace()
	testcases := []radcli.ValidateInput{
		{
			Input:         []string{"containers", "foo", "-o", "table"},
			ExpectedValid: true,
			ConfigHolder:  shared.ConfigHolder{"", config},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	config := radcli.LoadConfigWithWorkspace()
	testcases := []radcli.ValidateInput{
		{
			Input:         []string{"containers", "foo", "-o", "table"},
			ExpectedValid: true,
			ConfigHolder:  shared.ConfigHolder{"", config},
			ConnectionsFactoryMock:  connections.MockFactory{},
			OutputInterfaceMock:     output.MockInterface{},
			AppManagementClientMock: clients.MockApplicationsManagementClient{},
			InitMocks:     InitShowMocks,
			InitScenario:  InitShowValidContainerScenario,
		},
	}
	for _, testcase := range testcases {
		framework := &framework.Impl{nil, &testcase.ConfigHolder, nil}
		cmd, runner := NewCommand(framework)
		cmd.SetArgs(testcase.Input)

		err := cmd.ParseFlags(testcase.Input)
		require.NoError(t, err, "flag parsing failed")

		radcli.RunCommand(t, cmd, runner, testcase)
	}
}

func InitShowMocks(t *testing.T, input *radcli.ValidateInput) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	input.ConnectionsFactoryMock = *connections.NewMockFactory(ctrl)
	input.AppManagementClientMock = *clients.NewMockApplicationsManagementClient(ctrl)
	input.OutputInterfaceMock = *output.NewMockInterface(ctrl)
}

// func initScenarios(connectionsFactoryMock *connections.MockFactory, outputInterfaceMock *output.MockInterface, appManagementClientMock *clients.MockApplicationsManagementClient, cmd *cobra.Command, runner Runner) {
func InitShowValidContainerScenario(t *testing.T, input *radcli.ValidateInput, cmd *cobra.Command, runner framework.Runner) {
	showRunner, ok := runner.(*Runner)
	require.EqualValues(t, ok, true)

	resourceDetails := radcli.CreateContainerResource()
	input.ConnectionsFactoryMock.EXPECT().CreateApplicationsManagementClient(cmd.Context(), showRunner.Workspace).Return(input.AppManagementClientMock, nil)
	input.AppManagementClientMock.EXPECT().ShowResource(gomock.Any(), "containers", "foo").Return(resourceDetails, nil)
	input.OutputInterfaceMock.EXPECT().Write(showRunner.Format, resourceDetails, objectformats.GetResourceTableFormat()).Return(nil)
}
