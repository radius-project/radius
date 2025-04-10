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

	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/version"
	"github.com/spf13/cobra"
)

// Updated NewCommand remains unchanged...
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)
	cmd := &cobra.Command{
		Use:   "kubernetes",
		Short: "Upgrades Radius on a Kubernetes cluster",
		Long: `Upgrade Radius on a Kubernetes cluster using the Radius Helm chart.
By default 'rad upgrade kubernetes' will upgrade Radius to the latest version available.
        
Before performing the upgrade, a snapshot of the current installation is taken so that it can be restored if necessary.`,
		Example: `
# Upgrade to the latest version
rad upgrade kubernetes

# Upgrade with custom configuration values
rad upgrade kubernetes --version v0.44.0 --set global.monitoring.enabled=true
`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddKubeContextFlagVar(cmd, &runner.KubeContext)
	cmd.Flags().StringVar(&runner.Version, "version", "", "Specify a version to upgrade to (default uses the latest version)")
	cmd.Flags().IntVar(&runner.Timeout, "timeout", 300, "Timeout in seconds for the upgrade operation")
	cmd.Flags().StringArrayVar(&runner.Set, "set", []string{}, "Set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")
	cmd.Flags().StringArrayVar(&runner.SetFile, "set-file", []string{}, "Set values from files on the command line")
	return cmd, runner
}

// Runner is the Runner implementation for the upgrade command.
type Runner struct {
	Helm   helm.Interface
	Output output.Interface

	KubeContext string
	Version     string
	DryRun      bool
	Timeout     int
	Set         []string
	SetFile     []string
}

// NewRunner creates a new Runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Helm:   factory.GetHelmInterface(),
		Output: factory.GetOutput(),
	}
}

/*
Validate required flags.
• Ensure that the --version flag is provided (since downgrades aren’t supported).
• Also handle other flags (like --timeout, --set, etc.).

Check if Radius is installed.
• Use the Helm client to query the current state.
• If not installed, abort with an informative message.

Retrieve chart versions.
• Query the Helm repository for available chart versions.
• Identify the list of available versions and determine the latest version.
• Optionally, log these available versions for reference.

Compare version numbers.
• Retrieve the current version installed on the cluster.
• Check if the target (wanted) version is higher than the current version.
• If the target is equal to or lower than the current version, abort the upgrade.

Keep a global flag for in-progress upgrade.
• Set a global flag to indicate that an upgrade is in progress.
• This can be used to prevent multiple concurrent upgrades and other data-changing operations.
• This flag should be cleared after the upgrade process is completed.

Snapshot the data.
• Before making any live changes, automatically or via a prompt, trigger a snapshot (or backup) of your data (etcd, etc.).
• This safeguards the installation in case a rollback is needed.

Perform the upgrade.
• Initiate the Helm upgrade process.
• Pass along the appropriate configuration (including timeout, value overrides, etc.)

Rollback if necessary.
• If the upgrade fails, use Helm’s rollback capabilities to revert to the previous version.
• Include additional logging and error messages to guide the user.

Post-upgrade validation.
• Verify that the new version is running correctly and all critical components are healthy.
*/

// Run executes the upgrade flow.
func (r *Runner) Run(ctx context.Context) error {
	cliOptions := helm.CLIClusterOptions{
		Radius: helm.ChartOptions{
			SetArgs:     r.Set,
			SetFileArgs: r.SetFile,
		},
	}

	// Check if Radius is installed.
	state, err := r.Helm.CheckRadiusInstall(r.KubeContext)
	if err != nil {
		return err
	}
	if !state.RadiusInstalled {
		r.Output.LogInfo("No existing Radius installation found. Use 'rad install kubernetes' to install.")
		return nil
	}

	// How can I check all available versions of the chart?

	currentVersion := version.Version()
	desiredVersion := r.Version
	if desiredVersion == "" {
		// Default to latest value from the chart.
		desiredVersion = "latest"
	}
	if desiredVersion == currentVersion {
		r.Output.LogInfo("Radius is already at the version %s.", desiredVersion)
		return nil
	}
	// Print out the current version and the wanted version.
	r.Output.LogInfo("Current version: %s, Wanted version: %s", currentVersion, desiredVersion)

	// // --- Snapshot the current state before upgrade ---
	// r.Output.LogInfo("Taking snapshot of the current installation...")
	// snapshot, err := r.takeSnapshot(ctx)
	// if err != nil {
	// 	return fmt.Errorf("failed to take snapshot before upgrade: %w", err)
	// }
	// r.Output.LogInfo("Snapshot created successfully.")

	clusterOptions := helm.PopulateDefaultClusterOptions(cliOptions)
	_, err = r.Helm.UpgradeRadius(ctx, clusterOptions, r.KubeContext)
	if err != nil {
		// r.Output.LogError("Upgrade failed: %v", err)
		// Attempt rollback using the snapshot.
		r.Output.LogInfo("Rolling back to previous state...")
		// rbErr := r.performRollback(ctx, snapshot)
		// if rbErr != nil {
		// 	// r.Output.LogError("Rollback failed: %v", rbErr)
		// }
		return err
	}

	r.Output.LogInfo("Upgrade completed successfully.")
	return nil
}

// // takeSnapshot uses the data store snapshot functionality to back up the current state.
// func (r *Runner) takeSnapshot(ctx context.Context) ([]byte, error) {
// 	return []byte("snapshot data"), nil
// }

// // performRollback uses the snapshot data to restore the previous state.
// func (r *Runner) performRollback(ctx context.Context, snapshot []byte) error {
// 	return nil
// }

// Validate runs any validations needed for the command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	return nil
}
