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

package workspaceswitch // switch is a reserved word in go, so we can't use it as a package name.

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/test/radcli"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	config := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "switch explicit workspace flag valid",
			Input:         []string{"-w", radcli.TestWorkspaceName},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "switch explicit workspace positional valid",
			Input:         []string{radcli.TestWorkspaceName},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "switch workspace no-workspace-specified invalid",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "switch workspace not-found invalid",
			Input:         []string{"other-workspace"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "switch workspace too-many-args invalid",
			Input:         []string{"other-workspace", "other-thing"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "switch workspace flag and positional invalid",
			Input:         []string{"other-workspace", "-w", "other-thing"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Switch to current workspace", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		outputSink := &output.MockOutput{}

		config := viper.New()
		cli.UpdateWorkspaceSection(config, cli.WorkspaceSection{
			Default: "current-workspace",
			Items: map[string]workspaces.Workspace{
				"current-workspace": {
					Environment: "test-env",
					Connection:  map[string]any{},
				},
			},
		})

		// No calls expected for this case.
		configFile := framework.NewMockConfigFileInterface(ctrl)

		runner := &Runner{
			ConfigHolder:        &framework.ConfigHolder{Config: config},
			ConfigFileInterface: configFile,
			Output:              outputSink,
			WorkspaceName:       "current-workspace",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.LogOutput{
				Format: "Default environment is already set to %v",
				Params: []any{"current-workspace"},
			},
		}
		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Switch from blank workspace", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		outputSink := &output.MockOutput{}

		config := viper.New()
		cli.UpdateWorkspaceSection(config, cli.WorkspaceSection{
			Default: "", // Blank!
			Items: map[string]workspaces.Workspace{
				"new-workspace": {
					Environment: "test-env",
					Connection:  map[string]any{},
				},
			},
		})

		// This case should edit the configuration
		configFile := framework.NewMockConfigFileInterface(ctrl)
		configFile.EXPECT().
			SetDefaultWorkspace(gomock.Any(), config, "new-workspace").
			Return(nil).
			Times(1)

		runner := &Runner{
			ConfigHolder:        &framework.ConfigHolder{Config: config},
			ConfigFileInterface: configFile,
			Output:              outputSink,
			WorkspaceName:       "new-workspace",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.LogOutput{
				Format: "Switching default workspace to %v",
				Params: []any{"new-workspace"},
			},
		}
		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Switch from one workspace to another", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		outputSink := &output.MockOutput{}

		config := viper.New()
		cli.UpdateWorkspaceSection(config, cli.WorkspaceSection{
			Default: "current-workspace",
			Items: map[string]workspaces.Workspace{
				"current-workspace": {
					Environment: "test-env",
					Connection:  map[string]any{},
				},
				"new-workspace": {
					Environment: "test-env",
					Connection:  map[string]any{},
				},
			},
		})

		// This case should edit the configuration
		configFile := framework.NewMockConfigFileInterface(ctrl)
		configFile.EXPECT().
			SetDefaultWorkspace(gomock.Any(), config, "new-workspace").
			Return(nil).
			Times(1)

		runner := &Runner{
			ConfigHolder:        &framework.ConfigHolder{Config: config},
			ConfigFileInterface: configFile,
			Output:              outputSink,
			WorkspaceName:       "new-workspace",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.LogOutput{
				Format: "Switching default workspace from %v to %v",
				Params: []any{"current-workspace", "new-workspace"},
			},
		}
		require.Equal(t, expected, outputSink.Writes)
	})
}
