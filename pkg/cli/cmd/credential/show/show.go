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

package show

import (
	"context"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clierrors"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/cmd/credential/common"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad credential show` command.
//
// # Function Explanation
//
// NewCommand creates a new Cobra command that can be used to show details of a configured cloud provider credential, with
// optional flags for output and workspace.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "show [name]",
		Short: "Show details of a configured cloud provider credential",
		Long:  "Show details of a configured cloud provider credential." + common.LongDescriptionBlurb,
		Example: `
# Show cloud providers details for Azure
rad credential show azure

# Show cloud providers details for AWS
rad credential show aws
`,
		Args: cobra.ExactArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad credential show` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Format            string
	Workspace         *workspaces.Workspace
	Kind              string
}

// NewRunner creates a new instance of the `rad credential show` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad credential show` command.
//
// # Function Explanation
//
// Validate checks the workspace, output format, and cloud provider name from the command line arguments and returns
// an error if any of them are invalid.
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

	r.Kind = args[0] // Validated by Cobra
	err = common.ValidateCloudProviderName(r.Kind)
	if err != nil {
		return err
	}

	return nil
}

// Run runs the `rad credential show` command.
//
// # Function Explanation
//
// Run attempts to retrieve the credentials for a given cloud provider and prints them in a formatted table. It
// returns an error if the cloud provider cannot be found or if there is an issue with writing the formatted table.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("Showing credential for cloud provider %q for Radius installation %q...", r.Kind, r.Workspace.FmtConnection())
	client, err := r.ConnectionFactory.CreateCredentialManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	providers, err := client.Get(ctx, r.Kind)
	if clients.Is404Error(err) {
		return clierrors.Message("The cloud provider %q could not be found.", r.Kind)
	} else if err != nil {
		return err
	}

	err = r.Output.WriteFormatted(r.Format, providers, objectformats.GetCloudProviderTableFormat(r.Kind))
	if err != nil {
		return err
	}

	return nil
}
