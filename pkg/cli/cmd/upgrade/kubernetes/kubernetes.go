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
	"strings"

	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/upgrade/preflight"
	"github.com/radius-project/radius/pkg/version"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad upgrade kubernetes` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "kubernetes",
		Short: "Upgrades Radius on a Kubernetes cluster",
		Long: `Upgrade Radius in a Kubernetes cluster using the Radius Helm chart.
By default 'rad upgrade kubernetes' will upgrade Radius to the version matching the rad CLI version.

This command upgrades the Radius control plane in the cluster associated with the active workspace.
To upgrade Radius in a different cluster, switch to the appropriate workspace first using 'rad workspace switch'.

The upgrade process includes preflight checks to ensure the cluster is ready for upgrade.
Preflight checks include:
- Kubernetes connectivity and permissions
- Helm connectivity and Radius installation status
- Version compatibility validation
- Cluster resource availability
- Custom configuration parameter validation

Radius is installed in the 'radius-system' namespace. For more information visit https://docs.radapp.io/concepts/technical/architecture/

Overrides can be set by specifying Helm chart values with the '--set' flag. For more information visit https://docs.radapp.io/guides/operations/kubernetes/install/.
`,
		Example: `# Upgrade Radius in the cluster of the active workspace
rad upgrade kubernetes

# Check which workspace is active
rad workspace show

# Switch to a different workspace before upgrading
rad workspace switch myworkspace
rad upgrade kubernetes

# Upgrade Radius with custom configuration
rad upgrade kubernetes --set key=value

# Upgrade to a specific version
rad upgrade kubernetes --version v0.47.0

# Upgrade to the latest available version
rad upgrade kubernetes --version latest

# Upgrade Radius with values from a file
rad upgrade kubernetes --set-file global.rootCA.cert=/path/to/rootCA.crt

# Skip preflight checks (not recommended)
rad upgrade kubernetes --skip-preflight

# Run only preflight checks without upgrading
rad upgrade kubernetes --preflight-only

