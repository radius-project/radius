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

package uninstall

import (
	"context"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/terraform/common"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/terraform/installer"
	"github.com/spf13/cobra"
)

const (
	// DefaultTimeout is the default timeout for waiting for uninstallation to complete.
	DefaultTimeout = 10 * time.Minute

	// DefaultPollInterval is the default interval for polling uninstallation status.
	DefaultPollInterval = 2 * time.Second
)

// NewCommand creates an instance of the `rad terraform uninstall` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall Terraform from Radius",
		Long:  "Uninstall Terraform from Radius. This removes the currently installed Terraform binary.",
		Example: `
# Uninstall current Terraform version
rad terraform uninstall

# Uninstall a specific version
rad terraform uninstall --version 1.6.3

# Uninstall all installed versions
rad terraform uninstall --all

# Uninstall and remove version metadata (purge history)
rad terraform uninstall --purge

# Uninstall all versions and purge all metadata
rad terraform uninstall --all --purge

# Uninstall Terraform and wait for completion
rad terraform uninstall --wait

# Uninstall with a custom timeout (when using --wait)
rad terraform uninstall --wait --timeout 5m
`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	cmd.Flags().StringP("version", "v", "", "Specific version to uninstall")
	cmd.Flags().Bool("all", false, "Uninstall all installed versions")
	cmd.Flags().Bool("purge", false, "Remove version metadata from database (clears history)")
	cmd.Flags().Bool("wait", false, "Wait for the uninstallation to complete")
	cmd.Flags().Duration("timeout", DefaultTimeout, "Timeout when waiting for uninstallation (requires --wait)")

	return cmd, runner
}

// Runner is the runner implementation for the `rad terraform uninstall` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	Workspace    *workspaces.Workspace

	Version      string
	UninstallAll bool
	Purge        bool
	Wait         bool
	Timeout      time.Duration
	PollInterval time.Duration
}

// NewRunner creates a new instance of the `rad terraform uninstall` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
		PollInterval: DefaultPollInterval,
	}
}

// Validate runs validation for the `rad terraform uninstall` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	r.Version, err = cmd.Flags().GetString("version")
	if err != nil {
		return err
	}

	r.UninstallAll, err = cmd.Flags().GetBool("all")
	if err != nil {
		return err
	}

	r.Purge, err = cmd.Flags().GetBool("purge")
	if err != nil {
		return err
	}

	r.Wait, err = cmd.Flags().GetBool("wait")
	if err != nil {
		return err
	}

	r.Timeout, err = cmd.Flags().GetDuration("timeout")
	if err != nil {
		return err
	}

	// Validate that --timeout requires --wait
	if cmd.Flags().Changed("timeout") && !r.Wait {
		return clierrors.Message("--timeout requires --wait to be set.")
	}

	// Validate that --version and --all are mutually exclusive
	if r.Version != "" && r.UninstallAll {
		return clierrors.Message("--version and --all cannot be used together.")
	}

	// Validate that --wait cannot be used with --all (would need complex tracking)
	if r.UninstallAll && r.Wait {
		return clierrors.Message("--wait cannot be used with --all.")
	}

	return nil
}

// Run runs the `rad terraform uninstall` command.
func (r *Runner) Run(ctx context.Context) error {
	connection, err := r.Workspace.Connect(ctx)
	if err != nil {
		return err
	}

	client := common.NewClient(connection)

	// Handle --all flag: uninstall all versions
	if r.UninstallAll {
		return r.uninstallAll(ctx, client)
	}

	// Get current version before uninstalling so we can track its state
	var priorVersion string
	if r.Wait || r.Version == "" {
		status, err := client.Status(ctx)
		if err != nil {
			return err
		}
		priorVersion = status.CurrentVersion
		if priorVersion == "" && r.Version == "" {
			r.Output.LogInfo("No Terraform version is currently installed.")
			return nil
		}
	}

	r.Output.LogInfo("Uninstalling Terraform...")

	// Send uninstall request
	req := installer.UninstallRequest{
		Version: r.Version, // Empty string means uninstall current version
		Purge:   r.Purge,
	}
	if err := client.Uninstall(ctx, req); err != nil {
		return err
	}

	if r.Version != "" {
		r.Output.LogInfo("Terraform uninstall queued (version=%s).", r.Version)
	} else {
		r.Output.LogInfo("Terraform uninstall queued.")
	}

	if r.Wait {
		return r.waitForUninstallation(ctx, client, priorVersion)
	}

	return nil
}

