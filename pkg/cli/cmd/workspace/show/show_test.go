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

	"github.com/radius-project/radius/pkg/cli/cmd/workspace/common"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/test/radcli"
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
				Options: common.WorkspaceFormat(),
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
				Options: common.WorkspaceFormat(),
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})
}
