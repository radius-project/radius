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
	"sort"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad workspace list` command.
//

// NewCommand creates a new cobra command for listing local workspaces and returns a Runner to execute the command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List local workspaces",
		Long:  `List local workspaces`,
		Example: `# List workspaces
rad workspace list`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	return cmd, runner
}

// Runner is the runner implementation for the `rad workspace list` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	Format       string
}

// NewRunner creates a new instance of the `rad workspace list` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

// Validate runs validation for the `rad workspace list` command.
//

// Validate checks the output format of the command and sets the format in the Runner struct, returning
// an error if the output format is invalid.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	r.Format = format

	return nil
}

// Run runs the `rad workspace list` command.
//

// Run reads the workspace section from the config, puts the workspace names in alphabetical order, creates a slice of workspaces,
// and then writes the formatted workspaces to the output. It returns an error if any of these steps fail.
func (r *Runner) Run(ctx context.Context) error {
	section, err := cli.ReadWorkspaceSection(r.ConfigHolder.Config)
	if err != nil {
		return err
	}

	// Put in alphabetical order in a slice
	names := []string{}
	for name := range section.Items {
		names = append(names, name)
	}

	sort.Strings(names)

	items := []workspaces.Workspace{}
	for _, name := range names {
		items = append(items, section.Items[name])
	}

	err = r.Output.WriteFormatted(r.Format, items, objectformats.GetWorkspaceTableFormat())
	if err != nil {
		return err
	}

	return nil
}
