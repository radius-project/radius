// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package workspaceswitch // switch is a reserved word in go, so we can't use it as a package name.

import (
	"context"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "switch",
		Short: "Switch current workspace",
		Long:  `Switch current workspace`,
		Example: `# Switch current workspace
rad workspace switch my-workspace`,
		Args: cobra.RangeArgs(0, 1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

type Runner struct {
	ConfigHolder        *framework.ConfigHolder
	ConfigFileInterface framework.ConfigFileInterface
	Output              output.Interface
	WorkspaceName       string
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:        factory.GetConfigHolder(),
		ConfigFileInterface: factory.GetConfigFileInterface(),
		Output:              factory.GetOutput(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// We don't actually need the workspace, but we want to make sure it exists.
	//
	// So this is being called for the side-effect of running the validation.
	workspace, err := cli.RequireWorkspaceArgs(cmd, r.ConfigHolder.Config, args)
	if err != nil {
		return err
	}

	r.WorkspaceName = workspace.Name

	return nil
}

func (r *Runner) Run(ctx context.Context) error {
	section, err := cli.ReadWorkspaceSection(r.ConfigHolder.Config)
	if err != nil {
		return err
	}

	if strings.EqualFold(section.Default, r.WorkspaceName) {
		r.Output.LogInfo("Default environment is already set to %v", r.WorkspaceName)
		return nil
	}

	if section.Default == "" {
		r.Output.LogInfo("Switching default workspace to %v", r.WorkspaceName)
	} else {
		r.Output.LogInfo("Switching default workspace from %v to %v", section.Default, r.WorkspaceName)
	}

	err = r.ConfigFileInterface.SetDefaultWorkspace(ctx, r.ConfigHolder.Config, r.WorkspaceName)
	if err != nil {
		return err
	}

	return nil
}
