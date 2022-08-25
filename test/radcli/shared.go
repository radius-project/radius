// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radcli

import (
	"strings"
	"testing"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/cli/cmd/shared"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

var (
	ResourceID   = "/planes/radius/local/resourcegroups/kind-kind/providers/applications.core/containers/containera-app-with-resources"
	ResourceName = "containera-app-with-resources"
	ResourceType = "applications.core/containers"
	Location     = "global"
)

type ValidateInput struct {
	Input                   []string
	ExpectedValid           bool
	ConfigHolder            shared.ConfigHolder
	ConnectionsFactoryMock  connections.MockFactory
	OutputInterfaceMock     output.MockInterface
	AppManagementClientMock clients.MockApplicationsManagementClient
	InitMocks               func(*testing.T, *ValidateInput)
	InitScenario            func(*testing.T, *ValidateInput, *cobra.Command, framework.Runner)
}

func RunCommand(t *testing.T, cmd *cobra.Command, runner framework.Runner, testcase ValidateInput) {
	err := runner.Validate(cmd, cmd.Flags().Args())
	require.NoError(t, err)

	testcase.InitMocks(t, &testcase)
	testcase.InitScenario(t, &testcase, cmd, runner)

	err = runner.Run(cmd.Context())
	if testcase.ExpectedValid {
		require.NoError(t, err, "Command is expected to execute without errors")
	} else {
		require.Error(t, err, "Command is expected to give errorr, but returned no error")
	}
}

func SharedCommandValidation(t *testing.T, factory func(framework framework.Factory) (*cobra.Command, framework.Runner)) {
	cmd, _ := factory(&framework.Impl{})
	require.NotNil(t, cmd.Args, "Args is required")
	require.NotEmpty(t, cmd.Example, "Example is required")
	require.NotEmpty(t, cmd.Long, "Long is required")
	require.NotEmpty(t, cmd.Short, "Short is required")
	require.NotEmpty(t, cmd.Use, "Use is required")
	require.NotNil(t, cmd.RunE, "RunE is required")
}

func SharedValidateValidation(t *testing.T, factory func(framework framework.Factory) (*cobra.Command, framework.Runner), testcases []ValidateInput) {
	for _, testcase := range testcases {
		t.Run(strings.Join(testcase.Input, " "), func(t *testing.T) {
			framework := &framework.Impl{nil, &testcase.ConfigHolder, nil}
			cmd, runner := factory(framework)
			cmd.SetArgs(testcase.Input)

			err := cmd.ParseFlags(testcase.Input)
			require.NoError(t, err, "flag parsing failed")

			err = runner.Validate(cmd, cmd.Flags().Args())
			if testcase.ExpectedValid {
				require.NoError(t, err, "validation should have passed but it failed")
			} else {
				require.Error(t, err, "validation should have failed but it passed")
			}
		})
	}
}

func LoadConfigWithWorkspace() *viper.Viper {
	v, err := cli.LoadConfig("./testdata/config.yaml")
	if err != nil {
		return nil
	}
	return v
}

func CreateContainerResource() generated.GenericResource {
	resource := generated.Resource{
		ID:   &ResourceID,
		Name: &ResourceName,
		Type: &ResourceType,
	}

	trackedResource := generated.TrackedResource{
		Resource: resource,
		Location: &Location,
	}

	return generated.GenericResource{TrackedResource: trackedResource}
}