# Upgrade Radius using a Helm chart from specified file path
rad upgrade kubernetes --chart /root/radius/deploy/Chart
`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddKubeContextFlagVar(cmd, &runner.KubeContext)
	cmd.Flags().StringVar(&runner.Chart, "chart", "", "Specify a file path to a helm chart to upgrade Radius from")
	cmd.Flags().StringVar(&runner.Version, "version", "", "Specify the version to upgrade to (default: CLI version, use 'latest' for latest available)")
	cmd.Flags().StringArrayVar(&runner.Set, "set", []string{}, "Set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")
	cmd.Flags().StringArrayVar(&runner.SetFile, "set-file", []string{}, "Set values from files on the command line (can specify multiple or separate files with commas: key1=filename1,key2=filename2)")
	cmd.Flags().BoolVar(&runner.SkipPreflight, "skip-preflight", false, "Skip preflight checks before upgrade (not recommended)")
	cmd.Flags().BoolVar(&runner.PreflightOnly, "preflight-only", false, "Run only preflight checks without performing the upgrade")

	return cmd, runner
}

// Runner is the Runner implementation for the `rad upgrade kubernetes` command.
type Runner struct {
	Helm   helm.Interface
	Output output.Interface

	KubeContext   string
	Chart         string
	Version       string
	Set           []string
	SetFile       []string
	SkipPreflight bool
	PreflightOnly bool
}

// NewRunner creates an instance of the runner for the `rad upgrade kubernetes` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Helm:   factory.GetHelmInterface(),
		Output: factory.GetOutput(),
	}
}

// Validate runs validation for the `rad upgrade kubernetes` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	if r.SkipPreflight && r.PreflightOnly {
		return fmt.Errorf("cannot specify both --skip-preflight and --preflight-only")
	}
	return nil
}

// Run runs the `rad upgrade kubernetes` command.
func (r *Runner) Run(ctx context.Context) error {
	// Get current installation state
	state, err := r.Helm.CheckRadiusInstall(r.KubeContext)
	if err != nil {
		return fmt.Errorf("failed to check current Radius installation: %w", err)
	}

	if !state.RadiusInstalled {
		return fmt.Errorf("Radius is not currently installed. Use 'rad install kubernetes' to install Radius first")
	}

	currentVersion := state.RadiusVersion

	// Resolve target version
	targetVersion, err := r.resolveTargetVersion()
	if err != nil {
		return fmt.Errorf("failed to resolve target version: %w", err)
	}

	r.Output.LogInfo("Current Radius version: %s", currentVersion)
	r.Output.LogInfo("Target Radius version: %s", targetVersion)

	// Run preflight checks unless skipped
	if !r.SkipPreflight {
		r.Output.LogInfo("Running preflight checks...")

		err = r.runPreflightChecks(ctx, currentVersion, targetVersion)
		if err != nil {
			return fmt.Errorf("preflight checks failed: %w", err)
		}

		r.Output.LogInfo("✓ All preflight checks passed")
	}

	// If preflight-only mode, exit here
	if r.PreflightOnly {
		r.Output.LogInfo("Preflight checks completed successfully. Upgrade was not performed due to --preflight-only flag.")
		return nil
	}

	// Perform the upgrade
	r.Output.LogInfo("Upgrading Radius from version %s to %s...", currentVersion, targetVersion)

	cliOptions := helm.CLIClusterOptions{
		Radius: helm.ChartOptions{
			ChartPath:    r.Chart,
			ChartVersion: targetVersion,
			SetArgs:      r.Set,
			SetFileArgs:  r.SetFile,
		},
	}

	clusterOptions := helm.PopulateDefaultClusterOptions(cliOptions)

	err = r.Helm.UpgradeRadius(ctx, clusterOptions, r.KubeContext)
	if err != nil {
		return fmt.Errorf("failed to upgrade Radius: %w", err)
	}

	r.Output.LogInfo("✓ Radius upgrade completed successfully!")
	return nil
}

// runPreflightChecks executes all preflight checks before upgrade.
func (r *Runner) runPreflightChecks(ctx context.Context, currentVersion, targetVersion string) error {
	// Create preflight check registry
	registry := preflight.NewRegistry(r.Output)

	// Add checks to registry in order of importance
	// 1. Basic connectivity checks first
	registry.AddCheck(preflight.NewKubernetesConnectivityCheck(r.KubeContext))
	registry.AddCheck(preflight.NewHelmConnectivityCheck(r.Helm, r.KubeContext))

	// 2. Installation and version validation
	registry.AddCheck(preflight.NewRadiusInstallationCheck(r.Helm, r.KubeContext))
	registry.AddCheck(preflight.NewVersionCompatibilityCheck(currentVersion, targetVersion))

	// 3. Configuration validation (warnings only)
	registry.AddCheck(preflight.NewCustomConfigValidationCheck(r.Set, r.SetFile))

	// 4. Resource availability check (warnings only)
	registry.AddCheck(preflight.NewKubernetesResourceCheck(r.KubeContext))

	// Run all checks - registry handles execution and logging
	_, err := registry.RunChecks(ctx)
	return err
}

// resolveTargetVersion resolves the target version for upgrade based on user input.
func (r *Runner) resolveTargetVersion() (string, error) {
	// If no version specified, use CLI version
	if r.Version == "" {
		return version.Version(), nil
	}

	// If user specified "latest", resolve to actual latest version
	if strings.ToLower(r.Version) == "latest" {
		latestVersion, err := r.fetchLatestRadiusVersion()
		if err != nil {
			return "", fmt.Errorf("failed to fetch latest Radius version: %w", err)
		}
		r.Output.LogInfo("Resolved 'latest' to version: %s", latestVersion)
		return latestVersion, nil
	}

	// Otherwise, use the specified version
	return r.Version, nil
}

// fetchLatestRadiusVersion fetches the latest Radius version from Helm repository.
func (r *Runner) fetchLatestRadiusVersion() (string, error) {
	// Use the Helm interface to get the latest chart version
	return r.Helm.GetLatestRadiusVersion(context.Background())
}
