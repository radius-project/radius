# CLI Commands

This package `pkg/cli/cmd` is the root for our CLI commands. Commands are organized
according to their heirarchy of sub-commands. For example `rad resource show` would be
located in `pkg/cli/cmd/resource/show/show.go`.

Some of our command names are reserved words in Go and so they can't be used as package names.
When this happens add a prefix from the parent command. eg: `switch` -> `appswitch`.

Each command is its own package to discourage accidentally sharing code between commands.
Any functionality that needs to be shared should be moved to another location outside of
`pkg/cli/cmd`.

Make sure to run `make test-validate-cli` to get the test coverage for the file you have added tests.

## Template

Here's a useful template for a new (blank) command.

```go
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

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad <fill in the blank>` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "",
		Short:   "",
		Long:    "",
		Example: ``,
		Args:    cobra.ExactArgs(2),
		RunE:    framework.RunCommand(runner),
	}

	// Define your flags here
	commonflags.AddOutputFlag(cmd)
	cmd.Flags().StringP("flagName", "k (flag's shorthand notation like w for workspace)", "", "What does the flag ask for")

	return cmd, runner
}

// Runner is the Runner implementation for the `rad <fill in the blank>` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	Format       string
	Workspace    *workspaces.Workspace
}

// NewRunner creates an instance of the runner for the `rad <fill in the blank>` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

// Validate runs validation for the `rad <fill in the blank>` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Validate command line args and
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	return nil
}

// Run runs the `rad <fill in the blank>` command.
func (r *Runner) Run(ctx context.Context) error {
	// Implement your command here
	return nil
}
```

Here's a useful template for testing the new command.

```go
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
	"testing"

	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	config := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "example validation test",
			Input:         []string{"show", "-s", "cool-value"},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Validate Scenario 1", func(t *testing.T) {
		
	})
	t.Run("Validate Scenario 2", func(t *testing.T) {
		
	})
	t.Run("Validate Scenario 3", func(t *testing.T) {
		
	})
	t.Run("Validate Scenario i", func(t *testing.T) {
		
	})
}
```