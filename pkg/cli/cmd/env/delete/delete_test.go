// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package delete

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/workspaces"
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
			Name:          "Delete Command with default environment",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete Command with flag",
			Input:         []string{"-e", "test-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete Command with positional arg",
			Input:         []string{"test-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete Command with confirm",
			Input:         []string{"--yes"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete Command with fallback workspace",
			Input:         []string{"--environment", "test-env", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Delete Command with incorrect args",
			Input:         []string{"foo", "bar"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Show(t *testing.T) {
	t.Run("Success: Environment Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			DeleteEnv(gomock.Any(), "test-env").
			Return(true, nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name:  "kind-kind",
			Scope: "/planes/radius/local/resourceGroups/test-group",
		}
		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Workspace:         workspace,
			Format:            "table",
			Output:            outputSink,
			EnvironmentName:   "test-env",
			Confirm:           true,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.LogOutput{
				Format: "Environment deleted",
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Success: Prompt Confirmed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		promptMock := prompt.NewMockInterface(ctrl)
		promptMock.EXPECT().
			ConfirmWithDefault(fmt.Sprintf(deleteConfirmation, "test-env"), prompt.No).
			Return(true, nil).
			Times(1)

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			DeleteEnv(gomock.Any(), "test-env").
			Return(true, nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name:  "kind-kind",
			Scope: "/planes/radius/local/resourceGroups/test-group",
		}
		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Prompt:            promptMock,
			Workspace:         workspace,
			Format:            "table",
			Output:            outputSink,
			EnvironmentName:   "test-env",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.LogOutput{
				Format: "Environment deleted",
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Success: Prompt Cancelled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		promptMock := prompt.NewMockInterface(ctrl)
		promptMock.EXPECT().
			ConfirmWithDefault(fmt.Sprintf(deleteConfirmation, "test-env"), prompt.No).
			Return(false, nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name:  "kind-kind",
			Scope: "/planes/radius/local/resourceGroups/test-group",
		}
		outputSink := &output.MockOutput{}
		runner := &Runner{
			Prompt:          promptMock,
			Workspace:       workspace,
			Format:          "table",
			Output:          outputSink,
			EnvironmentName: "test-env",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
		require.Empty(t, outputSink.Writes)
	})

	// YES, this is a success case. Delete means "make it be gone", so if the envrionment is already
	// gone that counts as a success.
	//
	// We print a different message which is why it has a separate test
	t.Run("Success: Environment Not Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			DeleteEnv(gomock.Any(), "test-env").
			Return(false, nil).
			Times(1)

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},
			Name:  "kind-kind",
			Scope: "/planes/radius/local/resourceGroups/test-group",
		}
		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Workspace:         workspace,
			Format:            "table",
			Output:            outputSink,
			EnvironmentName:   "test-env",
			Confirm:           true,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.LogOutput{
				Format: "Environment '%s' does not exist or has already been deleted.",
				Params: []any{"test-env"},
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})
}
