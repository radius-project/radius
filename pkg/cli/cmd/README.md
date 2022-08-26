# CLI Commands

This package `pkg/cli/cmd` is the root for our CLI commands. Commands are organized
according to their heirarchy of sub-commands. For example `rad resource show` would be
located in `pkg/cli/cmd/resource/show/show.go`.

Each command is its own page to discourage accidentally sharing code between commands.
Any functionality that needs to be shared should be moved to another location outside of
`pkg/cli/cmd`.

## Template

Here's a useful template for a new (blank) command.

```go
// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import "github.com/spf13/cobra"

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{}

	return cmd
}

type Runner struct {
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	return nil
}

func (r *Runner) Run(cmd *cobra.Command, args []string) error {
	return nil
}
```

Here's a useful template for testing the new command.
```go
// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	config := radcli.LoadConfigWithWorkspace()
	testcases := []radcli.ValidateInput{
		
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