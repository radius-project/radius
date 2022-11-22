// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package delete

import (
	"context"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad resource delete` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "delete [resourceType] [resourceName]",
		Short: "Delete a RAD resource",
		Long:  "Deletes a RAD resource with the given name",
		Example: `
		sample list of resourceType: containers, gateways, httpRoutes, daprPubSubBrokers, daprInvokeHttpRoutes, extenders, mongoDatabases, rabbitMQMessageQueues, redisCaches, sqlDatabases, daprStateStores, daprSecretStores
		
		# Delete a container named orders
		rad resource delete containers orders`,
		Args: cobra.ExactArgs(2),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad resource delete` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	ResourceType      string
	ResourceName      string
	Format            string
}

// NewRunner creates a new instance of the `rad resource delete` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad resource delete` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	// TODO: support fallback workspace
	if !r.Workspace.IsNamedWorkspace() {
		return workspaces.ErrNamedWorkspaceRequired
	}

	resourceType, resourceName, err := cli.RequireResourceTypeAndName(args)
	if err != nil {
		return err
	}
	r.ResourceType = resourceType
	r.ResourceName = resourceName

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	return nil
}

// Run runs the `rad resource delete` command.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	var respFromCtx *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &respFromCtx)

	deleted, err := client.DeleteResource(ctxWithResp, r.ResourceType, r.ResourceName)
	if err != nil {
		return err
	}

	if deleted {
		r.Output.LogInfo("Resource deleted")
	} else {
		r.Output.LogInfo("Resource '%s' of type '%s' does not exist or has already been deleted", r.ResourceName, r.ResourceType)
	}

	return nil
}
