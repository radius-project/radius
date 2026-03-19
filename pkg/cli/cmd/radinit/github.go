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

package radinit

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/cli/gitstate"
	"github.com/radius-project/radius/pkg/cli/k3d"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"github.com/radius-project/radius/pkg/version"
)

// enterGitHubInitOptions gathers options for a GitHub workspace kind init.
//
// It opens (or creates) a git worktree for the state orphan branch. State files from any
// previous run are already present in the worktree directory — they are never written to
// the application working tree. It then records the semaphore state, writes a lock to
// signal that a deploy is in progress, and creates the k3d cluster.
func (r *Runner) enterGitHubInitOptions(ctx context.Context) (*initOptions, *workspaces.Workspace, error) {
	if err := k3d.EnsureInstalled(ctx); err != nil {
		return nil, nil, err
	}

	// Open (or create) the state worktree in a temp directory isolated from the app checkout.
	w, err := gitstate.OpenOrCreate(ctx, gitstate.DefaultBranch)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open state worktree: %w", err)
	}
	r.Worktree = w

	// Capture semaphore state BEFORE writing the lock so we know whether to restore later.
	r.GitHubSemaphoreState = w.CheckSemaphore()

	// Write .lock and push to signal that a deploy is in progress.
	// If the push fails (no remote) it is logged as a warning and we continue.
	if err := w.WriteLock(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to write deploy lock: %w", err)
	}

	clusterName := k3d.DefaultClusterName
	kubeContext, err := k3d.CreateCluster(ctx, clusterName)
	if err != nil {
		return nil, nil, err
	}

	options := &initOptions{
		Cluster: clusterOptions{
			Install:   true,
			Context:   kubeContext,
			Namespace: "radius-system",
			Version:   version.Version(),
		},
		Environment: environmentOptions{
			Create:    true,
			Name:      "default",
			Namespace: "default",
		},
		Recipes: recipePackOptions{
			DevRecipes: true,
		},
		// Enable PostgreSQL in the Helm chart.
		SetValues: []string{"database.enabled=true"},
	}

	workspace := &workspaces.Workspace{
		Name: "default",
		Connection: map[string]any{
			"kind":    workspaces.KindGitHub,
			"context": kubeContext,
		},
		Scope:       fmt.Sprintf("/planes/radius/local/resourceGroups/%s", options.Environment.Name),
		Environment: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/Applications.Core/environments/%s", options.Environment.Name, options.Environment.Name),
	}

	return options, workspace, nil
}

// runGitHubPostInstall performs post-install steps for the GitHub workspace kind:
//  1. Waits for PostgreSQL to be ready.
//  2. If the previous run completed cleanly (SemaphoreClean), restores the PostgreSQL state
//     from the backup files that are already present in the worktree directory.
//  3. After a successful restore, attempts to sync resource types from resource-types-contrib.
//     If the sync fails, logs a warning and continues using only the restored state rather
//     than failing the entire init — the user should resolve any type conflicts manually.
//  4. Logs an appropriate message for interrupted or first-run states.
func (r *Runner) runGitHubPostInstall(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	if r.Worktree == nil {
		return fmt.Errorf("state worktree is not initialised; this is a bug")
	}

	kubeContext, _ := r.Workspace.KubernetesContext()
	namespace := r.Options.Cluster.Namespace
	stateDir := r.Worktree.Path

	if err := r.PGBackupClient.WaitForReady(ctx, kubeContext, namespace); err != nil {
		return fmt.Errorf("failed waiting for PostgreSQL: %w", err)
	}

	switch r.GitHubSemaphoreState {

	case gitstate.SemaphoreInterrupted:
		// The previous runner was evicted mid-deploy (spot instance or similar).
		// The backup files in the worktree may be from an earlier successful run, but
		// the semaphore signals that the last deploy was incomplete, so we skip restore
		// to avoid applying a potentially stale state.
		logger.Info("Previous run was interrupted (spot instance eviction?); skipping state restore. Manual intervention may be required if partial state was applied.")

	case gitstate.SemaphoreFirstRun:
		logger.Info("First run detected; no prior state to restore.")

	case gitstate.SemaphoreClean:
		if !r.PGBackupClient.HasBackup(stateDir) {
			logger.Info("State branch is clean but contains no backup files; skipping restore.")
			break
		}

		logger.Info("Restoring PostgreSQL state from previous run", "stateDir", stateDir)
		if err := r.PGBackupClient.Restore(ctx, kubeContext, namespace, stateDir); err != nil {
			return fmt.Errorf("failed to restore PostgreSQL state: %w", err)
		}

		// TODO(github-workspace): After restore, sync resource types from resource-types-contrib:
		//   rad resource type sync --source oci://ghcr.io/radius-project/resource-types-contrib:latest
		//
		// The sync must happen AFTER restore so the restored types form the baseline. If the sync
		// detects a conflict (e.g. an attribute set was altered upstream), it should return an error
		// here and the caller should log a warning and proceed with saved state only — never silently
		// overwrite user data. Implement once 'rad resource type sync' exists.
		logger.Info("State restored. Resource-type sync from resource-types-contrib is not yet implemented; types reflect the saved state only.")
	}

	return nil
}
