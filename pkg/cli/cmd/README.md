# CLI Commands

This package `pkg/cli/cmd` is the root for our CLI commands. Commands are organized
according to their heirarchy of sub-commands. For example `rad resource show` would be
located in `pkg/cli/cmd/resource/show/show.go`.

Each command is its own page to discourage accidentally sharing code between commands.
Any functionality that needs to be shared should be moved to another location outside of
`pkg/cli/cmd`.

Make sure to run `make test-validate-cli` to get the test coverage for the file you have added tests.

## Template

Here's a useful template for a new (blank) command.

```go
// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

import "github.com/spf13/cobra"

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "",
		Short: "",
		Long:  "",
		Example: ``,
		Args: cobra.ExactArgs(),
		RunE: framework.RunCommand(runner),
	}

	outputDescription := fmt.Sprintf("output format (supported formats are %s)", strings.Join(output.SupportedFormats(), ", "))
	// Define your flags here
	cmd.Flags().StringP("flagName", "k (flag's shorthand notation like w for workspace)", "", "What does the flag ask for")

	return cmd, runner
}

type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	Output            output.Interface
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
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

func (r *Runner) Run(cmd *cobra.Command, args []string) error {
	err = r.Output.Write(r.Format, resourceDetails, objectformats.GetResourceTableFormat())
	if err != nil {
		return err
	}

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