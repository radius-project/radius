// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	config := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "show current workspace valid",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "show fallback workspace",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: radcli.LoadEmptyConfig(t)},
		},
		{
			Name:          "show explicit workspace flag valid",
			Input:         []string{"-w", radcli.TestWorkspaceName},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "show explicit workspace positional valid",
			Input:         []string{radcli.TestWorkspaceName},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "show workspace not-found invalid",
			Input:         []string{"other-workspace"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "show workspace too-many-args invalid",
			Input:         []string{"other-workspace", "other-thing"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "show workspace flag and positional invalid",
			Input:         []string{"other-workspace", "-w", "other-thing"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Show named workspace", func(t *testing.T) {
		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConfigHolder: &framework.ConfigHolder{},
			Output:       outputSink,
			Workspace: &workspaces.Workspace{
				Name:        "test-workspace",
				Environment: "test-environment",
				Connection:  map[string]any{},
			},
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.FormattedOutput{
				Format: "",
				Obj: &workspaces.Workspace{
					Name:        "test-workspace",
					Environment: "test-environment",
					Connection:  map[string]any{},
				},
				Options: objectformats.GetWorkspaceTableFormat(),
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Show fallback workspace", func(t *testing.T) {
		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConfigHolder: &framework.ConfigHolder{},
			Output:       outputSink,
			Workspace:    workspaces.MakeFallbackWorkspace(),
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.FormattedOutput{
				Format:  "",
				Obj:     runner.Workspace,
				Options: objectformats.GetWorkspaceTableFormat(),
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})
}
