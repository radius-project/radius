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

package list

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/test/radcli"
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
			Name:          "list workspaces valid",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "list workspaces with format",
			Input:         []string{"-o", "yaml"},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "list workspaces too-many-args invalid",
			Input:         []string{"another-arg"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("List workspaces", func(t *testing.T) {
		config := viper.New()
		cli.UpdateWorkspaceSection(config, cli.WorkspaceSection{
			Items: map[string]workspaces.Workspace{
				// Intentionally NOT in alphabetical order
				"workspace-b": {
					Environment: "b",
					Source:      workspaces.SourceUserConfig,
					Connection:  map[string]any{},
				},
				"workspace-a": {
					Environment: "a",
					Source:      workspaces.SourceUserConfig,
					Connection:  map[string]any{},
				},
			},
		})

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConfigHolder: &framework.ConfigHolder{
				Config: config,
			},
			Output: outputSink,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.FormattedOutput{
				Format: "",
				Obj: []workspaces.Workspace{
					{
						Name:        "workspace-a",
						Environment: "a",
						Source:      workspaces.SourceUserConfig,
						Connection:  map[string]any{},
					},
					{
						Name:        "workspace-b",
						Environment: "b",
						Source:      workspaces.SourceUserConfig,
						Connection:  map[string]any{},
					},
				},
				Options: objectformats.GetWorkspaceTableFormat(),
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})
}
