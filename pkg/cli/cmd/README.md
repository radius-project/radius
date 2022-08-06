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