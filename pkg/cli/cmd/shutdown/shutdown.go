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

package shutdown

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/gitstate"
	"github.com/radius-project/radius/pkg/cli/k3d"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/pgbackup"
	"github.com/radius-project/radius/pkg/cli/workspaces"
)

// NewCommand creates an instance of the `rad shutdown` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "shutdown",
		Short: "Shut down a Radius GitHub workspace",
		Long: `Shut down a Radius GitHub workspace.

Backs up PostgreSQL state to the state directory, commits the state to the
radius-state orphan branch, and optionally deletes the k3d cluster.

Note: after running this command, run 'git push origin radius-state' to push
the state to the remote repository.`,
		Example: `
# Shut down the current GitHub workspace
rad shutdown

# Shut down and delete the k3d cluster
rad shutdown --cleanup`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	cmd.Flags().Bool("cleanup", false, "Delete the k3d cluster after backing up state")

	return cmd, runner
}

// Runner is the runner implementation for the `rad shutdown` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	Workspace    *workspaces.Workspace
	Cleanup      bool
}

// NewRunner creates a new instance of the Runner for the `rad shutdown` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

// Validate runs validation for the `rad shutdown` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspaceArgs(cmd, r.ConfigHolder.Config, args)
	if err != nil {
		return err
	}

	if workspace.Connection["kind"] != workspaces.KindGitHub {
		return clierrors.Message("The 'rad shutdown' command is only supported for workspaces of kind '%s'. The current workspace '%s' has kind '%s'.",
			workspaces.KindGitHub, workspace.Name, workspace.Connection["kind"])
	}

	r.Workspace = workspace

	cleanup, err := cmd.Flags().GetBool("cleanup")
	if err != nil {
		return err
	}
	r.Cleanup = cleanup

	return nil
}

// Run runs the `rad shutdown` command.
func (r *Runner) Run(ctx context.Context) error {
	kubeContext, ok := r.Workspace.KubernetesContext()
	if !ok {
		return clierrors.Message("Could not determine Kubernetes context for workspace '%s'.", r.Workspace.Name)
	}

	stateDir := r.Workspace.StateDir()

	r.Output.LogInfo("Backing up PostgreSQL state to %s...", stateDir)
	if err := pgbackup.Backup(ctx, kubeContext, pgbackup.DefaultNamespace, stateDir); err != nil {
		return fmt.Errorf("failed to back up PostgreSQL state: %w", err)
	}

	r.Output.LogInfo("Committing state to git branch '%s'...", gitstate.DefaultBranch)
	if err := gitstate.CommitState(ctx, stateDir, gitstate.DefaultBranch); err != nil {
		return fmt.Errorf("failed to commit state: %w", err)
	}

	r.Output.LogInfo("State committed. Run 'git push origin %s' to push to the remote repository.", gitstate.DefaultBranch)

	if r.Cleanup {
		clusterName := k3d.DefaultClusterName
		if c, ok := r.Workspace.Connection["cluster"]; ok {
			if s, ok := c.(string); ok {
				clusterName = s
			}
		}

		r.Output.LogInfo("Deleting k3d cluster '%s'...", clusterName)
		if err := k3d.DeleteCluster(ctx, clusterName); err != nil {
			return fmt.Errorf("failed to delete k3d cluster: %w", err)
		}

		r.Output.LogInfo("Cluster deleted.")
	}

	return nil
}
