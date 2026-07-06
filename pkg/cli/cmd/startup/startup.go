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

// Package startup implements the `rad startup` command, which restores durable Radius state
// (PostgreSQL control-plane databases and Terraform state Secrets) previously saved by
// `rad shutdown` into the current workspace's running control plane.
//
// The command assumes Radius is already installed and running on the target cluster; it does not
// create clusters or install Radius. This keeps it usable in any context and independent of any
// particular workspace kind.
package startup

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
	storagegit "github.com/radius-project/radius/pkg/storage/git"
)

// NewCommand creates an instance of the `rad startup` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "startup",
		Short: "Restore Radius state after startup",
		Long: `Restore durable Radius state for the current workspace.

Opens the radius-state git orphan branch, waits for the control-plane PostgreSQL instance to be
ready, restores the control-plane databases, and re-creates the Terraform recipe state Secrets.
Run this after Radius is installed on a fresh cluster to resume from the state saved by
'rad shutdown'.

This command does not create the cluster or install Radius.`,
		Example: `
# Restore state for the current workspace
rad startup

# Restore state for a specific workspace
rad startup --workspace my-workspace`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// worktreeHandle decouples Run from the concrete storage session so the command can be tested
// without performing real git operations.
type worktreeHandle struct {
	path   string
	remove func(ctx context.Context)
}

// Runner is the runner implementation for the `rad startup` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	Workspace    *workspaces.Workspace
	StateClient  StateRestoreClient

	// openWorktree opens (or creates) the state worktree. Overridable in tests.
	openWorktree func(ctx context.Context) (worktreeHandle, error)

	// newScaler builds the control-plane scaler for a context/namespace. Overridable in tests.
	newScaler func(kubeContext, namespace string) (ControlPlaneScaler, error)
}

// NewRunner creates a new Runner for the `rad startup` command.
func NewRunner(factory framework.Factory) *Runner {
	r := &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
		StateClient:  NewStateRestoreClient(),
	}
	r.openWorktree = defaultOpenWorktree
	r.newScaler = newScalerForContext
	return r
}

// defaultOpenWorktree opens a git-backed storage session for the state branch and adapts it to
// worktreeHandle.
func defaultOpenWorktree(ctx context.Context) (worktreeHandle, error) {
	session, err := storagegit.NewBackend().Open(ctx, pgbackup.StateBranchName())
	if err != nil {
		return worktreeHandle{}, err
	}
	return worktreeHandle{
		path:   session.Path(),
		remove: session.Close,
	}, nil
}

// Validate resolves the workspace and ensures it targets a Kubernetes cluster.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}

	if _, ok := workspace.KubernetesContext(); !ok {
		return clierrors.Message("The 'rad startup' command requires a workspace connected to a Kubernetes cluster. Workspace %q is not connected to a Kubernetes cluster.", workspace.Name)
	}

	r.Workspace = workspace
	return nil
}

// Run restores the control-plane and Terraform state from the state branch.
//
// The database-backed control-plane deployments are scaled to zero before the restore and back up
// afterward. Restoring a pg_dump underneath live resource-provider connections would invalidate
// their cached prepared statements and race their writes; quiescing them makes the restore atomic
// with respect to its consumers without a separate restart step.
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

	scaler, err := r.newScaler(kubeContext, pgbackup.DefaultNamespace)
	if err != nil {
		return fmt.Errorf("failed to initialise control-plane scaler: %w", err)
	}

	r.Output.LogInfo("Scaling down control-plane components...")
	saved, err := scaler.ScaleDown(ctx)
	if err != nil {
		return fmt.Errorf("failed to scale down the control plane: %w", err)
	}

	// Ensure the control plane is brought back up even if a restore step fails.
	scaledBackUp := false
	defer func() {
		if scaledBackUp {
			return
		}
		if upErr := scaler.ScaleUp(ctx, saved); upErr != nil {
			r.Output.LogInfo("Warning: failed to scale the control plane back up: %v", upErr)
		}
	}()

	r.Output.LogInfo("Waiting for control-plane database to be ready...")
	if err := r.StateClient.WaitForDatabaseReady(ctx, kubeContext, pgbackup.DefaultNamespace); err != nil {
		return fmt.Errorf("failed waiting for control-plane database: %w", err)
	}

	r.Output.LogInfo("Restoring control-plane databases...")
	if err := r.StateClient.RestoreDatabases(ctx, kubeContext, pgbackup.DefaultNamespace, wt.path); err != nil {
		return fmt.Errorf("failed to restore control-plane databases: %w", err)
	}

	r.Output.LogInfo("Restoring Terraform recipe state...")
	if err := r.StateClient.RestoreTerraform(ctx, kubeContext, pgbackup.DefaultNamespace, wt.path); err != nil {
		return fmt.Errorf("failed to restore Terraform state: %w", err)
	}

	r.Output.LogInfo("Scaling control-plane components back up...")
	if err := scaler.ScaleUp(ctx, saved); err != nil {
		return fmt.Errorf("failed to scale the control plane back up: %w", err)
	}
	scaledBackUp = true

	r.Output.LogInfo("State restored successfully.")
	return nil
}
