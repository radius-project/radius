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

package preview

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/app/status"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// NewCommand creates an instance of the command and runner for the `rad app status --preview` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show Radius Application status (preview)",
		Long:  `Show Radius.Core application status using the preview API surface, including resource count and public endpoints.`,
		Args:  cobra.MaximumNArgs(1),
		Example: `
# Show status of specified application
rad app status my-app --preview

# Show status of specified application in a specified resource group
rad app status my-app --group my-group --preview
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the preview `rad app status` command.
type Runner struct {
	ConfigHolder            *framework.ConfigHolder
	ConnectionFactory       connections.Factory
	Output                  output.Interface
	Workspace               *workspaces.Workspace
	RadiusCoreClientFactory *corerpv20250801.ClientFactory

	ApplicationName string
	Format          string
}

// NewRunner creates a new instance of the preview status runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the preview status command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	r.Workspace.Scope, err = cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}

	r.ApplicationName, err = cli.RequireApplicationArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	r.Format, err = cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	return nil
}

// Run runs the preview `rad app status` command.
func (r *Runner) Run(ctx context.Context) error {
	if r.RadiusCoreClientFactory == nil {
		factory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace, r.Workspace.Scope)
		if err != nil {
			return err
		}
		r.RadiusCoreClientFactory = factory
	}

	appClient := r.RadiusCoreClientFactory.NewApplicationsClient()

	// Fetch the application resource.
	application, err := appClient.Get(ctx, r.ApplicationName, &corerpv20250801.ApplicationsClientGetOptions{})
	if clients.Is404Error(err) {
		return clierrors.Message("The application %q was not found or has been deleted.", r.ApplicationName)
	} else if err != nil {
		return err
	}

	// Enumerate resources owned by this application using the management client.
	managementClient, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	applicationID := r.Workspace.Scope + "/providers/" + datamodel.ApplicationResourceType_v20250801preview + "/" + r.ApplicationName
	resourceList, err := managementClient.ListResourcesInApplication(ctx, applicationID)
	if err != nil {
		return err
	}

	applicationStatus := clients.ApplicationStatus{
		Name:          *application.Name,
		ResourceCount: len(resourceList),
	}

	// Gather public endpoints from gateway resources.
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

	err = r.Output.WriteFormatted(r.Format, applicationStatus, status.StatusFormat())
	if err != nil {
		return err
	}

	if r.Format == output.FormatTable && len(applicationStatus.Gateways) > 0 {
		r.Output.LogInfo("")
		err = r.Output.WriteFormatted(r.Format, applicationStatus.Gateways, status.GatewayFormat())
		if err != nil {
			return err
		}
	}

	return nil
}
