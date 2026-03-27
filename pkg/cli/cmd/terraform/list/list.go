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

package list

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

// NewCommand creates an instance of the `rad terraform list` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed Terraform versions",
		Long:  "List all Terraform versions that have been installed, including their state and health status.",
		Example: `
# List all installed Terraform versions
rad terraform list

# List versions in JSON format
rad terraform list --output json
`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad terraform list` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	Workspace    *workspaces.Workspace
	Format       string
}

// NewRunner creates a new instance of the `rad terraform list` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

// Validate runs validation for the `rad terraform list` command.
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

	return nil
}

// Run runs the `rad terraform list` command.
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

	// Convert versions map to sorted slice for display
	versions := common.VersionsToList(status.Versions, status.CurrentVersion)

	if len(versions) == 0 {
		r.Output.LogInfo("No Terraform versions installed.")
		return nil
	}

	err = r.Output.WriteFormatted(r.Format, versions, versionsFormat())
	if err != nil {
		return err
	}

	return nil
}
