// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package list

import (
	"context"
	"sort"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

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

type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	Format       string
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	r.Format = format

	return nil
}

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