// uninstallAll uninstalls all installed Terraform versions.
func (r *Runner) uninstallAll(ctx context.Context, client *common.Client) error {
	status, err := client.Status(ctx)
	if err != nil {
		return err
	}

	if len(status.Versions) == 0 {
		r.Output.LogInfo("No Terraform versions to process.")
		return nil
	}

	if r.Purge {
		r.Output.LogInfo("Purging all Terraform versions...")
	} else {
		r.Output.LogInfo("Uninstalling all Terraform versions...")
	}

	// Process each version
	processCount := 0
	for version, vs := range status.Versions {
		// Skip versions that are already uninstalled or failed (unless purging)
		if !r.Purge && (vs.State == installer.VersionStateUninstalled || vs.State == installer.VersionStateFailed) {
			continue
		}

		req := installer.UninstallRequest{Version: version, Purge: r.Purge}
		if err := client.Uninstall(ctx, req); err != nil {
			r.Output.LogInfo("Failed to queue uninstall for version %s: %s", version, err)
			continue
		}
		if r.Purge {
			r.Output.LogInfo("Queued purge for version %s", version)
		} else {
			r.Output.LogInfo("Queued uninstall for version %s", version)
		}
		processCount++
	}

	if processCount == 0 {
		r.Output.LogInfo("No versions to process.")
	} else {
		r.Output.LogInfo("All requests queued.")
	}
	return nil
}

// waitForUninstallation polls the status endpoint until the uninstallation completes or fails.
// Success is defined as CurrentVersion being empty (no Terraform installed).
func (r *Runner) waitForUninstallation(ctx context.Context, client *common.Client, priorVersion string) error {
	r.Output.LogInfo("Waiting for uninstallation to complete...")

	deadline := time.Now().Add(r.Timeout)
	pollInterval := r.PollInterval

	for {
		if time.Now().After(deadline) {
			return clierrors.Message("Timed out waiting for Terraform uninstallation to complete.")
		}

		status, err := client.Status(ctx)
		if err != nil {
			return err
		}

		if status.Queue != nil && status.Queue.InProgress != nil {
			op := operationFromQueue(*status.Queue.InProgress)
			if op == installer.OperationInstall {
				return clierrors.Message("Terraform install in progress; uninstall wait requires no Terraform installed.")
			}
		}

		// Success: no current version installed
		if status.CurrentVersion == "" {
			r.Output.LogInfo("Terraform uninstalled successfully.")
			return nil
		}

		if priorVersion != "" && status.CurrentVersion != priorVersion {
			return clierrors.Message("Terraform version %s is now installed; uninstall wait requires no Terraform installed.", status.CurrentVersion)
		}

		// Check if the prior version uninstall failed
		if priorVersion != "" {
			if vs, ok := status.Versions[priorVersion]; ok {
				if vs.State == installer.VersionStateFailed {
					if vs.LastError != "" {
						return clierrors.Message("Terraform uninstallation failed: %s", vs.LastError)
					}
					return clierrors.Message("Terraform uninstallation failed.")
				}
			}
		}

		// Check overall state for failures
		if status.State == installer.ResponseStateFailed {
			if status.LastError != "" {
				return clierrors.Message("Terraform uninstallation failed: %s", status.LastError)
			}
			return clierrors.Message("Terraform uninstallation failed.")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
			// Continue polling
		}
	}
}

func operationFromQueue(inProgress string) installer.Operation {
	parts := strings.SplitN(inProgress, ":", 2)
	if len(parts) == 0 {
		return ""
	}
	return installer.Operation(parts[0])
}
