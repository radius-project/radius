/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package status

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad app status` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show Radius application status",
		Long:  `Show Radius application status, such as public endpoints and resource count. Shows details for the user's default application (if configured) by default.`,
		Args:  cobra.MaximumNArgs(1),
		Example: `
# Show status of current application
rad app status

# Show status of specified application
rad app status my-app

# Show status of specified application in a specified resource group
rad app status my-app --group my-group
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the Runner implementation for the `rad app status` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Workspace         *workspaces.Workspace
	Output            output.Interface

	ApplicationName string
	Format          string
}

// NewRunner creates an instance of the runner for the `rad app status` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad app status` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	// Allow '--group' to override scope
	scope, err := cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}
	r.Workspace.Scope = scope

	r.ApplicationName, err = cli.RequireApplicationArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	r.Format = format

	return nil
}

// Run runs the `rad app status` command.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	application, err := client.ShowApplication(ctx, r.ApplicationName)
	if clients.Is404Error(err) {
		return &cli.FriendlyError{Message: fmt.Sprintf("The application %q was not found or has been deleted.", r.ApplicationName)}
	} else if err != nil {
		return err
	}

	resourceList, err := client.ListAllResourcesByApplication(ctx, r.ApplicationName)
	if err != nil {
		return err
	}

	applicationStatus := clients.ApplicationStatus{
		Name:          *application.Name,
		ResourceCount: len(resourceList),
	}

	diagnosticsClient, err := r.ConnectionFactory.CreateDiagnosticsClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	for _, resource := range resourceList {
		resourceID, err := resources.ParseResource(*resource.ID)
		if err != nil {
			return err
		}

		publicEndpoint, err := diagnosticsClient.GetPublicEndpoint(ctx, clients.EndpointOptions{
			ResourceID: resourceID,
		})
		if err != nil {
			return err
		}

		if publicEndpoint != nil {
			applicationStatus.Gateways = append(applicationStatus.Gateways, clients.GatewayStatus{
				Name:     *resource.Name,
				Endpoint: *publicEndpoint,
			})
		}
	}

	err = r.Output.WriteFormatted(r.Format, applicationStatus, objectformats.GetApplicationStatusTableFormat())
	if err != nil {
		return err
	}

	if r.Format == output.FormatTable && len(applicationStatus.Gateways) > 0 {
		// Print newline for readability
		r.Output.LogInfo("")

		err = r.Output.WriteFormatted(r.Format, applicationStatus.Gateways, objectformats.GetApplicationGatewaysTableFormat())
		if err != nil {
			return err
		}
	}

	return nil
}
