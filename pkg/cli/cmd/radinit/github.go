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
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/k3d"
	"github.com/radius-project/radius/pkg/cli/pgbackup"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/version"
)

// enterGitHubInitOptions gathers options for a GitHub workspace type init.
// It creates a k3d cluster, installs Radius with PostgreSQL, and optionally restores state.
func (r *Runner) enterGitHubInitOptions(ctx context.Context, stateDir string) (*initOptions, *workspaces.Workspace, error) {
	// Ensure k3d is available.
	if err := k3d.EnsureInstalled(ctx); err != nil {
		return nil, nil, err
	}

	clusterName := k3d.DefaultClusterName

	// Create or reuse k3d cluster.
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
			"kind":     workspaces.KindGitHub,
			"context":  kubeContext,
			"stateDir": stateDir,
		},
		Scope:       fmt.Sprintf("/planes/radius/local/resourceGroups/%s", options.Environment.Name),
		Environment: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/Applications.Core/environments/%s", options.Environment.Name, options.Environment.Name),
	}

	return options, workspace, nil
}

// runGitHubPostInstall performs post-install steps for GitHub workspace type:
// waits for PostgreSQL readiness and restores state if a backup exists.
func (r *Runner) runGitHubPostInstall(ctx context.Context) error {
	kubeContext, _ := r.Workspace.KubernetesContext()
	stateDir := r.Workspace.StateDir()
	namespace := r.Options.Cluster.Namespace

	// Wait for PostgreSQL to be ready.
	if err := pgbackup.WaitForReady(ctx, kubeContext, namespace); err != nil {
		return fmt.Errorf("failed waiting for PostgreSQL: %w", err)
	}

	// Try to restore state from the git orphan branch first.
	_ = gitstate.RestoreState(ctx, stateDir, gitstate.DefaultBranch)

	// If backup files exist, restore them into PostgreSQL.
	if pgbackup.HasBackup(stateDir) {
		if err := pgbackup.Restore(ctx, kubeContext, namespace, stateDir); err != nil {
			return fmt.Errorf("failed to restore PostgreSQL state: %w", err)
		}
	}

	return nil
}

// installRadiusForGitHub installs Radius with GitHub-specific Helm values (database.enabled=true).
func (r *Runner) installRadiusForGitHub(ctx context.Context) error {
	cliOptions := helm.CLIClusterOptions{
		Radius: helm.ChartOptions{
			SetArgs:     append(r.Options.SetValues, r.Set...),
			SetFileArgs: r.SetFile,
		},
	}

	clusterOptions := helm.PopulateDefaultClusterOptions(cliOptions)

	if err := r.HelmInterface.InstallRadius(ctx, clusterOptions, r.Options.Cluster.Context); err != nil {
		return fmt.Errorf("failed to install Radius: %w", err)
	}

	return nil
}
