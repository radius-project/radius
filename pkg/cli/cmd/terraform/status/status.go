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

package status

import (
	"context"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/terraform/common"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad terraform status` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show Terraform installation status",
		Long:  "Show Terraform installation status, including the current version, state, and other details.",
		Example: `
# Show Terraform status
rad terraform status

# Show Terraform status with all installed versions
rad terraform status --all

# Show Terraform status in JSON format
rad terraform status --output json
`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddOutputFlag(cmd)
	cmd.Flags().BoolP("all", "a", false, "Show all installed versions")

	return cmd, runner
}

// Runner is the runner implementation for the `rad terraform status` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	Workspace    *workspaces.Workspace
	Format       string
	ShowAll      bool
}

// NewRunner creates a new instance of the `rad terraform status` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

// Validate runs validation for the `rad terraform status` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	showAll, err := cmd.Flags().GetBool("all")
	if err != nil {
		return err
	}
	r.ShowAll = showAll

	return nil
}

// Run runs the `rad terraform status` command.
func (r *Runner) Run(ctx context.Context) error {
	connection, err := r.Workspace.Connect(ctx)
	if err != nil {
		return err
	}

	client := common.NewClient(connection)

	status, err := client.Status(ctx)
	if err != nil {
		return err
	}

	// If --all flag is set, show all versions instead of just current status
	if r.ShowAll {
		versions := common.VersionsToList(status.Versions, status.CurrentVersion)
		if len(versions) == 0 {
			r.Output.LogInfo("No Terraform versions installed.")
			return nil
		}
		return r.Output.WriteFormatted(r.Format, versions, versionsFormat())
	}

	err = r.Output.WriteFormatted(r.Format, status, statusFormat())
	if err != nil {
		return err
	}

	return nil
}
