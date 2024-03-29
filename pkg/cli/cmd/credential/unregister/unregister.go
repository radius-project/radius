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

package unregister

import (
	"context"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/credential/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad credential unregister` command.
//

// NewCommand creates a new Cobra command and a new Runner to unregister a configured cloud provider credential from the
// Radius installation, and adds flags for output and workspace.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "unregister",
		Short: "Unregisters a configured cloud provider credential from the Radius installation",
		Long:  "Unregisters a configured cloud provider credential from the Radius installation." + common.LongDescriptionBlurb,
		Example: `
# Unregister Azure cloud provider credential
rad credential unregister azure

# Unregister AWS cloud provider credential
rad credential unregister aws
`,
		Args: cobra.ExactArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad credential unregister` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Format            string
	Workspace         *workspaces.Workspace
	Kind              string
}

// NewRunner creates a new instance of the `rad credential unregister` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad credential unregister` command.
//

// Validate validates the command line arguments, workspace and output format, and checks if the cloud provider
// name is valid, returning an error if any of these checks fail.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Validate command line args and
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

// Run runs the `rad credential unregister` command.
//

// Run attempts to delete a cloud provider credential for a given Radius installation, and logs a message
// depending on whether the credential was found and deleted or not.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("Unregistering %q cloud provider credential for Radius installation %q...", r.Kind, r.Workspace.FmtConnection())
	client, err := r.ConnectionFactory.CreateCredentialManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	deleted, err := client.Delete(ctx, r.Kind)
	if err != nil {
		return err
	}

	if deleted {
		r.Output.LogInfo("Cloud provider deleted.")
	} else {
		r.Output.LogInfo("Cloud provider %q was not found or has been already deleted.", r.Kind)
	}

	return nil
}
