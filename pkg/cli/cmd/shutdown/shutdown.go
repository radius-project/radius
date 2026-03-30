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

Backs up PostgreSQL state to the state worktree, commits everything to the
radius-state orphan branch (including .backup-ok sentinel), pushes to origin,
and deletes the k3d cluster.`,
		Example: `
# Shut down the current GitHub workspace
rad shutdown`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// worktreeHandle bundles the callbacks returned by openWorktree so Run can
// call them without holding a concrete *gitstate.StateWorktree.
type worktreeHandle struct {
	path       string
	clearLock  func(context.Context) error
	removeFunc func(context.Context)
}

// Runner is the runner implementation for the `rad shutdown` command.
type Runner struct {
	ConfigHolder   *framework.ConfigHolder
	Output         output.Interface
	Workspace      *workspaces.Workspace
	PGBackupClient PGBackupClient

	// openWorktree opens (or creates) the state worktree and returns a handle.
	// Overridable in tests to avoid real git operations.
	openWorktree func(ctx context.Context) (worktreeHandle, error)

	// deleteCluster deletes a k3d cluster by name. Overridable in tests.
	deleteCluster func(ctx context.Context, name string) error
}

// NewRunner creates a new instance of the Runner for the `rad shutdown` command.
func NewRunner(factory framework.Factory) *Runner {
	r := &Runner{
		ConfigHolder:   factory.GetConfigHolder(),
		Output:         factory.GetOutput(),
		PGBackupClient: NewPGBackupClient(),
		deleteCluster:  k3d.DeleteCluster,
	}
	r.openWorktree = func(ctx context.Context) (worktreeHandle, error) {
		w, err := gitstate.OpenOrCreate(ctx, gitstate.BranchName())
		if err != nil {
			return worktreeHandle{}, err
		}
		return worktreeHandle{
			path:       w.Path,
			clearLock:  w.ClearLock,
			removeFunc: w.Remove,
		}, nil
	}
	return r
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

	return nil
}

// Run runs the `rad shutdown` command.
func (r *Runner) Run(ctx context.Context) error {
	kubeContext, ok := r.Workspace.KubernetesContext()
	if !ok {
		return clierrors.Message("Could not determine Kubernetes context for workspace '%s'.", r.Workspace.Name)
	}

	wt, err := r.openWorktree(ctx)
	if err != nil {
		return fmt.Errorf("failed to open state worktree: %w", err)
	}
	defer wt.removeFunc(ctx)

	r.Output.LogInfo("Backing up PostgreSQL state to state worktree...")
	if err := r.PGBackupClient.Backup(ctx, kubeContext, pgbackup.DefaultNamespace, wt.path); err != nil {
		return fmt.Errorf("failed to back up PostgreSQL state: %w", err)
	}

	r.Output.LogInfo("Committing state and clearing deploy lock on branch '%s'...", gitstate.BranchName())
	if err := wt.clearLock(ctx); err != nil {
		return fmt.Errorf("failed to commit and push state: %w", err)
	}

	r.Output.LogInfo("State committed and pushed to branch '%s'.", gitstate.BranchName())

	clusterName := k3d.DefaultClusterName
	if c, ok := r.Workspace.Connection["cluster"]; ok {
		if s, ok := c.(string); ok {
			clusterName = s
		}
	}

	r.Output.LogInfo("Deleting k3d cluster '%s'...", clusterName)
	if err := r.deleteCluster(ctx, clusterName); err != nil {
		return fmt.Errorf("failed to delete k3d cluster: %w", err)
	}

	r.Output.LogInfo("Cluster deleted.")

	return nil
}
