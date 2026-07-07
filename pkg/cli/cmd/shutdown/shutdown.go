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

// Package shutdown implements the `rad shutdown` command, which backs up all durable Radius state
// (PostgreSQL control-plane databases and Terraform state Secrets) for the current workspace to a
// git orphan branch. It is the counterpart of `rad startup`.
//
// The command does not delete clusters or uninstall Radius; cluster lifecycle is the caller's
// responsibility. This keeps the command usable in any context and independent of any particular
// workspace kind.
package shutdown

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/pgbackup"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	archivegit "github.com/radius-project/radius/pkg/statearchive/git"
)

// NewCommand creates an instance of the `rad shutdown` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "shutdown",
		Short: "Back up Radius state and prepare for shutdown",
		Long: `Back up all durable Radius state for the current workspace.

Dumps the control-plane PostgreSQL databases and exports the Terraform recipe state Secrets,
commits them to the radius-state git orphan branch, and pushes to the remote when one is
configured. The state can be restored into a fresh control plane with 'rad startup'.

This command does not delete the cluster or uninstall Radius.`,
		Example: `
# Back up state for the current workspace
rad shutdown

# Back up state for a specific workspace
rad shutdown --workspace my-workspace`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// worktreeHandle decouples Run from the concrete storage session so the command can be tested
// without performing real git operations.
type worktreeHandle struct {
	path          string
	commitAndPush func(ctx context.Context, message string) error
	remove        func(ctx context.Context)
}

// Runner is the runner implementation for the `rad shutdown` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	Workspace    *workspaces.Workspace
	StateClient  StateBackupClient

	// openWorktree opens (or creates) the state worktree. Overridable in tests.
	openWorktree func(ctx context.Context) (worktreeHandle, error)
}

// NewRunner creates a new Runner for the `rad shutdown` command.
func NewRunner(factory framework.Factory) *Runner {
	r := &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
		StateClient:  NewStateBackupClient(),
	}
	r.openWorktree = defaultOpenWorktree
	return r
}

// defaultOpenWorktree opens a git-backed state-archive session for the state branch and adapts it
// to worktreeHandle.
func defaultOpenWorktree(ctx context.Context) (worktreeHandle, error) {
	session, err := archivegit.NewGitArchive().Open(ctx, pgbackup.StateBranchName())
	if err != nil {
		return worktreeHandle{}, err
	}
	return worktreeHandle{
		path:          session.Path(),
		commitAndPush: session.Commit,
		remove:        session.Close,
	}, nil
}

// Validate resolves the workspace and ensures it targets a Kubernetes cluster.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}

	if _, ok := workspace.KubernetesContext(); !ok {
		return clierrors.Message("The 'rad shutdown' command requires a workspace connected to a Kubernetes cluster. Workspace %q is not connected to a Kubernetes cluster.", workspace.Name)
	}

	r.Workspace = workspace
	return nil
}

// Run backs up the control-plane and Terraform state, then commits and pushes it.
func (r *Runner) Run(ctx context.Context) error {
	kubeContext, ok := r.Workspace.KubernetesContext()
	if !ok {
		return clierrors.Message("Could not determine the Kubernetes context for workspace %q.", r.Workspace.Name)
	}

	wt, err := r.openWorktree(ctx)
	if err != nil {
		return fmt.Errorf("failed to open state worktree: %w", err)
	}
	defer wt.remove(ctx)

	r.Output.LogInfo("Backing up control-plane databases...")
	if err := r.StateClient.BackupDatabases(ctx, kubeContext, pgbackup.DefaultNamespace, wt.path); err != nil {
		return fmt.Errorf("failed to back up control-plane databases: %w", err)
	}

	r.Output.LogInfo("Backing up Terraform recipe state...")
	if err := r.StateClient.BackupTerraform(ctx, kubeContext, pgbackup.DefaultNamespace, wt.path); err != nil {
		return fmt.Errorf("failed to back up Terraform state: %w", err)
	}

	r.Output.LogInfo("Committing state to branch %q...", pgbackup.StateBranchName())
	if err := wt.commitAndPush(ctx, "radius: shutdown backup"); err != nil {
		return fmt.Errorf("failed to commit and push state: %w", err)
	}

	r.Output.LogInfo("State backed up successfully.")
	return nil
}
