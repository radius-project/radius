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

package kubernetes

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad rollback kubernetes` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "kubernetes",
		Short: "Rolls back Radius on a Kubernetes cluster",
		Long: `Roll back Radius in a Kubernetes cluster to a previous revision.

This command rolls back the Radius control plane in the cluster associated with the active workspace.
By default, it rolls back to the previous successful deployment with an older version (n-1 revision).
You can also specify a specific revision number to rollback to.

The rollback operation will:
- Check that Radius is currently installed
- Verify that the target revision exists and is valid
- Roll back to the specified revision or previous version
- Wait for the rollback to complete

This command operates on the cluster associated with the active workspace.
To rollback Radius in a different cluster, switch to the appropriate workspace first using 'rad workspace switch'.

Radius is installed in the 'radius-system' namespace. For more information visit https://docs.radapp.io/concepts/technical/architecture/
`,
		Example: `# Rollback Radius to the previous version in the cluster of the active workspace
rad rollback kubernetes

# Rollback Radius to a specific revision number
rad rollback kubernetes --revision 3

# Check which workspace is active  
rad workspace show

# Switch to a different workspace before rolling back
rad workspace switch myworkspace
rad rollback kubernetes --revision 2
`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddKubeContextFlagVar(cmd, &runner.KubeContext)
	cmd.Flags().IntVar(&runner.Revision, "revision", 0, "Specify the revision number to rollback to (defaults to previous version)")

	return cmd, runner
}

// Runner is the Runner implementation for the `rad rollback kubernetes` command.
type Runner struct {
	Helm   helm.Interface
	Output output.Interface

	KubeContext string
	Revision    int
}

// NewRunner creates an instance of the runner for the `rad rollback kubernetes` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Helm:   factory.GetHelmInterface(),
		Output: factory.GetOutput(),
	}
}

// Validate runs validation for the `rad rollback kubernetes` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	return nil
}

// Run runs the `rad rollback kubernetes` command.
func (r *Runner) Run(ctx context.Context) error {
	// Check current installation state
	state, err := r.Helm.CheckRadiusInstall(r.KubeContext)
	if err != nil {
		return fmt.Errorf("failed to check current Radius installation: %w", err)
	}

	if !state.RadiusInstalled {
		return fmt.Errorf("Radius is not currently installed. Use 'rad install kubernetes' to install Radius first")
	}

	r.Output.LogInfo("Current Radius version: %s", state.RadiusVersion)

	if r.Revision != 0 {
		// Specific revision rollback
		r.Output.LogInfo("Rolling back to specified revision %d...", r.Revision)
		err = r.Helm.RollbackRadiusToRevision(ctx, r.KubeContext, r.Revision)
		if err != nil {
			return fmt.Errorf("failed to rollback Radius to revision %d: %w", r.Revision, err)
		}
	} else {
		// Automatic rollback to previous version
		r.Output.LogInfo("Checking for previous revisions...")
		err = r.Helm.RollbackRadius(ctx, r.KubeContext)
		if err != nil {
			return fmt.Errorf("failed to rollback Radius: %w", err)
		}
	}

	r.Output.LogInfo("✓ Radius rollback completed successfully!")
	return nil
}
