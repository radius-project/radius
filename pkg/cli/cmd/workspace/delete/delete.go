// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package delete

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

const (
	deleteConfirmationFmt = "Are you sure you want to delete workspace '%v' from local config [y/N]? This will update config but will not delete any deployed resources."
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete local workspace",
		Long:  `Delete local workspace`,
		Example: `# Delete current workspace
rad workspace delete

# Delete named workspace
rad workspace delete my-workspace`,
		Args: cobra.RangeArgs(0, 1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddConfirmationFlag(cmd)

	return cmd, runner
}

type Runner struct {
	ConfigHolder        *framework.ConfigHolder
	ConfigFileInterface framework.ConfigFileInterface
	Output              output.Interface
	Prompt              prompt.Interface
	Workspace           *workspaces.Workspace
	Confirm             bool
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigFileInterface: factory.GetConfigFileInterface(),
		ConfigHolder:        factory.GetConfigHolder(),
		Prompt:              factory.GetPrompter(),
		Output:              factory.GetOutput(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspaceArgs(cmd, r.ConfigHolder.Config, args)
	if err != nil {
		return err
	}

	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	r.Workspace = workspace
	r.Confirm = yes

	if !r.Workspace.IsNamedWorkspace() {
		// Only workspaces stored in configuration can be deleted.
		return workspaces.ErrNamedWorkspaceRequired
	}

	return nil
}

func (r *Runner) Run(ctx context.Context) error {
	// Prompt user to confirm deletion
	if !r.Confirm {
		message := fmt.Sprintf(deleteConfirmationFmt, r.Workspace.Name)
		confirmed, err := r.Prompt.ConfirmWithDefault(message, prompt.No)
		if err != nil {
			return err
		}

		if !confirmed {
			return nil
		}
	}

	err := r.ConfigFileInterface.DeleteWorkspace(ctx, r.ConfigHolder.Config, r.Workspace.Name)
	if err != nil {
		return err
	}

	return nil
}
